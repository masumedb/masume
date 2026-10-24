package turso

import (
	"errors"
	"net"
	"net/url"
	"strconv"

	_ "github.com/tursodatabase/libsql-client-go/libsql"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/sqlite"
)

// Support is everything known about Turso before a connection exists. It speaks the libSQL
// protocol, so it takes the SQLite dialect and language.
var Support = sqlite.BuildSupport(core.EngineTurso)

// Flavour opens the libSQL server over its websocket protocol. The HTTP protocol of the
// same server runs each statement on a connection of its own, which ends a transaction
// before the next statement reaches the server.
var Flavour = sqlite.Flavour{DriverName: "libsql", BuildSource: BuildSource}

// BuildSource returns the websocket URL of that profile. The password is the auth token.
func BuildSource(profile cfg.Profile, password string) (string, error) {
	if profile.Host == "" {
		return "", errors.New("the host of the database is missing")
	}

	scheme := "wss"
	if profile.SSLMode == core.SSLDisable {
		scheme = "ws"
	}
	// The libSQL driver checks the certificate against the host in the URL, which is the
	// local end of the tunnel.
	if scheme == "wss" && profile.DialHost != "" {
		return "", errors.New("a TLS connection cannot go through an ssh tunnel; set sslmode to disable")
	}
	dialHost, dialPort := profile.DialAddress()
	host := dialHost
	if dialPort > 0 {
		host = net.JoinHostPort(host, strconv.Itoa(dialPort))
	}

	query := url.Values{}
	for name, value := range profile.Options {
		query.Set(name, value)
	}
	if password != "" {
		query.Set("authToken", password)
	}
	built := url.URL{Scheme: scheme, Host: host, RawQuery: query.Encode()}
	return built.String(), nil
}
