package postgres

// Flavour is the engine-specific configuration for the shared PostgreSQL adapter.
type Flavour struct {
	// PostgreSQL uses parenthesized EXPLAIN options. CockroachDB omits parentheses; Redshift supports estimates only.
	BuildExplainPrefix func(analyze bool) string
	// The statement that opens a session where the server refuses every write.
	ReadOnlyStatement string
	// Statement that stops the session with process ID $1. A row count above zero is a stop.
	BuildCancelStatement func(terminate bool) string
	// True if pg_extension is available. Redshift lacks this catalog.
	HasExtensionCatalog bool
}

const postgresReadOnlyStatement = "set default_transaction_read_only = on"

func buildPostgresCancelStatement(terminate bool) string {
	if terminate {
		return "select 1 where pg_terminate_backend($1)"
	}
	return "select 1 where pg_cancel_backend($1)"
}

func buildCockroachCancelStatement(terminate bool) string {
	if terminate {
		return `cancel sessions if exists (
		          select session_id from crdb_internal.cluster_sessions where pg_backend_pid = $1)`
	}
	return `cancel queries if exists (
	          select query.query_id
	            from crdb_internal.cluster_queries query
	            join crdb_internal.cluster_sessions session on session.session_id = query.session_id
	           where session.pg_backend_pid = $1)`
}

// FlavourStandard is PostgreSQL itself, which the other flavours differ from.
var FlavourStandard = Flavour{
	HasExtensionCatalog: true,
	BuildExplainPrefix: func(analyze bool) string {
		if analyze {
			return "explain (ANALYZE, BUFFERS, COSTS)"
		}
		return "explain (COSTS)"
	},
	ReadOnlyStatement:    postgresReadOnlyStatement,
	BuildCancelStatement: buildPostgresCancelStatement,
}

// FlavourCockroach writes its own plan, and takes no options in brackets.
var FlavourCockroach = Flavour{
	HasExtensionCatalog: true,
	BuildExplainPrefix: func(analyze bool) string {
		if analyze {
			return "explain analyze"
		}
		return "explain"
	},
	ReadOnlyStatement:    postgresReadOnlyStatement,
	BuildCancelStatement: buildCockroachCancelStatement,
}

// FlavourRedshift plans without measuring, so it is only asked for the estimate.
var FlavourRedshift = Flavour{
	BuildExplainPrefix:   func(bool) string { return "explain" },
	ReadOnlyStatement:    postgresReadOnlyStatement,
	BuildCancelStatement: buildPostgresCancelStatement,
}

// MysqlFlavour holds the parts each MySQL-protocol server does differently. They share
