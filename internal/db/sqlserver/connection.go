package sqlserver

import (
	"crypto/tls"
	"database/sql"
	"time"

	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// sqlserverConnectTimeout is the connection time limit.
const sqlserverConnectTimeout = 15 * time.Second

// applicationName is the name this client sends with the connection.
const applicationName = "masume"

// buildEncryption returns the encryption of the connection with the TLS settings it uses.
// Unset and prefer modes encrypt the login only.
func buildEncryption(profile cfg.Profile) (msdsn.Encryption, *tls.Config, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	if policy == core.PolicyOff {
		return msdsn.EncryptionDisabled, nil, nil
	}
	var encryption msdsn.Encryption = msdsn.EncryptionRequired
	if policy == core.PolicyUnset || policy == core.PolicyPrefer {
		encryption, policy = msdsn.EncryptionOff, core.PolicyEncryptOnly
	}
	config, err := db.BuildPolicyTLS(policy, profile.Host, profile.BuildSSLFiles())
	return encryption, config, err
}

// buildSqlserverConfig returns the connection as the driver takes it.
func buildSqlserverConfig(profile cfg.Profile, password string) (msdsn.Config, error) {
	encryption, tlsConfig, err := buildEncryption(profile)
	if err != nil {
		return msdsn.Config{}, err
	}
	dialHost, dialPort := profile.DialAddress()
	return msdsn.Config{
		Host: dialHost, Port: uint64(dialPort), Database: profile.Database,
		User: profile.User, Password: password,
		Encryption: encryption, TLSConfig: tlsConfig,
		TrustServerCertificate: tlsConfig != nil && tlsConfig.InsecureSkipVerify,
		AppName:                applicationName,
		DialTimeout:            sqlserverConnectTimeout,
		// The driver dials the protocols of this list, and it dials none without one.
		Protocols:  []string{"tcp"},
		Parameters: map[string]string{},
	}, nil
}

// openSqlserverPool opens a pool limited to one connection.
func openSqlserverPool(profile cfg.Profile, password string) (*sql.DB, error) {
	config, err := buildSqlserverConfig(profile, password)
	if err != nil {
		return nil, err
	}
	pool := sql.OpenDB(mssql.NewConnectorConfig(config))
	pool.SetMaxOpenConns(1)
	return pool, nil
}
