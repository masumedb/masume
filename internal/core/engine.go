package core

import (
	"slices"
	"strings"
)

// Engine is a supported database engine.
type Engine string

// The supported servers. Each one has an entry in the registry below.
const (
	EnginePostgres       Engine = "postgres"
	EngineMysql          Engine = "mysql"
	EngineSqlite         Engine = "sqlite"
	EngineRedis          Engine = "redis"
	EngineTurso          Engine = "turso"
	EngineCockroach      Engine = "cockroach"
	EngineTimescale      Engine = "timescale"
	EngineRedshift       Engine = "redshift"
	EngineNeon           Engine = "neon"
	EngineSupabase       Engine = "supabase"
	EngineAuroraPostgres Engine = "aurora-postgres"
	EngineMariadb        Engine = "mariadb"
	EngineTidb           Engine = "tidb"
	EnginePlanetscale    Engine = "planetscale"
	EngineAuroraMysql    Engine = "aurora-mysql"
	EngineSqlserver      Engine = "sqlserver"
	EngineClickhouse     Engine = "clickhouse"
	EngineMongo          Engine = "mongodb"
)

// Engines lists every engine, in the order used by the docs.
var Engines = []Engine{
	EnginePostgres, EngineMysql, EngineSqlite, EngineRedis,
	EngineCockroach, EngineTimescale, EngineRedshift, EngineNeon, EngineSupabase, EngineAuroraPostgres,
	EngineMariadb, EngineTidb, EnginePlanetscale, EngineAuroraMysql,
	EngineTurso,
	EngineSqlserver,
	EngineClickhouse,
	EngineMongo,
}

// DefaultEngine is the engine used when a profile does not name one.
const DefaultEngine = EnginePostgres

// Family is the database protocol shared by an adapter and dialect.
type Family string

// The supported database protocols.
const (
	FamilyPostgres   Family = "postgres"
	FamilyMysql      Family = "mysql"
	FamilySqlite     Family = "sqlite"
	FamilySqlserver  Family = "sqlserver"
	FamilyClickhouse Family = "clickhouse"
	FamilyRedis      Family = "redis"
	FamilyMongo      Family = "mongo"
)

// Capabilities is the set of supported engine operations.
type Capabilities struct {
	PlansStatement bool
	MeasuresPlan   bool
	// True if planning includes statements such as DROP.
	PlansEveryStatement bool
	HasServerSessions   bool
	// True if the server reports lock waits between sessions.
	ReportsLockWaits bool
	// True if the server reports load statistics.
	ReportsServerLoad bool
	// True if statement statistics are available. The connected session sets this capability.
	ReportsStatementStats bool
	// A cancel needs a second connection to the same server.
	CancelsRunningQuery bool
	HasTransactions     bool
	// True if the engine supports sorting read results.
	SortsRead      bool
	TruncatesTable bool
	WritesDDL      bool
	// True if SQL writes support affected row and relation previews.
	PlansWrites bool
	// True if a connection supports read-only mode. TiDB accepts the statement but does not enforce the mode.
	TakesReadOnlyMode bool
	// True if staged changes support atomic application. The connected deployment can change this capability.
	AppliesChangesTogether bool
	// True if the server joins tables in one statement, which the query builder writes.
	JoinsTables bool
}

// EngineInfo is the engine metadata available before connection. Query support adds the dialect and language.
type EngineInfo struct {
	Engine        Engine
	Family        Family
	Capabilities  Capabilities
	DefaultPort   int
	OpensFile     bool
	NeedsUser     bool
	NeedsPassword bool
	// True where the password belongs to the server and not to a named user, so a profile
	// without a user can still have a password. Redis is the only one: `requirepass`
	// has no user.
	PasswordWithoutUser bool
	// True where a connection needs the name of one database. A MySQL-protocol server
	// connects without one.
	NeedsDatabase  bool
	URLSchemes     []string
	DefaultSSLMode SSLMode
	// The full names of the schemas this server reserves for itself.
	SystemSchemas []string
	// The name prefixes of the schemas this server creates for itself.
	SystemSchemaPrefixes []string
}

// The catalog schemas of every PostgreSQL-protocol server.
var postgresCatalogSchemas = []string{"pg_catalog", "information_schema"}

// The name prefixes of the schemas a PostgreSQL server creates for itself.
var postgresOwnPrefixes = []string{"pg_toast", "pg_temp"}

// The schemas a SQL Server database holds for itself, in every database it serves.
var sqlserverSystemSchemas = []string{
	"sys", "information_schema", "guest",
	"db_owner", "db_accessadmin", "db_securityadmin", "db_ddladmin",
	"db_backupoperator", "db_datareader", "db_datawriter",
	"db_denydatareader", "db_denydatawriter",
}

// The databases every MySQL-protocol server reserves for itself.
var mysqlSystemSchemas = []string{"mysql", "information_schema", "performance_schema", "sys"}

var postgresCapabilities = Capabilities{
	PlansStatement:      true,
	MeasuresPlan:        true,
	PlansEveryStatement: false,
	HasServerSessions:   true,
	ReportsLockWaits:    true,
	ReportsServerLoad:   true,
	CancelsRunningQuery: true,
	HasTransactions:     true,
	SortsRead:           true,
	TruncatesTable:      true,
	WritesDDL:           true,
	PlansWrites:         true,
	TakesReadOnlyMode:   true,
	JoinsTables:         true,

	AppliesChangesTogether: true,
}

var mysqlCapabilities = withPostgres(func(capabilities *Capabilities) {
	// MySQL lock waits require performance_schema queries that this client does not implement.
	capabilities.ReportsLockWaits = false
})

var sqliteCapabilities = Capabilities{
	// SQLite plans a statement, but it does not measure the run.
	PlansStatement: true,
	MeasuresPlan:   false,
	// SQLite has no server session list.
	HasServerSessions:   false,
	CancelsRunningQuery: false,
	HasTransactions:     true,
	SortsRead:           true,
	// SQLite empties a table with a delete of every row.
	TruncatesTable:         false,
	WritesDDL:              true,
	PlansWrites:            true,
	TakesReadOnlyMode:      true,
	JoinsTables:            true,
	AppliesChangesTogether: true,
}

var sqlserverCapabilities = Capabilities{
	// SHOWPLAN_ALL estimates the plan and STATISTICS PROFILE counts the rows of every step.
	PlansStatement:      true,
	MeasuresPlan:        true,
	PlansEveryStatement: false,
	HasServerSessions:   true,
	ReportsLockWaits:    true,
	ReportsServerLoad:   true,
	// KILL ends a session. T-SQL has no statement that stops one statement, so the
	// driver cancels through the context.
	CancelsRunningQuery: false,
	HasTransactions:     true,
	SortsRead:           true,
	TruncatesTable:      true,
	WritesDDL:           true,
	PlansWrites:         true,
	// The server has no read-only session, so this client blocks the write.
	TakesReadOnlyMode:      true,
	JoinsTables:            true,
	AppliesChangesTogether: true,
}

var clickhouseCapabilities = Capabilities{
	// EXPLAIN returns the plan of a read. No plan carries a measurement of a run.
	PlansStatement:      true,
	MeasuresPlan:        false,
	PlansEveryStatement: false,
	// system.processes lists the running queries, and KILL QUERY stops one.
	HasServerSessions: true,
	// The server takes no lock a session waits for.
	ReportsLockWaits:    false,
	ReportsServerLoad:   true,
	CancelsRunningQuery: true,
	// Transactions are experimental, so this client offers none.
	HasTransactions: false,
	SortsRead:       true,
	TruncatesTable:  true,
	WritesDDL:       true,
	// A write is a mutation of its own shape, which the write plan does not read.
	PlansWrites: false,
	// `set readonly = 1` refuses every write on the server.
	TakesReadOnlyMode: true,
	JoinsTables:       true,
	// Without a transaction, every change stands on its own.
	AppliesChangesTogether: false,
}

var engineRegistry = map[Engine]EngineInfo{
	EnginePostgres: {
		Engine: EnginePostgres, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		URLSchemes:    []string{"postgres", "postgresql"},
		SystemSchemas: postgresCatalogSchemas, SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineCockroach: {
		Engine: EngineCockroach, Family: FamilyPostgres,
		Capabilities: withPostgres(func(capabilities *Capabilities) {
			// The server also plans a schema change: `explain drop table` returns a plan.
			capabilities.PlansEveryStatement = true
			// CockroachDB session IDs are strings. The server has no pg_cancel_backend.
			capabilities.HasServerSessions = false
			capabilities.CancelsRunningQuery = false
			capabilities.ReportsLockWaits = false
			capabilities.ReportsServerLoad = false
		}),
		DefaultPort: 26257, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		URLSchemes:           []string{"cockroachdb"},
		SystemSchemas:        append(append([]string{}, postgresCatalogSchemas...), "pg_extension", "crdb_internal"),
		SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineTimescale: {
		Engine: EngineTimescale, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		SystemSchemas:        postgresCatalogSchemas,
		SystemSchemaPrefixes: append(append([]string{}, postgresOwnPrefixes...), "_timescaledb_", "timescaledb_"),
	},
	EngineRedshift: {
		Engine: EngineRedshift, Family: FamilyPostgres,
		Capabilities: withPostgres(func(capabilities *Capabilities) {
			// EXPLAIN only estimates. There is no ANALYZE that measures the query.
			capabilities.MeasuresPlan = false
			// The cluster reports its locks and its load through its own `stv_` tables.
			capabilities.ReportsLockWaits = false
			capabilities.ReportsServerLoad = false
		}),
		DefaultPort: 5439, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		URLSchemes: []string{"redshift"},
		// The cluster accepts a TLS connection only.
		DefaultSSLMode:       SSLRequire,
		SystemSchemas:        append(append([]string{}, postgresCatalogSchemas...), "pg_internal", "catalog_history"),
		SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineNeon: {
		Engine: EngineNeon, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true, DefaultSSLMode: SSLRequire,
		SystemSchemas: postgresCatalogSchemas, SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineSupabase: {
		Engine: EngineSupabase, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true, DefaultSSLMode: SSLRequire,
		SystemSchemas: append(append([]string{}, postgresCatalogSchemas...),
			"auth", "storage", "realtime", "graphql", "graphql_public", "extensions",
			"vault", "supabase_functions", "supabase_migrations", "pgbouncer", "net", "cron"),
		SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineAuroraPostgres: {
		Engine: EngineAuroraPostgres, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		SystemSchemas:        postgresCatalogSchemas,
		SystemSchemaPrefixes: append(append([]string{}, postgresOwnPrefixes...), "aws_", "rds_"),
	},
	EngineMysql: {
		Engine: EngineMysql, Family: FamilyMysql, Capabilities: mysqlCapabilities,
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true,
		URLSchemes: []string{"mysql"}, SystemSchemas: mysqlSystemSchemas,
	},
	EngineMariadb: {
		Engine: EngineMariadb, Family: FamilyMysql, Capabilities: mysqlCapabilities,
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true,
		URLSchemes: []string{"mariadb"}, SystemSchemas: mysqlSystemSchemas,
	},
	EngineTidb: {
		Engine: EngineTidb, Family: FamilyMysql,
		Capabilities: withMysql(func(capabilities *Capabilities) {
			// TiDB accepts `set session transaction read only` without enforcing read-only mode.
			capabilities.TakesReadOnlyMode = false
			// The status variables of the server are its own, not the ones MySQL reports.
			capabilities.ReportsServerLoad = false
		}),
		DefaultPort: 4000, NeedsUser: true, NeedsPassword: true,
		SystemSchemas: append(append([]string{}, mysqlSystemSchemas...), "metrics_schema"),
	},
	EnginePlanetscale: {
		Engine: EnginePlanetscale, Family: FamilyMysql,
		Capabilities: withMysql(func(capabilities *Capabilities) {
			capabilities.HasServerSessions = false
			capabilities.CancelsRunningQuery = false
			capabilities.ReportsServerLoad = false
		}),
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true, DefaultSSLMode: SSLRequire,
		SystemSchemas: mysqlSystemSchemas,
	},
	EngineAuroraMysql: {
		Engine: EngineAuroraMysql, Family: FamilyMysql, Capabilities: mysqlCapabilities,
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true,
		SystemSchemas: mysqlSystemSchemas,
	},
	EngineSqlserver: {
		Engine: EngineSqlserver, Family: FamilySqlserver, Capabilities: sqlserverCapabilities,
		DefaultPort: 1433, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		URLSchemes: []string{"sqlserver", "mssql"}, SystemSchemas: sqlserverSystemSchemas,
	},
	EngineClickhouse: {
		Engine: EngineClickhouse, Family: FamilyClickhouse,
		Capabilities: clickhouseCapabilities,
		DefaultPort:  9000, NeedsUser: true, NeedsPassword: true, NeedsDatabase: true,
		URLSchemes: []string{"clickhouse"},
		// The server holds the catalog twice: once as `system`, and once as the standard
		// views under two names of one database. `default` holds tables of the user.
		SystemSchemas: []string{"system", "information_schema"},
	},
	EngineRedis: {
		Engine: EngineRedis, Family: FamilyRedis,
		Capabilities: Capabilities{
			// A command states the operation, so the server makes no plan.
			PlansStatement: false,
			MeasuresPlan:   false,
			// CLIENT LIST lists every connection, and CLIENT KILL stops one.
			HasServerSessions: true,
			// Redis runs one command at a time and completes it before the next one.
			CancelsRunningQuery: false,
			// MULTI queues the commands and returns no result before EXEC.
			HasTransactions: false,
			// A SCAN returns the keys in the order of the server only.
			SortsRead: false,
			// A prefix is only a name pattern, so no command deletes it.
			TruncatesTable: false,
			WritesDDL:      false,
			// The server has no read-only session, so this client blocks the write.
			TakesReadOnlyMode: true,
			// MULTI is not a transaction the user controls, but it does apply a staged
			// set as one unit.
			AppliesChangesTogether: true,
		},
		DefaultPort: 6379, NeedsDatabase: true,
		URLSchemes:          []string{"redis", "rediss"},
		PasswordWithoutUser: true,
	},
	EngineSqlite: {
		Engine: EngineSqlite, Family: FamilySqlite, Capabilities: sqliteCapabilities,
		// A file is opened locally, so there is no port and no URL scheme.
		DefaultPort: 0, OpensFile: true, NeedsDatabase: true,
	},
	EngineTurso: {
		Engine: EngineTurso, Family: FamilySqlite,
		Capabilities: withSqlite(func(capabilities *Capabilities) {
			// The server takes no read-only connection, so the client checks every write.
			capabilities.TakesReadOnlyMode = false
		}),
		// The database is the host name of the server, and the auth token is the password.
		DefaultPort: 443, NeedsPassword: true, DefaultSSLMode: SSLRequire,
		URLSchemes: []string{"libsql"},
	},
	EngineMongo: {
		Engine: EngineMongo, Family: FamilyMongo,
		Capabilities: Capabilities{
			// The server explains a find and an aggregate, and it can measure both.
			PlansStatement: true,
			MeasuresPlan:   true,
			// The client only requests plans for reads.
			PlansEveryStatement: false,
			// currentOp lists every running operation, and killOp stops one.
			HasServerSessions: true,
			// The driver cancels through the context. A second connection cannot find
			// the operation id of the call it would stop.
			CancelsRunningQuery: false,
			// Transactions require a replica set or sharded cluster. The connected session checks deployment support.
			HasTransactions: true,
			// A find accepts a sort, so the server sorts the page.
			SortsRead: true,
			// Emptying a collection deletes all documents.
			TruncatesTable: false,
			// Every statement is a command, so the object menu has no SQL to generate.
			WritesDDL: false,
			// The server has no read-only session, so this client blocks the write.
			TakesReadOnlyMode: true,
			// Atomic changes require transactions. The connected session checks deployment support.
			AppliesChangesTogether: true,
		},
		// A username enables authentication and password lookup. Profiles without a user omit authentication.
		NeedsPassword: true, NeedsDatabase: true,
		DefaultPort:   27017,
		URLSchemes:    []string{"mongodb"},
		SystemSchemas: []string{"admin", "config", "local"},
	},
}

func withPostgres(change func(*Capabilities)) Capabilities {
	capabilities := postgresCapabilities
	change(&capabilities)
	return capabilities
}

func withSqlite(change func(*Capabilities)) Capabilities {
	capabilities := sqliteCapabilities
	change(&capabilities)
	return capabilities
}

func withMysql(change func(*Capabilities)) Capabilities {
	capabilities := mysqlCapabilities
	change(&capabilities)
	return capabilities
}

// ResolveEngineInfo returns engine metadata, or the default engine metadata for an unknown name.
func ResolveEngineInfo(engine Engine) EngineInfo {
	info, known := engineRegistry[engine]
	if !known {
		return engineRegistry[DefaultEngine]
	}
	return info
}

// ListEngineInfo returns the properties of every engine, in the order of Engines.
func ListEngineInfo() []EngineInfo {
	listed := make([]EngineInfo, 0, len(Engines))
	for _, engine := range Engines {
		listed = append(listed, engineRegistry[engine])
	}
	return listed
}

// FindEngine parses the text as an engine name.
func FindEngine(written string) (Engine, bool) {
	return FindAllowed(Engines, strings.ToLower(strings.TrimSpace(written)))
}

// HoldsSystemSchema is true if the server reserves that schema for itself.
func (info EngineInfo) HoldsSystemSchema(schema string) bool {
	name := strings.ToLower(schema)
	if slices.Contains(info.SystemSchemas, name) {
		return true
	}
	for _, prefix := range info.SystemSchemaPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// OpensFile is true for an engine that opens a local file. It has no host, port or user.
func OpensFile(engine Engine) bool {
	return ResolveEngineInfo(engine).OpensFile
}

// NeedsUser is true if the engine connects as a named user.
func NeedsUser(engine Engine) bool {
	return ResolveEngineInfo(engine).NeedsUser
}

// NeedsDatabase is true if the engine connects to one named database.
func NeedsDatabase(engine Engine) bool {
	return ResolveEngineInfo(engine).NeedsDatabase
}

// ResolveDefaultPort returns the default port of the engine.
func ResolveDefaultPort(engine Engine) int {
	return ResolveEngineInfo(engine).DefaultPort
}
