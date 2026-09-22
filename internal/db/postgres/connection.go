package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// postgresConnectTimeout is the connection time limit.
const postgresConnectTimeout = 15 * time.Second

// buildPostgresTLS returns TLS settings and permission to retry without encryption. Unset and prefer modes permit unencrypted fallback.
func buildPostgresTLS(profile cfg.Profile) (*tls.Config, bool, error) {
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	if policy == core.PolicyOff {
		return nil, false, nil
	}
	// Unset and prefer modes encrypt without certificate verification.
	mayFallBack := policy == core.PolicyUnset || policy == core.PolicyPrefer
	if mayFallBack {
		policy = core.PolicyEncryptOnly
	}
	config, err := db.BuildPolicyTLS(policy, profile.Host, profile.BuildSSLFiles())
	return config, mayFallBack, err
}

func buildPostgresConfig(profile cfg.Profile, password string) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig("")
	if err != nil {
		config = &pgx.ConnConfig{}
	}
	dialHost, dialPort := profile.DialAddress()
	config.Host = dialHost
	config.Port = uint16(dialPort)
	config.Database = profile.Database
	config.User = profile.User
	config.Password = password
	config.ConnectTimeout = postgresConnectTimeout
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["application_name"] = "masume"
	// PostgreSQL also enforces the statement time limit on the server.
	if profile.StatementTimeout > 0 {
		config.RuntimeParams["statement_timeout"] =
			strconv.FormatInt(profile.StatementTimeout.Milliseconds(), 10)
	}

	tlsConfig, mayFallBack, err := buildPostgresTLS(profile)
	if err != nil {
		return nil, err
	}
	config.TLSConfig = tlsConfig
	// A unix socket carries no TLS, and the server refuses a client that offers it.
	if core.IsSocketHost(dialHost) {
		config.TLSConfig = nil
		return config, nil
	}
	// Unset and prefer modes permit a retry without TLS.
	if tlsConfig != nil && mayFallBack {
		config.Fallbacks = []*pgconn.FallbackConfig{
			{Host: dialHost, Port: uint16(dialPort), TLSConfig: nil},
		}
	}
	// Proxies can lack named prepared statement support. Exec mode avoids the statement cache.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	return config, nil
}

func openPostgresConnection(
	ctx context.Context, profile cfg.Profile, password string,
) (*pgx.Conn, error) {
	config, err := buildPostgresConfig(profile, password)
	if err != nil {
		return nil, err
	}
	return pgx.ConnectConfig(ctx, config)
}

// keepJSONFieldOrder returns raw JSON bytes from the driver. The default map decoder loses field order.
func keepJSONFieldOrder(connection *pgx.Conn) {
	unmarshal := func(data []byte, target any) error {
		held, isAny := target.(*any)
		if !isAny {
			return json.Unmarshal(data, target)
		}
		// The driver reuses its input buffer; the result requires a copy.
		*held = json.RawMessage(bytes.Clone(data))
		return nil
	}
	types := connection.TypeMap()
	types.RegisterType(&pgtype.Type{
		Name: "json", OID: pgtype.JSONOID,
		Codec: &pgtype.JSONCodec{Marshal: json.Marshal, Unmarshal: unmarshal},
	})
	types.RegisterType(&pgtype.Type{
		Name: "jsonb", OID: pgtype.JSONBOID,
		Codec: &pgtype.JSONBCodec{Marshal: json.Marshal, Unmarshal: unmarshal},
	})
}

// readTypeNames returns server type names by OID, including custom enums, domains, and composites.
func readTypeNames(ctx context.Context, connection *pgx.Conn) (map[uint32]string, error) {
	rows, err := connection.Query(ctx, "select oid, typname from pg_type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := map[uint32]string{}
	for rows.Next() {
		var oid uint32
		var name string
		if scanErr := rows.Scan(&oid, &name); scanErr != nil {
			return nil, scanErr
		}
		names[oid] = name
	}
	return names, rows.Err()
}
