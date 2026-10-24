// Package sqlbase is the session code of the engines that run on database/sql.
package sqlbase

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query/statement"
)

// Session is one user connection, its catalog pool and its transaction state.
type Session struct {
	db.SessionFacts

	Connection  *sql.Conn
	Pool        *sql.DB
	Side        *db.SideConnection[*sql.DB]
	Transaction db.TransactionMark
	// MainQueue serializes user connection calls. The single-connection catalog pool
	// serializes its own calls.
	MainQueue *db.CallQueue
	Hooks     Hooks

	// True where the user connection cannot take another statement.
	broken bool
}

// Hooks are the parts of a session that differ between engines.
type Hooks struct {
	// BeginStatement is the statement that opens a transaction.
	BeginStatement string
	// PrepareConnection prepares a user connection opened again. It may be nil.
	PrepareConnection func(ctx context.Context, opened *sql.Conn) error
	// IsDriverBroken is true for a driver error that leaves the connection unusable. It may
	// be nil.
	IsDriverBroken func(err error) bool
	// StopAbandonedStatement stops a statement the client stopped waiting for. It may be nil.
	StopAbandonedStatement func(ctx context.Context)
	// ReportFailure updates the transaction state after a failed statement.
	ReportFailure func(ctx context.Context, err error)
	// WrapTransactionError wraps the error of a statement that opens or ends a transaction.
	WrapTransactionError func(err error) error
	// WrapStatementError wraps the error of a streamed or staged statement. It may be nil.
	WrapStatementError func(err error) error
	// PrepareStream prepares the user connection for a streamed statement. It may be nil.
	PrepareStream func(ctx context.Context) error
	// ReadStreamColumns returns the columns of a streamed result and the reader of one row.
	ReadStreamColumns func(rows *sql.Rows) ([]db.ResultColumn, func(*sql.Rows) ([]any, error), error)
}

// StateWait is the time limit for one call the client makes after a failure.
const StateWait = 5 * time.Second

// reopenWait is the time limit for opening the connection again after a failure.
const reopenWait = 15 * time.Second

// closeWait is the time limit for waiting to close the connection.
const closeWait = 5 * time.Second

func (session *Session) ReadTransactionState() db.TransactionState {
	return session.Transaction.ReadState()
}

// MarkTransactionFromStatement updates transaction state after statements from the editor.
func (session *Session) MarkTransactionFromStatement(sql string) {
	session.Transaction.ApplyStatementEffect(
		statement.ResolveTransactionEffect(sql, session.Support.Dialect.Syntax))
}

// MarkBroken records a user connection that cannot take another statement, and opens it
// again at once.
func (session *Session) MarkBroken(ctx context.Context, err error) {
	if !db.IsBrokenConnection(ctx, err) &&
		(session.Hooks.IsDriverBroken == nil || !session.Hooks.IsDriverBroken(err)) {
		return
	}
	if ctx.Err() != nil && session.Hooks.StopAbandonedStatement != nil {
		session.Hooks.StopAbandonedStatement(ctx)
	}
	session.DropConnection(ctx)
}

// DropConnection records the connection as broken, and opens it again at once if no
// transaction is open.
func (session *Session) DropConnection(ctx context.Context) {
	session.broken = true
	if session.Transaction.ReadState() != db.TransactionNone {
		// The next call reports the transaction that was lost with the connection.
		return
	}
	// A connection that does not open now is opened by the next call.
	_ = session.ReopenBrokenConnection(ctx)
}

// RecordFailure marks a broken connection and updates the transaction state after a failed
// statement of the user connection.
func (session *Session) RecordFailure(ctx context.Context, err error) {
	session.MarkBroken(ctx, err)
	session.Hooks.ReportFailure(ctx, err)
}

// ReopenBrokenConnection opens the user connection again. A transaction is lost with the
// connection, so one that was open is reported instead of carried on.
func (session *Session) ReopenBrokenConnection(ctx context.Context) error {
	if !session.broken {
		return nil
	}
	lost := session.Transaction.ReadState() != db.TransactionNone
	session.Transaction.WriteState(db.TransactionNone)

	// The pool takes the old connection back and hands it out again, because a driver does
	// not always report one as bad. This marks it bad itself, so the pool drops it and the
	// connection that follows is a new one. The call takes a time limit of its own: the one
	// of the statement it failed in is spent.
	_ = session.Connection.Raw(func(any) error { return driver.ErrBadConn })
	_ = session.Connection.Close()
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), reopenWait)
	defer stop()
	opened, err := session.Pool.Conn(held)
	if err != nil {
		return db.WrapDatabaseMessage(
			db.BuildConnectMessage(session.Descriptor.Profile, err), err)
	}
	if session.Hooks.PrepareConnection != nil {
		if prepareErr := session.Hooks.PrepareConnection(held, opened); prepareErr != nil {
			_ = opened.Close()
			return db.WrapDatabaseMessage(
				db.BuildConnectMessage(session.Descriptor.Profile, prepareErr), prepareErr)
		}
	}
	session.Connection = opened
	session.broken = false
	if lost {
		return db.NewDatabaseError("the connection closed and the transaction was lost")
	}
	return nil
}

// QueryRunner is what a catalog read needs of a connection or a pool.
type QueryRunner interface {
	QueryContext(ctx context.Context, sql string, params ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, sql string, params ...any) (sql.Result, error)
}

// resolveCatalogRunner uses the user connection during a transaction and the catalog pool
// otherwise.
func (session *Session) resolveCatalogRunner() (QueryRunner, error) {
	if session.Transaction.ReadState() != db.TransactionNone {
		return session.Connection, nil
	}
	side, err := session.Side.Read()
	if err != nil {
		return nil, err
	}
	return side, nil
}

// holdCatalogPool reserves the user connection for transactional catalog reads and returns
// a release callback.
func (session *Session) holdCatalogPool(
	ctx context.Context,
) (QueryRunner, func(), error) {
	runner, err := session.resolveCatalogRunner()
	if err != nil {
		return nil, nil, err
	}
	if session.Transaction.ReadState() == db.TransactionNone {
		return runner, func() {}, nil
	}
	giveBack, waitErr := session.MainQueue.Take(ctx)
	if waitErr != nil {
		return nil, nil, waitErr
	}
	return runner, giveBack, nil
}

// ReadNamedRows reads a catalog result as rows keyed by column name, and returns the order
// the server named them in.
func (session *Session) ReadNamedRows(
	ctx context.Context, sql string, params ...any,
) ([]map[string]any, []string, error) {
	runner, giveBack, err := session.holdCatalogPool(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer giveBack()

	rows, queryErr := runner.QueryContext(ctx, sql, params...)
	if queryErr != nil {
		return nil, nil, queryErr
	}
	defer func() { _ = rows.Close() }()

	names, nameErr := rows.Columns()
	if nameErr != nil {
		return nil, nil, nameErr
	}

	read := []map[string]any{}
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(names))
		if scanErr != nil {
			return nil, nil, scanErr
		}
		row := map[string]any{}
		for at, name := range names {
			row[name] = values[at]
		}
		read = append(read, row)
	}
	return read, names, rows.Err()
}

// wrapStatementError wraps the error of a streamed or staged statement.
func (session *Session) wrapStatementError(err error) error {
	if session.Hooks.WrapStatementError == nil {
		return err
	}
	return session.Hooks.WrapStatementError(err)
}

// StreamQuery reads a batch at a time, so an export never holds the whole relation.
func (session *Session) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	if err := db.RefuseSeveralCommands(sql, session.Support.Dialect.Syntax); err != nil {
		return 0, err
	}
	giveBack, waitErr := session.MainQueue.Take(ctx)
	if waitErr != nil {
		return 0, waitErr
	}
	defer giveBack()

	if err := session.ReopenBrokenConnection(ctx); err != nil {
		return 0, err
	}
	if session.Hooks.PrepareStream != nil {
		if err := session.Hooks.PrepareStream(ctx); err != nil {
			return 0, err
		}
	}

	rows, err := session.Connection.QueryContext(ctx, sql, params...)
	if err != nil {
		session.RecordFailure(ctx, err)
		return 0, session.wrapStatementError(err)
	}
	defer func() { _ = rows.Close() }()

	columns, readRow, columnErr := session.Hooks.ReadStreamColumns(rows)
	if columnErr != nil {
		return 0, columnErr
	}
	if columns == nil {
		return 0, nil
	}

	batcher := db.NewRowBatcher(batchSize, onBatch)
	for rows.Next() {
		values, scanErr := readRow(rows)
		if scanErr != nil {
			return batcher.CountRows(), scanErr
		}
		if batchErr := batcher.AddRow(values, columns); batchErr != nil {
			return batcher.CountRows(), batchErr
		}
	}
	if rows.Err() != nil {
		session.RecordFailure(ctx, rows.Err())
		return batcher.CountRows(), session.wrapStatementError(rows.Err())
	}
	if batchErr := batcher.FlushRows(columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// RunPlain runs a statement of the client itself, which binds nothing and returns nothing.
func (session *Session) RunPlain(ctx context.Context, sql string) error {
	_, err := session.Connection.ExecContext(ctx, sql)
	session.MarkBroken(ctx, err)
	return err
}

// runTransactionStatement runs a statement that opens or ends a transaction, and records
// the state it leaves.
func (session *Session) runTransactionStatement(
	ctx context.Context, sql string, state db.TransactionState,
) error {
	giveBack, waitErr := session.MainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.ReopenBrokenConnection(ctx); err != nil {
		return err
	}
	if err := session.RunPlain(ctx, sql); err != nil {
		return session.Hooks.WrapTransactionError(err)
	}
	session.Transaction.WriteState(state)
	return nil
}

func (session *Session) BeginTransaction(ctx context.Context) error {
	return session.runTransactionStatement(ctx, session.Hooks.BeginStatement, db.TransactionOpen)
}

func (session *Session) CommitTransaction(ctx context.Context) error {
	return session.runTransactionStatement(ctx, "commit", db.TransactionNone)
}

func (session *Session) RollbackTransaction(ctx context.Context) error {
	return session.runTransactionStatement(ctx, "rollback", db.TransactionNone)
}

func (session *Session) ApplyChanges(ctx context.Context, changes []db.Change) error {
	giveBack, waitErr := session.MainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.ReopenBrokenConnection(ctx); err != nil {
		return err
	}
	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		JoinsUserTransaction: session.Transaction.ReadState() == db.TransactionOpen,
		Begin: func(ctx context.Context) error {
			return session.RunPlain(ctx, session.Hooks.BeginStatement)
		},
		Commit:   func(ctx context.Context) error { return session.RunPlain(ctx, "commit") },
		Rollback: func(ctx context.Context) error { return session.RunPlain(ctx, "rollback") },
		Apply: func(ctx context.Context, change db.Change) error {
			held, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			_, execErr := session.Connection.ExecContext(ctx, held.SQL, held.Params...)
			if execErr == nil {
				return nil
			}
			session.MarkBroken(ctx, execErr)
			return session.wrapStatementError(execErr)
		},
		CountMatches: db.BuildGuardCounter(
			func(ctx context.Context, sql string, params ...any) db.ScanOneRow {
				return session.Connection.QueryRowContext(ctx, sql, params...)
			}),
		ReportFailed: func(err error) { session.Hooks.ReportFailure(ctx, err) },
	})
}

// Ping checks the catalog connection outside transactions. Transactional checks use the
// user connection and skip busy connections.
func (session *Session) Ping(ctx context.Context) error {
	runner, err := session.resolveCatalogRunner()
	if err != nil {
		return err
	}
	if session.Transaction.ReadState() != db.TransactionNone {
		giveBack, free := session.MainQueue.TryTake()
		if !free {
			return nil
		}
		defer giveBack()
	}
	_, execErr := runner.ExecContext(ctx, "select 1")
	return execErr
}

// The user connection is one socket the driver takes no second caller on.
func (session *Session) Close() error {
	ctx, stop := context.WithTimeout(context.Background(), closeWait)
	defer stop()
	if side, opened := session.Side.Find(); opened {
		_ = side.Close()
	}
	if giveBack, waitErr := session.MainQueue.Take(ctx); waitErr == nil {
		defer giveBack()
	}
	err := session.Connection.Close()
	_ = session.Pool.Close()
	return err
}
