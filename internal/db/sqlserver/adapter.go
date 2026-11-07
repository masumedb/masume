package sqlserver

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/sqlbase"
	"github.com/masumedb/masume/internal/query/syntax"
)

// sqlserverSession is one session on a SQL Server.
type sqlserverSession struct {
	sqlbase.Session

	// True where the connection may read the statement counters of the server.
	holdsStatementStats bool
	// The session id of the user connection.
	spid atomic.Int64
}

// ReadBackendID returns the session id of the user connection.
func (session *sqlserverSession) ReadBackendID() (int64, bool) {
	held := session.spid.Load()
	return held, held > 0
}

// readSessionID returns the session id of the connection, or zero.
func readSessionID(ctx context.Context, connection *sql.Conn) int64 {
	held := int64(0)
	if connection.QueryRowContext(ctx, "select @@spid as spid").Scan(&held) != nil {
		return 0
	}
	return held
}

// Capabilities returns engine support plus statement statistics availability from the
// connected server.
func (session *sqlserverSession) Capabilities() core.Capabilities {
	held := session.Support.Capabilities
	held.ReportsStatementStats = session.holdsStatementStats
	return held
}

// readServerTransaction reads the transaction state of the server back after a failure.
// An uncommittable transaction takes no further statement, and a statement of its own can
// have rolled the whole transaction back.
func (session *sqlserverSession) readServerTransaction(ctx context.Context) {
	if session.Transaction.ReadState() == db.TransactionNone {
		return
	}
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), sqlbase.StateWait)
	defer stop()

	var state int64
	if session.Connection.QueryRowContext(
		held, "select xact_state() as state").Scan(&state) != nil {
		return
	}
	switch state {
	case transactionUncommittable:
		session.Transaction.MarkFailed()
	case transactionNone:
		session.Transaction.WriteState(db.TransactionNone)
	}
}

// The states xact_state() returns.
const (
	transactionUncommittable = -1
	transactionNone          = 0
)

func (session *sqlserverSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	giveBack, waitErr := session.MainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.ReopenBrokenConnection(ctx); err != nil {
		return db.QueryResult{}, err
	}
	defer session.MarkConnectionUsed()

	flavour := session.Support.Dialect.Syntax
	lastStatement := db.ReadLastStatement(sql, flavour)
	command := syntax.ReadCommandWord(lastStatement, flavour)
	writes := db.IsWriteCommand(command) && !HoldsOutputClause(lastStatement, flavour)
	batch := sql
	if writes {
		// The count of a write belongs to the batch that wrote it: a later batch reads
		// the count of its own last statement.
		batch += rowCountProbe
	}

	answered, err := session.Connection.QueryContext(ctx, batch, params...)
	if err != nil {
		session.RecordFailure(ctx, err)
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = answered.Close() }()

	// The last result set of the batch is the one the pane shows.
	read, columns, readErr := readLastResultSet(answered, db.ReadOverscanRowLimit(rowLimit))
	if readErr != nil {
		session.RecordFailure(ctx, readErr)
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(readErr), readErr)
	}

	result := db.QueryResult{
		Elapsed: time.Since(startedAt), Command: strings.ToUpper(command),
	}
	if writes {
		result.Affected, result.HasAffected = readAffected(read)
	} else {
		result = db.BuildCappedResult(db.CappedRead{
			Rows: read, RowLimit: rowLimit, Columns: columns,
			Elapsed: result.Elapsed, Command: result.Command,
		})
	}
	session.MarkTransactionFromStatement(sql)
	return result, nil
}

// rowCountProbe is the statement that reads the count of the write before it.
const rowCountProbe = "\n;select @@rowcount as affected"

// readAffected reads the count of a write out of the rows the probe answered.
func readAffected(rows [][]any) (int64, bool) {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, false
	}
	return db.ReadNonNegativeCount(rows[0][0]), true
}

func (session *sqlserverSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect)
}

func (session *sqlserverSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement asks the server to compile the statement without running it. The server
// returns the fault as a row, so no statement of the client is refused for it.
func (session *sqlserverSession) CheckStatement(
	ctx context.Context, sql string,
) (db.StatementProblem, bool) {
	if strings.TrimSpace(sql) == "" ||
		db.HoldsSeveralCommands(sql, session.Support.Dialect.Syntax) {
		return db.StatementProblem{}, false
	}
	rows, _, err := session.ReadNamedRows(ctx, describeStatementSQL, sql)
	if err != nil || len(rows) == 0 {
		return db.StatementProblem{}, false
	}
	return ReadStatementProblem(sql, rows)
}

// ExplainQuery returns the plan the server writes as rows. SHOWPLAN_ALL estimates the plan,
// and STATISTICS PROFILE runs the statement and counts the rows of every step.
func (session *sqlserverSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	if err := db.RefuseSeveralPlans(sql, session.Support.Dialect.Syntax); err != nil {
		return db.QueryPlan{}, err
	}
	giveBack, waitErr := session.MainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.ReopenBrokenConnection(ctx); err != nil {
		return db.QueryPlan{}, err
	}
	defer session.MarkConnectionUsed()

	setting := planSetting(analyze)
	if _, err := session.Connection.ExecContext(ctx, "set "+setting+" on"); err != nil {
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer session.stopPlanSetting(ctx, setting)

	rows, err := session.Connection.QueryContext(ctx, sql)
	if err != nil {
		session.RecordFailure(ctx, err)
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = rows.Close() }()

	read, columns, readErr := readLastResultSet(rows, -1)
	if readErr != nil {
		session.readServerTransaction(ctx)
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(readErr), readErr)
	}

	plan, built := BuildPlan(db.QueryResult{Columns: columns, Rows: read}, analyze)
	if !built {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

// stopPlanSetting turns the plan setting off again, whatever the statement did. The setting
// stays on the connection until it does.
func (session *sqlserverSession) stopPlanSetting(ctx context.Context, setting string) {
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), sqlbase.StateWait)
	defer stop()
	if _, err := session.Connection.ExecContext(held, "set "+setting+" off"); err != nil {
		session.DropConnection(ctx)
	}
}

func (session *sqlserverSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	rows, _, err := session.ReadNamedRows(ctx, listActivitySQL)
	if err != nil {
		return nil, err
	}
	activity := make([]db.Activity, 0, len(rows))
	for _, row := range rows {
		activity = append(activity, db.Activity{
			PID: db.ReadNonNegativeCount(row["pid"]), User: db.ReadAnyText(row["user"]),
			ApplicationName: db.ReadAnyText(row["application_name"]),
			ClientAddress:   db.ReadAnyText(row["client_address"]),
			State:           db.ReadAnyText(row["state"]),
			Duration: time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) *
				time.Millisecond,
			Query: db.ReadAnyText(row["query"]),
		})
	}
	return activity, nil
}

// ListLockWaits returns every session waiting for a lock another session holds.
func (session *sqlserverSession) ListLockWaits(ctx context.Context) ([]db.LockWait, error) {
	rows, _, err := session.ReadNamedRows(ctx, listLockWaitsSQL)
	if err != nil {
		return nil, err
	}
	waits := make([]db.LockWait, 0, len(rows))
	for _, row := range rows {
		waits = append(waits, db.LockWait{
			BlockedPID:   db.ReadNonNegativeCount(row["blocked_pid"]),
			BlockedQuery: db.ReadAnyText(row["blocked_query"]),
			Waiting: time.Duration(db.ReadNonNegativeCount(row["waiting_ms"])) *
				time.Millisecond,
			Mode:          db.ReadAnyText(row["mode"]),
			Relation:      db.ReadAnyText(row["relation"]),
			BlockingPID:   db.ReadNonNegativeCount(row["blocking_pid"]),
			BlockingQuery: db.ReadAnyText(row["blocking_query"]),
			BlockingFor: time.Duration(db.ReadNonNegativeCount(row["blocking_ms"])) *
				time.Millisecond,
		})
	}
	return waits, nil
}

// ReadServerLoad returns the connections of the server, its connection limit and the time
// it started.
func (session *sqlserverSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
	rows, _, err := session.ReadNamedRows(ctx, readServerLoadSQL)
	if err != nil {
		return db.ServerLoad{}, err
	}
	if len(rows) == 0 {
		return db.ServerLoad{}, nil
	}
	load := db.ServerLoad{
		Connections:    db.ReadNonNegativeCount(rows[0]["connections"]),
		MaxConnections: db.ReadNonNegativeCount(rows[0]["max_connections"]),
	}
	if started, isTime := rows[0]["started_at"].(time.Time); isTime {
		load.StartedAt = started
	}
	return load, nil
}

// ListSlowStatements returns the statements the server spent the most time in, the slowest
// by mean time first.
func (session *sqlserverSession) ListSlowStatements(
	ctx context.Context, limit int,
) ([]db.StatementStat, error) {
	if !session.holdsStatementStats {
		return nil, db.NewUnsupportedError("report slow statements")
	}
	rows, _, err := session.ReadNamedRows(ctx, listSlowStatementsSQL, limit)
	if err != nil {
		return nil, err
	}
	stats := make([]db.StatementStat, 0, len(rows))
	for _, row := range rows {
		stats = append(stats, db.StatementStat{
			Query: db.ReadAnyText(row["query"]),
			Calls: db.ReadNonNegativeCount(row["calls"]),
			MeanTime: time.Duration(db.ReadNonNegativeCount(row["mean_us"])) *
				time.Microsecond,
			TotalTime: time.Duration(db.ReadNonNegativeCount(row["total_us"])) *
				time.Microsecond,
			Rows: db.ReadNonNegativeCount(row["rows_back"]),
		})
	}
	return stats, nil
}

// CancelBackend stops another session through the catalog connection. KILL ends the whole
// session, and T-SQL has no statement that stops one statement of it.
func (session *sqlserverSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	if !terminate {
		return false, db.NewUnsupportedError("cancel one statement of another session")
	}
	side, err := session.Side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, BuildKillStatement(pid)); execErr != nil {
		return false, execErr
	}
	return true, nil
}

// CancelRunningQuery is refused, because the driver cancels through the context.
func (session *sqlserverSession) CancelRunningQuery(context.Context) (bool, error) {
	return false, db.NewUnsupportedError("cancel a running statement")
}

// sqlserverAdapter opens a connection on a SQL Server.
type sqlserverAdapter struct {
	support db.EngineSupport
}

// NewAdapter returns the adapter of the SQL Server engine.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &sqlserverAdapter{support: support}
}

func (adapter *sqlserverAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	pool, poolErr := openSqlserverPool(profile, password)
	if poolErr != nil {
		return nil, db.WrapDatabaseMessage(
			db.BuildConnectMessage(profile, poolErr), poolErr)
	}

	// One connection retains the transaction of the user.
	connection, connectionErr := pool.Conn(ctx)
	if connectionErr != nil {
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(
			db.BuildConnectMessage(profile, connectionErr), connectionErr)
	}

	return buildSqlserverSession(sqlserverSessionParts{
		support: adapter.support, profile: profile, password: password,
		pool: pool, connection: connection,
		serverVersion:       readServerVersion(ctx, connection),
		defaultSchema:       readDefaultSchema(ctx, connection),
		holdsStatementStats: readsStatementStats(ctx, connection),
		spid:                readSessionID(ctx, connection),
	}), nil
}

// sqlserverSessionParts is what a session is built from: the user connection and the facts
// read at connect.
type sqlserverSessionParts struct {
	support             db.EngineSupport
	profile             cfg.Profile
	password            string
	pool                *sql.DB
	connection          *sql.Conn
	serverVersion       string
	defaultSchema       string
	holdsStatementStats bool
	spid                int64
}

// buildSqlserverSession returns the session of a user connection that is ready.
func buildSqlserverSession(parts sqlserverSessionParts) *sqlserverSession {
	profile, password := parts.profile, parts.password
	session := &sqlserverSession{
		Session: sqlbase.Session{
			SessionFacts: db.SessionFacts{
				Descriptor: db.SessionDescriptor{
					Profile: profile, ServerVersion: parts.serverVersion,
					DefaultSchema: parts.defaultSchema,
				},
				Support: parts.support,
			},
			Connection: parts.connection, Pool: parts.pool,
			Side: db.NewSideConnection(func() (*sql.DB, error) {
				return openSqlserverPool(profile, password)
			}, nil),
			MainQueue: db.NewCallQueue(),
		},
		holdsStatementStats: parts.holdsStatementStats,
	}
	session.spid.Store(parts.spid)
	session.MarkConnectionUsed()
	session.Hooks = sqlbase.Hooks{
		BeginStatement: "begin transaction",
		PrepareConnection: func(ctx context.Context, opened *sql.Conn) error {
			session.spid.Store(readSessionID(ctx, opened))
			return nil
		},
		ReportFailure: func(ctx context.Context, _ error) {
			session.readServerTransaction(ctx)
		},
		WrapTransactionError: wrapFailure,
		WrapStatementError:   wrapFailure,
		ReadStreamColumns:    readStreamColumns,
	}
	return session
}

// OpenSibling opens one more session to the same server. The new connection reads only its
// session id. The server version, the default schema and the statement counter check come
// from this session.
func (session *sqlserverSession) OpenSibling(ctx context.Context, password string) (db.Session, error) {
	profile := session.Descriptor.Profile
	pool, err := openSqlserverPool(profile, password)
	if err != nil {
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, err), err)
	}
	connection, connectionErr := pool.Conn(ctx)
	if connectionErr != nil {
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(
			db.BuildConnectMessage(profile, connectionErr), connectionErr)
	}
	return buildSqlserverSession(sqlserverSessionParts{
		support: session.Support, profile: profile, password: password,
		pool: pool, connection: connection,
		serverVersion:       session.Descriptor.ServerVersion,
		defaultSchema:       session.Descriptor.DefaultSchema,
		holdsStatementStats: session.holdsStatementStats,
		spid:                readSessionID(ctx, connection),
	}), nil
}

// readStreamColumns returns the columns of a streamed result and the reader of one row.
func readStreamColumns(
	rows *sql.Rows,
) ([]db.ResultColumn, func(*sql.Rows) ([]any, error), error) {
	columns, reader, err := buildSqlserverColumns(rows)
	return columns, reader.readRow, err
}

// readServerVersion returns the version of the server, as it writes it itself.
func readServerVersion(ctx context.Context, connection *sql.Conn) string {
	var written string
	if connection.QueryRowContext(ctx,
		"select cast(serverproperty('productversion') as nvarchar(128)) as version").
		Scan(&written) != nil || written == "" {
		return "unknown"
	}
	return written
}

// readDefaultSchema returns the schema the connection reads an unqualified name in.
func readDefaultSchema(ctx context.Context, connection *sql.Conn) string {
	var written string
	if connection.QueryRowContext(
		ctx, "select schema_name() as name").Scan(&written) != nil || written == "" {
		return "dbo"
	}
	return written
}

// readsStatementStats is true where this connection may read the statement counters of the
// server, which needs the VIEW SERVER STATE permission.
func readsStatementStats(ctx context.Context, connection *sql.Conn) bool {
	var counted int64
	return connection.QueryRowContext(ctx,
		"select count_big(*) as held from sys.dm_exec_query_stats").Scan(&counted) == nil
}

// Compile-time Session interface check.
var _ db.Session = (*sqlserverSession)(nil)
