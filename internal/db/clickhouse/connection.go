package clickhouse

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
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
	dialHost, dialPort := profile.DialAddress()
	return &driver.Options{
		Addr: []string{fmt.Sprintf("%s:%d", dialHost, dialPort)},
		Auth: driver.Auth{
			Database: profile.Database, Username: profile.User, Password: password,
		},
		Settings:    settings,
		TLS:         tlsConfig,
		DialTimeout: clickhouseConnectTimeout,
		ClientInfo:  driver.ClientInfo{Products: []struct{ Name, Version string }{{Name: clientName}}},
	}, nil
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
