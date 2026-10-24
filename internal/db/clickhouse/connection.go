package clickhouse

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// clickhouseConnectTimeout is the connection time limit.
const clickhouseConnectTimeout = 15 * time.Second

// idleConnectionTimeout is the time an unused connection stays open. It is shorter than the
// keep-alive time of the server.
const idleConnectionTimeout = 2 * time.Second

// clientName is the name this client sends, which the activity list of the server shows.
const clientName = "masume"

// nativePorts are the ports of the native protocol of the server.
var nativePorts = map[int]bool{9000: true, 9440: true}

// checkPort refuses a port of the native protocol.
func checkPort(profile cfg.Profile) error {
	if nativePorts[profile.Port] {
		return db.NewDatabaseError(
			"port %d is the ClickHouse native protocol port; "+
				"the HTTP port is 8123, or 8443 with TLS", profile.Port)
	}
	return nil
}

// buildTLS returns the TLS settings of the connection. An unset mode and a preferred mode
// connect without encryption. The server name is the host of the profile, also through a
// tunnel.
func buildTLS(profile cfg.Profile) (*tls.Config, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	if policy == core.PolicyPrefer {
		return nil, nil
	}
	config, err := db.BuildPolicyTLS(policy, profile.Host, profile.BuildSSLFiles())
	if err != nil || config == nil {
		return config, err
	}
	if config.ServerName == "" {
		config = config.Clone()
		config.ServerName = profile.Host
	}
	return config, nil
}

// clientOptions are the options of a profile this client reads itself. Every other option
// is a server setting.
var clientOptions = []string{"dial_timeout", "compress"}

// readServerSettings returns the options of a profile that are server settings.
func readServerSettings(options map[string]string) map[string]string {
	settings := map[string]string{}
	for name, value := range options {
		if !slices.Contains(clientOptions, name) {
			settings[name] = value
		}
	}
	return settings
}

// buildClient returns the HTTP client of a profile.
func buildClient(profile cfg.Profile, password string) (*httpClient, error) {
	tlsConfig, err := buildTLS(profile)
	if err != nil {
		return nil, err
	}
	dialTimeout := clickhouseConnectTimeout
	if written, set := profile.Options["dial_timeout"]; set {
		parsed, parseErr := time.ParseDuration(written)
		if parseErr != nil {
			return nil, db.WrapDatabaseMessage(
				"invalid options: dial_timeout: "+parseErr.Error(), parseErr)
		}
		dialTimeout = parsed
	}
	settings := url.Values{}
	if written, set := profile.Options["compress"]; set {
		compress, parseErr := strconv.ParseBool(written)
		if written == "gzip" {
			compress, parseErr = true, nil
		}
		if parseErr != nil {
			return nil, db.WrapDatabaseMessage(
				"invalid options: compress is true, false or gzip", parseErr)
		}
		if compress {
			settings.Set("enable_http_compression", "1")
		}
	}
	for name, value := range readServerSettings(profile.Options) {
		settings.Set(name, value)
	}
	if profile.AccessMode != cfg.AccessReadOnly {
		// A staged edit is a mutation, and this makes the server finish it before it
		// answers, so the grid reads the row back as it now stands.
		settings.Set("mutations_sync", "1")
	}
	settings.Set("default_format", resultFormat)
	settings.Set("date_time_output_format", "unix_timestamp")
	settings.Set("output_format_decimal_trailing_zeros", "0")
	settings.Set("output_format_tsv_crlf_end_of_line", "0")

	scheme := "http"
	if tlsConfig != nil {
		scheme = "https"
	}
	dialHost, dialPort := profile.DialAddress()
	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	return &httpClient{
		client: &http.Client{Transport: &http.Transport{
			DialContext: dialer.DialContext, TLSClientConfig: tlsConfig,
			TLSHandshakeTimeout: dialTimeout, IdleConnTimeout: idleConnectionTimeout,
			MaxIdleConnsPerHost: 4,
		}},
		endpoint: fmt.Sprintf("%s://%s/", scheme,
			net.JoinHostPort(dialHost, strconv.Itoa(dialPort))),
		user: profile.User, password: password, settings: settings,
	}, nil
}

// buildClientID returns a new id of a statement or of a server session.
func buildClientID() string {
	held := make([]byte, 8)
	if _, err := rand.Read(held); err != nil {
		return ""
	}
	return clientName + "-" + hex.EncodeToString(held)
}

// checkSettingNames refuses an option that is no server setting. The check runs on a request
// without the options.
func checkSettingNames(ctx context.Context, profile cfg.Profile, password string) error {
	names := []string{}
	for name := range readServerSettings(profile.Options) {
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	plain := profile
	plain.Options = nil
	client, err := buildClient(plain, password)
	if err != nil {
		return err
	}
	defer client.close()
	statement, err := bindParams("select name from system.settings where name in ?",
		[]any{names})
	if err != nil {
		return err
	}
	rows, err := readAllRows(ctx, client, statement, nil)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, row := range rows {
		known[db.ReadAnyText(row[0])] = true
	}
	for _, name := range slices.Sorted(slices.Values(names)) {
		if !known[name] {
			return db.NewDatabaseError("unknown setting %s in the options", name)
		}
	}
	return nil
}

// readAllRows runs a read and returns every row of it.
func readAllRows(
	ctx context.Context, client *httpClient, statement string, params url.Values,
) ([][]any, error) {
	response, err := client.send(ctx, statement, params)
	if err != nil {
		return nil, err
	}
	reader, err := openResult(response)
	if err != nil {
		return nil, err
	}
	defer reader.close()
	return reader.readRows(-1)
}
