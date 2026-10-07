package clickhouse

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"time"

	driver "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// clickhouseConnectTimeout is the connection time limit.
const clickhouseConnectTimeout = 15 * time.Second

// clientName is the name this client sends, which the activity list of the server shows.
const clientName = "masume"

// buildTLS returns the TLS settings of the connection. The native protocol does not
// negotiate, so an unset mode and a preferred mode both connect without encryption.
func buildTLS(profile cfg.Profile) (*tls.Config, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	if policy == core.PolicyPrefer {
		return nil, nil
	}
	return db.BuildPolicyTLS(policy, profile.Host, profile.BuildSSLFiles())
}

// buildOptions returns the connection as the driver takes it. A read-only session sends no
// setting of its own, because the server refuses to change one.
func buildOptions(profile cfg.Profile, password string) (*driver.Options, error) {
	settings := driver.Settings{}
	if profile.AccessMode != cfg.AccessReadOnly {
		// A staged edit is a mutation, and this makes the server finish it before it
		// answers, so the grid reads the row back as it now stands.
		settings["mutations_sync"] = 1
	}
	tlsConfig, err := buildTLS(profile)
	if err != nil {
		return nil, err
	}
	options := &driver.Options{DialTimeout: clickhouseConnectTimeout}
	if len(profile.Options) > 0 {
		parsed, parseErr := driver.ParseDSN("clickhouse://localhost?" + cfg.WriteOptions(profile.Options))
		if parseErr != nil {
			return nil, db.WrapDatabaseMessage("invalid options: "+parseErr.Error(), parseErr)
		}
		if _, set := profile.Options["dial_timeout"]; !set {
			parsed.DialTimeout = clickhouseConnectTimeout
		}
		maps.Copy(settings, parsed.Settings)
		options = parsed
	}
	dialHost, dialPort := profile.DialAddress()
	options.Addr = []string{fmt.Sprintf("%s:%d", dialHost, dialPort)}
	options.Auth = driver.Auth{Database: profile.Database, Username: profile.User, Password: password}
	options.Settings = settings
	options.TLS = tlsConfig
	options.ClientInfo = driver.ClientInfo{Products: []struct{ Name, Version string }{{Name: clientName}}}
	return options, nil
}

// openClickhousePool opens a pool limited to one connection.
func openClickhousePool(profile cfg.Profile, password string) (*sql.DB, error) {
	options, err := buildOptions(profile, password)
	if err != nil {
		return nil, err
	}
	pool := driver.OpenDB(options)
	pool.SetMaxOpenConns(1)
	return pool, nil
}

// buildQueryID returns the id this client gives one statement, so a second connection can
// stop it.
func buildQueryID() string {
	held := make([]byte, 8)
	if _, err := rand.Read(held); err != nil {
		return ""
	}
	return clientName + "-" + hex.EncodeToString(held)
}

// checkSettingNames reads the names of the server settings in the options on a connection
// without them. The server refuses every statement of a session with an unknown setting,
// and the driver reports that as a bad connection.
func checkSettingNames(ctx context.Context, profile cfg.Profile, password string) error {
	options, err := buildOptions(profile, password)
	if err != nil || len(profile.Options) == 0 {
		return err
	}
	names := []string{}
	for name := range options.Settings {
		if _, set := profile.Options[name]; set {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	plain := profile
	plain.Options = nil
	pool, err := openClickhousePool(plain, password)
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()
	rows, err := pool.QueryContext(context.WithoutCancel(ctx),
		"select name from system.settings where name in ?", names)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	known := map[string]bool{}
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return scanErr
		}
		known[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, name := range slices.Sorted(slices.Values(names)) {
		if !known[name] {
			return db.NewDatabaseError("unknown setting %s in the options", name)
		}
	}
	return nil
}
