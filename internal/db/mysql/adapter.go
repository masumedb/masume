package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/sqlbase"
	"github.com/masumedb/masume/internal/query/statement"
	"github.com/masumedb/masume/internal/query/syntax"
)

type mysqlSession struct {
	db.NoSequences
	db.NoServerLoad
	sqlbase.Session

	flavour  Flavour
	threadID int64
	profile  cfg.Profile
	password string
	// The row cap of the connection, kept so it is not set twice.
	selectLimit    int
	hasSelectLimit bool
	// True if innodb_rollback_on_timeout enables full rollback after a lock timeout.
	rollsBackOnTimeout bool
}

// markTransactionFailed records full transaction rollback errors. Ordinary MySQL errors leave the transaction open.
// Lock timeouts roll back only the statement unless innodb_rollback_on_timeout is enabled. BEGIN commits an existing MySQL transaction.
func (session *mysqlSession) markTransactionFailed(err error) {
	var reported *driver.MySQLError
	if !errors.As(err, &reported) {
		return
	}
	if reported.Number == mysqlDeadlock ||
		(reported.Number == mysqlLockTimeout && session.rollsBackOnTimeout) {
		session.Transaction.MarkFailed()
	}
}

// stopAbandonedStatement stops the statement of the user connection on the server.
func (session *mysqlSession) stopAbandonedStatement(ctx context.Context) {
	statement := session.flavour.BuildKillStatement(session.threadID, false)
	if session.threadID <= 0 || statement == "" {
		return
	}
	side, err := session.Side.Read()
	if err != nil {
		return
	}
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), sqlbase.StateWait)
	defer stop()
	_, _ = side.ExecContext(held, statement)
}

// isDriverBroken is true for the error of a connection the driver found unusable. The driver
// closes the socket of a cancelled statement, and the server runs that statement on until a
// KILL QUERY stops it.
func isDriverBroken(err error) bool {
	return errors.Is(err, driver.ErrInvalidConn)
}

// prepareReopenedConnection prepares the user connection opened again after a failure.
func (session *mysqlSession) prepareReopenedConnection(
	ctx context.Context, opened *sql.Conn,
) error {
	threadID, err := prepareUserConnection(ctx, opened, session.profile, session.flavour)
	if err != nil {
		return err
	}
	session.threadID = threadID
	session.hasSelectLimit = false
	return nil
}

// resolveSelectLimit returns the session row cap for a statement. A statement whose SELECT
// feeds a write runs without the cap.
func resolveSelectLimit(sql string, flavour syntax.SyntaxFlavour, rowLimit int) int {
	if !statement.ReturnsRowsToClient(sql, flavour) {
		return -1
	}
	return db.ReadOverscanRowLimit(rowLimit)
}

// applySelectLimit sets the session row limit.
func (session *mysqlSession) applySelectLimit(ctx context.Context, cap int) error {
	if session.hasSelectLimit && session.selectLimit == cap {
		return nil
	}
	written := "default"
	if cap >= 0 {
		if cap < 1 {
			cap = 1
		}
		written = strconv.Itoa(cap)
	}
	if _, err := session.Connection.ExecContext(
		ctx, "set session sql_select_limit = "+written); err != nil {
		session.MarkBroken(ctx, err)
		return err
	}
	session.selectLimit = cap
	session.hasSelectLimit = true
	return nil
}

// buildMysqlColumns reads result column names, types, and binary flags.
func buildMysqlColumns(rows *sql.Rows) ([]db.ResultColumn, []bool, error) {
	names, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, nil
	}
	// ColumnTypes names the scan type used to read bytes as hex.
	types, typeErr := rows.ColumnTypes()
	if typeErr != nil {
		return nil, nil, typeErr
	}

	columns := make([]db.ResultColumn, 0, len(names))
	// Binary column values remain bytes for hexadecimal display.
	binary := make([]bool, 0, len(names))
	for at, name := range names {
		dataType := ""
		if at < len(types) && types[at] != nil {
			dataType = strings.ToLower(types[at].DatabaseTypeName())
		}
		columns = append(columns, db.ResultColumn{Name: name, DataType: dataType})
		binary = append(binary, db.IsBinaryColumnType(dataType))
	}
	return columns, binary, nil
}

// readMysqlRows reads a result as rows of plain values, with its columns.
func readMysqlRows(rows *sql.Rows, cap int) ([][]any, []db.ResultColumn, error) {
	columns, binary, err := buildMysqlColumns(rows)
	if err != nil || columns == nil {
		return nil, nil, err
	}

	read := [][]any{}
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(columns), binary...)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		read = append(read, values)
		if cap >= 0 && len(read) >= cap {
			break
		}
	}
	return read, columns, rows.Err()
}

func (session *mysqlSession) RunQuery(
	ctx context.Context, statement string, rowLimit int, params []any,
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
	flavour := session.Support.Dialect.Syntax
	if err := session.applySelectLimit(
		ctx, resolveSelectLimit(statement, flavour, rowLimit)); err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}

	lastStatement := db.ReadLastStatement(statement, flavour)
	command := syntax.ReadCommandWord(lastStatement, flavour)
	rows, err := session.Connection.QueryContext(ctx, statement, params...)
	if err != nil {
		session.RecordFailure(ctx, err)
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	defer func() { _ = rows.Close() }()

	// The driver exposes only row results. The final result here belongs to the last row-returning statement.
	read, columns, readErr := readMysqlRows(rows, db.ReadOverscanRowLimit(rowLimit))
	if readErr != nil {
		session.RecordFailure(ctx, readErr)
		return db.QueryResult{}, db.WrapDatabaseError(readErr)
	}
	for rows.NextResultSet() {
		next, nextColumns, nextErr := readMysqlRows(rows, db.ReadOverscanRowLimit(rowLimit))
		if nextErr != nil {
			session.RecordFailure(ctx, nextErr)
			return db.QueryResult{}, db.WrapDatabaseError(nextErr)
		}
		read, columns = next, nextColumns
	}

	writes := db.IsWriteCommand(command)
	if writes && !db.HoldsReturningClause(lastStatement, flavour) {
		// A final write uses its affected count instead of an earlier statement's rows.
		read, columns = nil, nil
	}
	result := db.BuildCappedResult(db.CappedRead{
		Rows: read, RowLimit: rowLimit, Columns: columns,
		Elapsed: time.Since(startedAt), Command: strings.ToUpper(command),
	})
	if writes {
		result.Affected, result.HasAffected = session.countAffected(ctx)
	}
	session.MarkTransactionFromStatement(statement)
	return result, nil
}

// countAffected returns the last write count and distinguishes an unavailable count from zero.
func (session *mysqlSession) countAffected(ctx context.Context) (int64, bool) {
	row := session.Connection.QueryRowContext(ctx, "select row_count() as changed")
	var changed int64
	if row.Scan(&changed) != nil {
		return 0, false
	}
	if changed < 0 {
		return 0, true
	}
	return changed, true
}

func (session *mysqlSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect)
}

func (session *mysqlSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement prepares the statement, which makes the server parse it and resolve
// every name in it.
func (session *mysqlSession) CheckStatement(
	ctx context.Context, statement string,
) (db.StatementProblem, bool) {
	if session.Transaction.ReadState() != db.TransactionNone {
		return db.StatementProblem{}, false
	}
	if strings.TrimSpace(statement) == "" ||
		db.HoldsSeveralCommands(statement, session.Support.Dialect.Syntax) {
		return db.StatementProblem{}, false
	}

	side, err := session.Side.Read()
	if err != nil {
		return db.StatementProblem{}, false
	}
	prepared, prepareErr := side.PrepareContext(ctx, statement)
	if prepareErr == nil {
		_ = prepared.Close()
		return db.StatementProblem{}, false
	}

	var reported *driver.MySQLError
	if !errors.As(prepareErr, &reported) || reported.Message == "" {
		// A broken connection reports no server message, and the statement is not at fault.
		return db.StatementProblem{}, false
	}
	if reported.Number == mysqlCannotPrepare || isUnfinishedMysqlStatement(reported.Message) {
		return db.StatementProblem{}, false
	}
	problem := db.StatementProblem{Message: reported.Message}
	if offset, found := findMysqlOffset(statement, reported.Message); found {
		problem.Offset = offset
		problem.HasOffset = true
	}
	return problem, true
}

// ExplainQuery returns the plan as the server writes it, asked for in the form this
// server takes.
func (session *mysqlSession) ExplainQuery(
	ctx context.Context, statement string, analyze bool,
) (db.QueryPlan, error) {
	if err := db.RefuseSeveralPlans(statement, session.Support.Dialect.Syntax); err != nil {
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
	// Planning requires the original statement without the session row cap.
	if err := session.applySelectLimit(ctx, -1); err != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(err)
	}

	// An analyzed plan runs the statement, so it uses the connection of the user.
	rows, err := session.Connection.QueryContext(
		ctx, session.flavour.BuildExplainStatement(statement, analyze))
	if err != nil {
		session.RecordFailure(ctx, err)
		return db.QueryPlan{}, db.WrapDatabaseError(err)
	}
	defer func() { _ = rows.Close() }()

	read, columns, readErr := readMysqlRows(rows, -1)
	if readErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(readErr)
	}
	plan, built := session.flavour.ReadPlan(
		db.QueryResult{Columns: columns, Rows: read}, analyze)
	if !built {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

func (session *mysqlSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	if !session.Support.Capabilities.HasServerSessions {
		return nil, db.NewUnsupportedError("list sessions")
	}
	rows, _, err := session.ReadNamedRows(ctx, listMysqlActivitySQL)
	if err != nil {
		return nil, err
	}
	activity := make([]db.Activity, 0, len(rows))
	for _, row := range rows {
		activity = append(activity, db.Activity{
			PID: db.ReadNonNegativeCount(row["pid"]), User: db.ReadAnyText(row["user"]),
			ApplicationName: db.ReadAnyText(row["db"]), ClientAddress: db.ReadAnyText(row["host"]),
			State:    db.ReadAnyText(row["command"]),
			Duration: time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) * time.Millisecond,
			Query:    db.ReadAnyText(row["query"]),
		})
	}
	return activity, nil
}

func (session *mysqlSession) ListLockWaits(ctx context.Context) ([]db.LockWait, error) {
	if !session.Support.Capabilities.ReportsLockWaits || session.flavour.ListLockWaitsStatement == "" {
		return nil, db.NewUnsupportedError("report lock waits")
	}
	rows, _, err := session.ReadNamedRows(ctx, session.flavour.ListLockWaitsStatement)
	if err != nil {
		return nil, err
	}
	waits := make([]db.LockWait, 0, len(rows))
	for _, row := range rows {
		waits = append(waits, db.LockWait{
			BlockedPID:    db.ReadNonNegativeCount(row["blocked_pid"]),
			BlockedQuery:  db.ReadAnyText(row["blocked_query"]),
			Waiting:       time.Duration(db.ReadNonNegativeCount(row["waiting_ms"])) * time.Millisecond,
			Mode:          db.ReadAnyText(row["mode"]),
			Relation:      db.ReadAnyText(row["relation"]),
			BlockingPID:   db.ReadNonNegativeCount(row["blocking_pid"]),
			BlockingQuery: db.ReadAnyText(row["blocking_query"]),
			BlockingFor:   time.Duration(db.ReadNonNegativeCount(row["blocking_ms"])) * time.Millisecond,
		})
	}
	return waits, nil
}

// ReadServerLoad returns load statistics and calculates server start time from uptime.
func (session *mysqlSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
	if !session.Support.Capabilities.ReportsServerLoad {
		return db.ServerLoad{}, db.NewUnsupportedError("report server load")
	}
	rows, _, err := session.ReadNamedRows(ctx, readMysqlServerLoadSQL)
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
	if seconds := db.ReadNonNegativeCount(rows[0]["uptime_seconds"]); seconds > 0 {
		load.StartedAt = time.Now().Add(-time.Duration(seconds) * time.Second)
	}
	return load, nil
}

// CancelBackend stops another session through the catalog connection.
func (session *mysqlSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	statement := session.flavour.BuildKillStatement(pid, terminate)
	if statement == "" {
		return false, db.NewUnsupportedError("stop another session")
	}
	side, err := session.Side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, statement); execErr != nil {
		return false, execErr
	}
	return true, nil
}

// CancelRunningQuery needs a second connection, because the busy one cannot answer.
func (session *mysqlSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	if !session.Support.Capabilities.CancelsRunningQuery {
		return false, db.NewUnsupportedError("cancel a running statement")
	}
	if session.threadID <= 0 {
		return false, db.NewDatabaseError(
			"cannot cancel the statement: the connection ID is unavailable")
	}
	statement := session.flavour.BuildKillStatement(session.threadID, false)
	if statement == "" {
		return false, db.NewUnsupportedError("cancel a running statement")
	}
	side, err := session.Side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, statement); execErr != nil {
		return false, execErr
	}
	return true, nil
}

// mysqlAdapter opens a connection on a MySQL-protocol server.
type mysqlAdapter struct {
	support db.EngineSupport
	flavour Flavour
}

// NewAdapter returns the adapter of one MySQL-protocol engine.
func NewAdapter(support db.EngineSupport, flavour Flavour) db.Adapter {
	return &mysqlAdapter{support: support, flavour: flavour}
}

func (adapter *mysqlAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	pool, err := openMysqlPool(profile, password)
	if err != nil {
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, err), err)
	}

	// One connection retains the session transaction and row limit.
	connection, connectionErr := pool.Conn(ctx)
	if connectionErr != nil {
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, connectionErr), connectionErr)
	}

	fail := func(reason error) (db.Session, error) {
		_ = connection.Close()
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), reason)
	}

	// Read-only profiles require server support.
	if profile.AccessMode == cfg.AccessReadOnly && adapter.flavour.ReadOnlyStatement == "" {
		return fail(errors.New("this server does not support read-only sessions"))
	}
	threadID, prepareErr := prepareUserConnection(ctx, connection, profile, adapter.flavour)
	if prepareErr != nil {
		return fail(prepareErr)
	}

	serverVersion := "unknown"
	var written string
	if connection.QueryRowContext(
		ctx, "select version() as version").Scan(&written) == nil && written != "" {
		serverVersion = written
	}

	session := &mysqlSession{
		rollsBackOnTimeout: readRollsBackOnTimeout(ctx, connection),
		Session: sqlbase.Session{
			SessionFacts: db.SessionFacts{
				Descriptor: db.SessionDescriptor{
					Profile: profile, ServerVersion: serverVersion,
					// A MySQL schema is a database. The connected one is the default.
					DefaultSchema: profile.Database,
				},
				Support: adapter.support,
			},
			Connection: connection, Pool: pool,
			Side: db.NewSideConnection(func() (*sql.DB, error) {
				return openMysqlPool(profile, password)
			}, nil),
			MainQueue: db.NewCallQueue(),
		},
		flavour:  adapter.flavour,
		threadID: threadID,
		profile:  profile, password: password,
	}
	session.Hooks = sqlbase.Hooks{
		BeginStatement:         "start transaction",
		PrepareConnection:      session.prepareReopenedConnection,
		IsDriverBroken:         isDriverBroken,
		StopAbandonedStatement: session.stopAbandonedStatement,
		ReportFailure: func(_ context.Context, err error) {
			session.markTransactionFailed(err)
		},
		WrapTransactionError: db.WrapDatabaseError,
		PrepareStream: func(ctx context.Context) error {
			// The row cap of a page would cut the export short.
			return session.applySelectLimit(ctx, -1)
		},
		ReadStreamColumns: readStreamColumns,
	}
	return session, nil
}

// readStreamColumns returns the columns of a streamed result and the reader of one row.
func readStreamColumns(
	rows *sql.Rows,
) ([]db.ResultColumn, func(*sql.Rows) ([]any, error), error) {
	columns, _, err := buildMysqlColumns(rows)
	return columns, func(rows *sql.Rows) ([]any, error) {
		return db.ScanRow(rows, len(columns))
	}, err
}

// prepareUserConnection makes the user connection read-only for a read-only profile, and
// returns the server connection ID. A missing ID disables cancellation.
func prepareUserConnection(
	ctx context.Context, connection *sql.Conn, profile cfg.Profile, flavour Flavour,
) (int64, error) {
	if profile.AccessMode == cfg.AccessReadOnly {
		if _, err := connection.ExecContext(ctx, flavour.ReadOnlyStatement); err != nil {
			return 0, err
		}
	}
	threadID := int64(0)
	if connection.QueryRowContext(ctx, "select connection_id() as id").Scan(&threadID) != nil {
		threadID = 0
	}
	return threadID, nil
}

// readRollsBackOnTimeout reads innodb_rollback_on_timeout. An unavailable value uses the MySQL and MariaDB default: statement-only rollback.
func readRollsBackOnTimeout(ctx context.Context, connection *sql.Conn) bool {
	var written string
	if err := connection.QueryRowContext(
		ctx, "select @@innodb_rollback_on_timeout as held").Scan(&written); err != nil {
		return false
	}
	return written == "1" || strings.EqualFold(written, "on")
}

// Compile-time Session interface check.
var _ db.Session = (*mysqlSession)(nil)
