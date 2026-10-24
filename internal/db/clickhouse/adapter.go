package clickhouse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query/statement"
	"github.com/masumedb/masume/internal/query/syntax"
)

// The error codes of the server this client acts on.
const (
	codeSessionNotFound       = 372
	codeSessionLocked         = 373
	codeInvalidSessionTimeout = 374
)

// sessionTimeout is the idle time a server session lives, in seconds. It is the largest the
// server takes by default.
const sessionTimeout = "3600"

// sessionLockWait is the time limit for waiting on a server session a stopped statement
// still holds.
const sessionLockWait = 5 * time.Second

// sessionLockPoll is the time between two tries of a locked server session.
const sessionLockPoll = 50 * time.Millisecond

// stopWait is the time limit for stopping a statement on the server.
const stopWait = 5 * time.Second

// clickhouseSession is one session on a ClickHouse.
type clickhouseSession struct {
	db.NoSequences
	db.SessionFacts
	// The server holds no transaction of the user.
	db.NoUserTransactions

	client *httpClient
	// The server session that keeps the SET and USE statements of the user.
	sessionID string
	// The idle time of the server session, empty for the default of the server.
	sessionTimeout string
	// True where the server holds the log this client reads statement counters from.
	holdsStatementStats bool

	// The guard protects the id of the running statement and the ids of the activity list.
	guard sync.Mutex
	// The id of the statement the user session runs now, which a stop needs.
	runningID string
	// The query the client listed at each row of the activity list. The server names a
	// query by a text id, and the list of this client counts its rows instead.
	listedIDs map[int64]string

	// mainQueue serializes the calls of the user session.
	mainQueue *db.CallQueue
}

// Capabilities returns engine support plus statement statistics availability from the
// connected server.
func (session *clickhouseSession) Capabilities() core.Capabilities {
	held := session.Support.Capabilities
	held.ReportsStatementStats = session.holdsStatementStats
	return held
}

// holdRunningID records the statement the session runs, and gives it back afterwards.
func (session *clickhouseSession) holdRunningID(id string) func() {
	session.guard.Lock()
	session.runningID = id
	session.guard.Unlock()
	return func() {
		session.guard.Lock()
		if session.runningID == id {
			session.runningID = ""
		}
		session.guard.Unlock()
	}
}

// readRunningID returns the id of the statement the session runs now.
func (session *clickhouseSession) readRunningID() string {
	session.guard.Lock()
	defer session.guard.Unlock()
	return session.runningID
}

// buildSideParams returns the parameters of a request outside the server session.
func (session *clickhouseSession) buildSideParams() url.Values {
	params := url.Values{}
	if database := session.Descriptor.Profile.Database; database != "" {
		params.Set("database", database)
	}
	return params
}

// buildSessionParams returns the parameters of a request in the server session. A
// read-only profile sends readonly = 2 with every statement: mode 2 refuses a write and
// takes a setting.
func (session *clickhouseSession) buildSessionParams(queryID string, check bool) url.Values {
	params := url.Values{"session_id": {session.sessionID}}
	if session.sessionTimeout != "" {
		params.Set("session_timeout", session.sessionTimeout)
	}
	if check {
		params.Set("session_check", "1")
	}
	if queryID != "" {
		params.Set("query_id", queryID)
	}
	if session.Descriptor.Profile.AccessMode == cfg.AccessReadOnly {
		params.Set("readonly", "2")
	}
	params.Set("cancel_http_readonly_queries_on_client_close", "1")
	return params
}

// openServerSession opens the server session in the database of the profile.
func (session *clickhouseSession) openServerSession(ctx context.Context) error {
	opening := "select 1"
	if database := session.Descriptor.Profile.Database; database != "" {
		opening = "use " + session.Support.Dialect.QuoteIdentifier(database)
	}
	response, err := session.client.send(ctx, opening, session.buildSessionParams("", false))
	if code, reported := findServerCode(err); reported && code == codeInvalidSessionTimeout &&
		session.sessionTimeout != "" {
		session.sessionTimeout = ""
		return session.openServerSession(ctx)
	}
	if err != nil {
		return err
	}
	return drainResult(response)
}

// sendInSession sends one statement in the server session. A session that expired opens
// again in the database of the profile, and a session a stopped statement still holds is
// tried again for a short time.
func (session *clickhouseSession) sendInSession(
	ctx context.Context, statement, queryID string,
) (*http.Response, error) {
	deadline := time.Now().Add(sessionLockWait)
	reopened := false
	for {
		response, err := session.client.send(
			ctx, statement, session.buildSessionParams(queryID, true))
		code, reported := findServerCode(err)
		switch {
		case reported && code == codeSessionNotFound && !reopened:
			reopened = true
			if openErr := session.openServerSession(ctx); openErr != nil {
				return nil, openErr
			}
			continue
		case reported && code == codeSessionLocked && time.Now().Before(deadline):
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sessionLockPoll):
			}
			continue
		}
		return response, err
	}
}

// stopStatement stops a statement on the server after the client stopped reading it.
func (session *clickhouseSession) stopStatement(ctx context.Context, id string) {
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), stopWait)
	defer stop()
	_, _ = session.killQuery(held, id)
}

// failStatement returns the error of a statement. A statement that failed on the client
// side is stopped on the server.
func (session *clickhouseSession) failStatement(ctx context.Context, id string, err error) error {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if _, reported := findServerCode(err); !reported {
		session.stopStatement(ctx, id)
	}
	return db.WrapDatabaseMessage(describeFailure(err), err)
}

func (session *clickhouseSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	flavour := session.Support.Dialect.Syntax
	statements := statement.SplitStatements(sql, flavour)
	if len(statements) == 0 {
		return db.QueryResult{}, nil
	}
	// The server takes one statement per request, so a buffer of several runs one at a
	// time and the values of the user belong to a buffer that holds one.
	if len(statements) > 1 && len(params) > 0 {
		return db.QueryResult{}, db.NewDatabaseError(
			"only one statement at a time can bind values on this server")
	}

	result := db.QueryResult{}
	for _, held := range statements {
		answered, err := session.runOne(ctx, held, rowLimit, params)
		if err != nil {
			return db.QueryResult{}, err
		}
		result = answered
	}
	result.Elapsed = time.Since(startedAt)
	return result, nil
}

// runOne runs one statement of a buffer and returns what it answered.
func (session *clickhouseSession) runOne(
	ctx context.Context, written string, rowLimit int, params []any,
) (db.QueryResult, error) {
	flavour := session.Support.Dialect.Syntax
	command := syntax.ReadCommandWord(written, flavour)
	bound, bindErr := bindParams(written, params)
	if bindErr != nil {
		return db.QueryResult{}, db.WrapDatabaseMessage(bindErr.Error(), bindErr)
	}
	id := buildClientID()
	release := session.holdRunningID(id)
	defer release()

	response, err := session.sendInSession(ctx, bound, id)
	if err != nil {
		return db.QueryResult{}, session.failStatement(ctx, id, err)
	}
	if !ReadsRows(command) {
		if drainErr := drainResult(response); drainErr != nil {
			return db.QueryResult{}, session.failStatement(ctx, id, drainErr)
		}
		// The server reports no count for a write, so the result names the command alone.
		return db.QueryResult{Command: strings.ToUpper(command)}, nil
	}

	reader, err := openResult(response)
	if err != nil {
		return db.QueryResult{}, session.failStatement(ctx, id, err)
	}
	defer reader.close()
	wanted := db.ReadOverscanRowLimit(rowLimit)
	read, readErr := reader.readRows(wanted)
	if readErr != nil {
		return db.QueryResult{}, session.failStatement(ctx, id, readErr)
	}
	if rowLimit >= 0 && len(read) >= wanted {
		_, nextErr := reader.readRow()
		switch {
		case nextErr == nil:
			reader.close()
			session.stopStatement(ctx, id)
		case !errors.Is(nextErr, io.EOF):
			return db.QueryResult{}, session.failStatement(ctx, id, nextErr)
		}
	}
	return db.BuildCappedResult(db.CappedRead{
		Rows: read, RowLimit: rowLimit, Columns: reader.columns,
		Command: strings.ToUpper(command),
	}), nil
}

func (session *clickhouseSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect)
}

func (session *clickhouseSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement asks the server to plan the statement, which reads every name in it and
// runs nothing. Only a read is planned.
func (session *clickhouseSession) CheckStatement(
	ctx context.Context, sql string,
) (db.StatementProblem, bool) {
	trimmed := strings.TrimSpace(sql)
	flavour := session.Support.Dialect.Syntax
	if trimmed == "" || db.HoldsSeveralCommands(sql, flavour) {
		return db.StatementProblem{}, false
	}
	if !PlansStatement(syntax.ReadCommandWord(sql, flavour)) {
		return db.StatementProblem{}, false
	}

	response, planErr := session.client.send(ctx, planPrefix+trimmed, session.buildSideParams())
	if planErr == nil {
		_ = drainResult(response)
		return db.StatementProblem{}, false
	}
	return ReadStatementProblem(planErr, planPrefix)
}

// StreamQuery reads a batch at a time, so an export never holds the whole relation.
func (session *clickhouseSession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	if err := db.RefuseSeveralCommands(sql, session.Support.Dialect.Syntax); err != nil {
		return 0, err
	}
	bound, bindErr := bindParams(sql, params)
	if bindErr != nil {
		return 0, db.WrapDatabaseMessage(bindErr.Error(), bindErr)
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return 0, waitErr
	}
	defer giveBack()

	id := buildClientID()
	release := session.holdRunningID(id)
	defer release()

	response, err := session.sendInSession(ctx, bound, id)
	if err != nil {
		return 0, session.failStatement(ctx, id, err)
	}
	reader, err := openResult(response)
	if err != nil {
		return 0, session.failStatement(ctx, id, err)
	}
	defer reader.close()
	if reader.columns == nil {
		return 0, nil
	}

	batcher := db.NewRowBatcher(batchSize, onBatch)
	for {
		values, readErr := reader.readRow()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return batcher.CountRows(), session.failStatement(ctx, id, readErr)
		}
		if batchErr := batcher.AddRow(values, reader.columns); batchErr != nil {
			reader.close()
			session.stopStatement(ctx, id)
			return batcher.CountRows(), batchErr
		}
	}
	if batchErr := batcher.FlushRows(reader.columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// readNamedRows reads a catalog result outside the server session as rows keyed by column
// name, and returns the order the server named them in.
func (session *clickhouseSession) readNamedRows(
	ctx context.Context, sql string, params ...any,
) ([]map[string]any, []string, error) {
	bound, err := bindParams(sql, params)
	if err != nil {
		return nil, nil, err
	}
	response, err := session.client.send(ctx, bound, session.buildSideParams())
	if err != nil {
		return nil, nil, err
	}
	reader, err := openResult(response)
	if err != nil {
		return nil, nil, err
	}
	defer reader.close()
	values, err := reader.readRows(-1)
	if err != nil {
		return nil, nil, err
	}

	names := make([]string, 0, len(reader.columns))
	for _, column := range reader.columns {
		names = append(names, column.Name)
	}
	read := make([]map[string]any, 0, len(values))
	for _, held := range values {
		row := map[string]any{}
		for at, name := range names {
			row[name] = held[at]
		}
		read = append(read, row)
	}
	return read, names, nil
}

// ExplainQuery returns the plan of a read, which the server writes as one line per step.
func (session *clickhouseSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	if analyze {
		return db.QueryPlan{}, db.NewDatabaseError(
			"measured query plans are unsupported; request an estimated plan")
	}
	if err := db.RefuseSeveralPlans(sql, session.Support.Dialect.Syntax); err != nil {
		return db.QueryPlan{}, err
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	id := buildClientID()
	release := session.holdRunningID(id)
	defer release()

	response, err := session.sendInSession(ctx, BuildExplainStatement(sql), id)
	if err != nil {
		return db.QueryPlan{}, session.failStatement(ctx, id, err)
	}
	reader, err := openResult(response)
	if err != nil {
		return db.QueryPlan{}, session.failStatement(ctx, id, err)
	}
	defer reader.close()
	read, readErr := reader.readRows(-1)
	if readErr != nil {
		return db.QueryPlan{}, session.failStatement(ctx, id, readErr)
	}
	plan, built := BuildPlan(db.QueryResult{Columns: reader.columns, Rows: read})
	if !built {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

// runInSession runs one statement of a staged change in the server session.
func (session *clickhouseSession) runInSession(
	ctx context.Context, sql string, params []any,
) error {
	bound, err := bindParams(sql, params)
	if err != nil {
		return db.WrapDatabaseMessage(err.Error(), err)
	}
	id := buildClientID()
	response, err := session.sendInSession(ctx, bound, id)
	if err == nil {
		err = drainResult(response)
	}
	if err != nil {
		return session.failStatement(ctx, id, err)
	}
	return nil
}

// firstValue is the first value of a result, which a guard reads as a count.
type firstValue struct {
	value any
	err   error
}

func (held firstValue) Scan(targets ...any) error {
	if held.err != nil {
		return held.err
	}
	if len(targets) != 1 {
		return db.NewDatabaseError("a guard reads one count")
	}
	counted, isCount := targets[0].(*int64)
	if !isCount {
		return db.NewDatabaseError("a guard reads one count")
	}
	*counted = db.ReadNonNegativeCount(held.value)
	return nil
}

// readFirstValue runs a read in the server session and returns its first value.
func (session *clickhouseSession) readFirstValue(
	ctx context.Context, sql string, params ...any,
) db.ScanOneRow {
	bound, err := bindParams(sql, params)
	if err != nil {
		return firstValue{err: err}
	}
	id := buildClientID()
	response, err := session.sendInSession(ctx, bound, id)
	if err != nil {
		return firstValue{err: session.failStatement(ctx, id, err)}
	}
	reader, err := openResult(response)
	if err != nil {
		return firstValue{err: session.failStatement(ctx, id, err)}
	}
	defer reader.close()
	rows, err := reader.readRows(-1)
	if err != nil {
		return firstValue{err: session.failStatement(ctx, id, err)}
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return firstValue{err: db.NewDatabaseError("the guard returned no row")}
	}
	return firstValue{value: rows[0][0]}
}

// ApplyChanges applies changes in order. The server holds no transaction, so a change that
// fails leaves the changes before it applied, and the error names the change that failed.
func (session *clickhouseSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	if len(changes) == 0 {
		return nil
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		// Every change stands on its own, so nothing opens, commits or rolls back.
		Begin:    func(context.Context) error { return nil },
		Commit:   func(context.Context) error { return nil },
		Rollback: func(context.Context) error { return nil },
		Apply: func(ctx context.Context, change db.Change) error {
			held, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			return session.runInSession(ctx, held.SQL, held.Params)
		},
		CountMatches: db.BuildGuardCounter(session.readFirstValue),
		ReportFailed: func(error) {},
	})
}

func (session *clickhouseSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	rows, _, err := session.readNamedRows(ctx, listActivitySQL)
	if err != nil {
		return nil, err
	}

	activity := make([]db.Activity, 0, len(rows))
	listed := map[int64]string{}
	for at, row := range rows {
		// The server names a query with a text of its own, so the list counts its rows and
		// keeps the name of each one for a stop.
		id := int64(at + 1)
		listed[id] = db.ReadAnyText(row["query_id"])
		activity = append(activity, db.Activity{
			PID: id, User: db.ReadAnyText(row["user"]),
			ApplicationName: db.ReadAnyText(row["client"]),
			ClientAddress:   db.ReadAnyText(row["address"]),
			State:           db.ReadAnyText(row["state"]),
			Duration: time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) *
				time.Millisecond,
			Query: db.ReadAnyText(row["query"]),
		})
	}
	session.guard.Lock()
	session.listedIDs = listed
	session.guard.Unlock()
	return activity, nil
}

// findListedID returns the query the activity list held at that row.
func (session *clickhouseSession) findListedID(pid int64) (string, bool) {
	session.guard.Lock()
	defer session.guard.Unlock()
	id, held := session.listedIDs[pid]
	return id, held && id != ""
}

// ReadServerLoad returns the connections of the server, its connection limit and the time
// it started.
func (session *clickhouseSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
	rows, _, err := session.readNamedRows(ctx, readServerLoadSQL)
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

// ListLockWaits is refused, because the server takes no lock a session waits for.
func (session *clickhouseSession) ListLockWaits(context.Context) ([]db.LockWait, error) {
	return nil, db.NewUnsupportedError("report lock waits")
}

// ListSlowStatements returns the statements the server spent the most time in, the slowest
// by mean time first.
func (session *clickhouseSession) ListSlowStatements(
	ctx context.Context, limit int,
) ([]db.StatementStat, error) {
	if !session.holdsStatementStats {
		return nil, db.NewUnsupportedError("report slow statements")
	}
	rows, _, err := session.readNamedRows(ctx, listSlowStatementsSQL, limit)
	if err != nil {
		return nil, err
	}
	stats := make([]db.StatementStat, 0, len(rows))
	for _, row := range rows {
		stats = append(stats, db.StatementStat{
			Query: db.ReadAnyText(row["query"]),
			Calls: db.ReadNonNegativeCount(row["calls"]),
			MeanTime: time.Duration(db.ReadNonNegativeCount(row["mean_ms"])) *
				time.Millisecond,
			TotalTime: time.Duration(db.ReadNonNegativeCount(row["total_ms"])) *
				time.Millisecond,
			Rows: db.ReadNonNegativeCount(row["rows_read"]),
		})
	}
	return stats, nil
}

// CancelBackend stops the statement the activity list held at that row. A query of this
// server belongs to no session that can be closed, so a stop ends the statement alone.
func (session *clickhouseSession) CancelBackend(
	ctx context.Context, pid int64, _ bool,
) (bool, error) {
	id, held := session.findListedID(pid)
	if !held {
		return false, db.NewDatabaseError("this statement is no longer in the list")
	}
	return session.killQuery(ctx, id)
}

// CancelRunningQuery stops the statement of this connection through a second connection,
// because the busy one cannot answer.
func (session *clickhouseSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	id := session.readRunningID()
	if id == "" {
		return false, db.NewDatabaseError("this connection runs no statement")
	}
	return session.killQuery(ctx, id)
}

// killQuery stops one statement of the server, named by its id, outside the server session.
func (session *clickhouseSession) killQuery(
	ctx context.Context, id string,
) (bool, error) {
	bound, err := bindParams(killQuerySQL, []any{id})
	if err != nil {
		return false, err
	}
	response, err := session.client.send(ctx, bound, session.buildSideParams())
	if err == nil {
		err = drainResult(response)
	}
	if err != nil {
		return false, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	return true, nil
}

// Ping runs a statement outside the server session, which answers while the session is
// busy.
func (session *clickhouseSession) Ping(ctx context.Context) error {
	response, err := session.client.send(ctx, "select 1", session.buildSideParams())
	if err != nil {
		return err
	}
	return drainResult(response)
}

// closeWait is the time limit for waiting to close the session.
const closeWait = 5 * time.Second

func (session *clickhouseSession) Close() error {
	ctx, stop := context.WithTimeout(context.Background(), closeWait)
	defer stop()
	if giveBack, waitErr := session.mainQueue.Take(ctx); waitErr == nil {
		defer giveBack()
	}
	session.client.close()
	return nil
}

// clickhouseAdapter opens a connection on a ClickHouse.
type clickhouseAdapter struct {
	support db.EngineSupport
}

// NewAdapter returns the adapter of the ClickHouse engine.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &clickhouseAdapter{support: support}
}

func (adapter *clickhouseAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	fail := func(reason error) (db.Session, error) {
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), reason)
	}
	if err := checkPort(profile); err != nil {
		return fail(err)
	}
	client, err := buildClient(profile, password)
	if err != nil {
		return fail(err)
	}
	if err := checkSettingNames(ctx, profile, password); err != nil {
		client.close()
		return fail(err)
	}

	session := &clickhouseSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile,
				// A ClickHouse schema is a database. The connected one is the default.
				DefaultSchema: profile.Database,
			},
			Support: adapter.support,
		},
		client: client, sessionID: buildClientID(), sessionTimeout: sessionTimeout,
		listedIDs: map[int64]string{}, mainQueue: db.NewCallQueue(),
	}
	if err := session.openServerSession(ctx); err != nil {
		client.close()
		return fail(err)
	}
	session.Descriptor.ServerVersion = session.readServerVersion(ctx)
	session.holdsStatementStats = session.readsStatementStats(ctx)
	return session, nil
}

// readServerVersion returns the version of the server, as it writes it itself.
func (session *clickhouseSession) readServerVersion(ctx context.Context) string {
	rows, _, err := session.readNamedRows(ctx, "select version() as version")
	if err != nil || len(rows) == 0 {
		return "unknown"
	}
	if written := db.ReadAnyText(rows[0]["version"]); written != "" {
		return written
	}
	return "unknown"
}

// readsStatementStats is true where the server keeps the log of the statements it ran.
func (session *clickhouseSession) readsStatementStats(ctx context.Context) bool {
	_, _, err := session.readNamedRows(ctx, "select count() from system.query_log limit 1")
	return err == nil
}

// Compile-time Session interface check.
var _ db.Session = (*clickhouseSession)(nil)
