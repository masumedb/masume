package postgres

// Flavour is the engine-specific configuration for the shared PostgreSQL adapter.
type Flavour struct {
	// PostgreSQL uses parenthesized EXPLAIN options. CockroachDB omits parentheses; Redshift supports estimates only.
	BuildExplainPrefix func(analyze bool) string
	// The statement that opens a session where the server refuses every write.
	ReadOnlyStatement string
	// Statement that stops the session with process ID $1. A row count above zero is a stop.
	BuildCancelStatement   func(terminate bool) string
	ListActivityStatement  string
	ListLockWaitsStatement string
	// SQL expression, true where the server keeps statement statistics. Empty for none.
	CountsStatementsExpression  string
	ListSlowStatementsStatement string
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

const countsPgStatStatements = `(select count(*) from pg_extension
                                   where extname = 'pg_stat_statements') > 0`

// FlavourStandard is PostgreSQL itself, which the other flavours differ from.
var FlavourStandard = Flavour{
	CountsStatementsExpression:  countsPgStatStatements,
	ListSlowStatementsStatement: listSlowStatementsSQL,
	BuildExplainPrefix: func(analyze bool) string {
		if analyze {
			return "explain (ANALYZE, BUFFERS, COSTS)"
		}
		return "explain (COSTS)"
	},
	ReadOnlyStatement:      postgresReadOnlyStatement,
	BuildCancelStatement:   buildPostgresCancelStatement,
	ListActivityStatement:  listActivitySQL,
	ListLockWaitsStatement: listLockWaitsSQL,
}

// FlavourCockroach writes its own plan, and takes no options in brackets.
var FlavourCockroach = Flavour{
	CountsStatementsExpression:  "true",
	ListSlowStatementsStatement: listCockroachSlowStatementsSQL,
	BuildExplainPrefix: func(analyze bool) string {
		if analyze {
			return "explain analyze"
		}
		return "explain"
	},
	ReadOnlyStatement:      postgresReadOnlyStatement,
	BuildCancelStatement:   buildCockroachCancelStatement,
	ListActivityStatement:  listCockroachActivitySQL,
	ListLockWaitsStatement: listCockroachLockWaitsSQL,
}

// FlavourRedshift plans without measuring, so it is only asked for the estimate.
var FlavourRedshift = Flavour{
	BuildExplainPrefix:     func(bool) string { return "explain" },
	ReadOnlyStatement:      postgresReadOnlyStatement,
	BuildCancelStatement:   buildPostgresCancelStatement,
	ListActivityStatement:  listActivitySQL,
	ListLockWaitsStatement: listLockWaitsSQL,
}

// MysqlFlavour holds the parts each MySQL-protocol server does differently. They share
