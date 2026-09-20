package mysql

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// mysqlConnectTimeout is the connection time limit.
const mysqlConnectTimeout = 15 * time.Second

// mysqlTLSName is the prefix of a registered TLS configuration name. The driver requires a
// name for custom TLS settings.
const mysqlTLSName = "masume"

// buildMysqlTLSName returns the registration name of these settings. Two profiles with
// different certificate files register two configurations.
func buildMysqlTLSName(profile cfg.Profile, policy core.SSLPolicy) string {
	files := profile.BuildSSLFiles()
	held := sha256.Sum256([]byte(strings.Join([]string{
		string(policy), profile.Host, files.RootCert, files.Cert, files.Key,
	}, "\x00")))
	return mysqlTLSName + "-" + hex.EncodeToString(held[:8])
}

// droppedLog discards direct driver output to the terminal.
type droppedLog struct{}

func (droppedLog) Print(...any) {}

func init() {
	_ = driver.SetLogger(droppedLog{})
}

// resolveMysqlTLS returns the registered TLS configuration name and permission to connect
// without encryption. Unset and prefer modes allow unencrypted connections.
func resolveMysqlTLS(profile cfg.Profile) (string, bool, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	if policy == core.PolicyOff {
		return "false", false, nil
	}
	// Preferred mode uses TLS if available and otherwise connects without encryption.
	mayFallBack := policy == core.PolicyUnset || policy == core.PolicyPrefer
	if mayFallBack {
		policy = core.PolicyEncryptOnly
	}
	// The driver verifies the certificate against the dial address. This config sets
	// ServerName to the database host.
	config, err := db.BuildPolicyTLS(policy, profile.Host, profile.BuildSSLFiles())
	if err != nil {
		return "", false, err
	}
	name := buildMysqlTLSName(profile, policy)
	if err := driver.RegisterTLSConfig(name, config); err != nil {
		return "", false, err
	}
	return name, mayFallBack, nil
}

func buildMysqlDsn(profile cfg.Profile, password string) (string, error) {
	config := driver.NewConfig()
	config.User = profile.User
	config.Passwd = password
	config.Net = "tcp"
	dialHost, dialPort := profile.DialAddress()
	config.Addr = fmt.Sprintf("%s:%d", dialHost, dialPort)

	tlsName, mayFallBack := "false", false
	// A unix socket carries no TLS, and the driver dials the file itself.
	if core.IsSocketHost(dialHost) {
		config.Net, config.Addr = "unix", dialHost
	} else {
		resolvedName, resolvedFallback, err := resolveMysqlTLS(profile)
		if err != nil {
			return "", err
		}
		tlsName, mayFallBack = resolvedName, resolvedFallback
	}
	config.DBName = profile.Database
	config.Timeout = mysqlConnectTimeout
	config.TLSConfig = tlsName
	config.AllowFallbackToPlaintext = mayFallBack
	// Query buffers can contain multiple statements.
	config.MultiStatements = true
	// MySQL dates have no time zone. Text preserves their stored values.
	config.ParseTime = false
	config.InterpolateParams = false
	return config.FormatDSN(), nil
}

// openMysqlPool opens a pool limited to one connection.

func openMysqlPool(profile cfg.Profile, password string) (*sql.DB, error) {
	dsn, err := buildMysqlDsn(profile, password)
	if err != nil {
		return nil, err
	}
	pool, openErr := sql.Open("mysql", dsn)
	if openErr != nil {
		return nil, openErr
	}
	pool.SetMaxOpenConns(1)
	return pool, nil
}
