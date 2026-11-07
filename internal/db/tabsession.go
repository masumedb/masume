package db

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/language"
)

// TabSession is the session of one tab. It connects in the background, or on the first
// statement of the tab. The catalog reads and the statement check use it while it holds a
// transaction, and the connection session otherwise. The server admin calls always use the
// connection session.
type TabSession struct {
	shared Session
	open   func(ctx context.Context) (Session, error)

	opening sync.Mutex
	guard   sync.Mutex
	inner   Session
	closed  bool
	// True while a background open runs.
	pending atomic.Bool
	// True after a failed background open, until the tab connects.
	failed atomic.Bool
	// The reads of the tab that run on the connection session now.
	borrowed atomic.Int32
}

// ClaimOpen is true where the tab is not connected, no background open runs, and no
// background open failed. A true answer claims the background open, and OpenAhead ends the
// claim.
func (session *TabSession) ClaimOpen() bool {
	if _, open := session.findOpen(); open || session.failed.Load() {
		return false
	}
	return session.pending.CompareAndSwap(false, true)
}

// OpenAhead connects the tab session in the background. After a failed open, no further
// background open starts, and the next statement of the tab connects again.
func (session *TabSession) OpenAhead(ctx context.Context) error {
	defer session.pending.Store(false)
	_, err := session.reach(ctx)
	if err != nil {
		if _, open := session.findOpen(); !open {
			session.failed.Store(true)
		}
	}
	return err
}

// BorrowShared returns the tab session with its reads on the connection session while the
// tab is not connected. Every other call goes to the tab session.
func (session *TabSession) BorrowShared() Session {
	return &borrowingSession{TabSession: session}
}

// borrowingSession is a tab session whose reads run on the connection session until the
// tab connects.
type borrowingSession struct {
	*TabSession
}

// lend returns the session a read runs on, and the call that ends the loan.
func (session *borrowingSession) lend() (Session, func()) {
	if _, open := session.findOpen(); open {
		return session.TabSession, func() {}
	}
	session.borrowed.Add(1)
	return session.shared, func() { session.borrowed.Add(-1) }
}

func (session *borrowingSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (QueryResult, error) {
	lent, done := session.lend()
	defer done()
	return lent.RunQuery(ctx, sql, rowLimit, params)
}

func (session *borrowingSession) ReadPage(
	ctx context.Context, read ComposedRead, window ReadWindow,
) (QueryResult, error) {
	lent, done := session.lend()
	defer done()
	return lent.ReadPage(ctx, read, window)
}

func (session *borrowingSession) CountRead(ctx context.Context, read ComposedRead) (int64, bool, error) {
	lent, done := session.lend()
	defer done()
	return lent.CountRead(ctx, read)
}

// NewTabSession returns a tab session that is not connected yet.
func NewTabSession(shared Session, open func(ctx context.Context) (Session, error)) *TabSession {
	return &TabSession{shared: shared, open: open}
}

// findOpen returns the connected session, and false before the first connect.
func (session *TabSession) findOpen() (Session, bool) {
	session.guard.Lock()
	defer session.guard.Unlock()
	return session.inner, session.inner != nil
}

// resolveCatalogSession returns the connected session while it holds a transaction, and the
// connection session otherwise.
func (session *TabSession) resolveCatalogSession() Session {
	if inner, open := session.findOpen(); open && inner.ReadTransactionState() != TransactionNone {
		return inner
	}
	return session.shared
}

// reach returns the connected session, and connects first where there is none.
func (session *TabSession) reach(ctx context.Context) (Session, error) {
	if inner, open := session.findOpen(); open {
		return inner, nil
	}
	session.opening.Lock()
	defer session.opening.Unlock()

	session.guard.Lock()
	inner, closed := session.inner, session.closed
	session.guard.Unlock()
	if closed {
		return nil, NewDatabaseError("the tab is closed")
	}
	if inner != nil {
		return inner, nil
	}

	opened, err := session.open(ctx)
	if err != nil {
		return nil, err
	}
	session.guard.Lock()
	if session.closed {
		session.guard.Unlock()
		_ = opened.Close()
		return nil, NewDatabaseError("the tab is closed")
	}
	session.inner = opened
	session.guard.Unlock()
	session.failed.Store(false)
	return opened, nil
}

// Detach returns the connected session for closing, and forgets it. The next call that
// needs the server connects again.
func (session *TabSession) Detach() (Session, bool) {
	session.guard.Lock()
	defer session.guard.Unlock()
	inner := session.inner
	session.inner = nil
	return inner, inner != nil
}

func (session *TabSession) Describe() SessionDescriptor     { return session.shared.Describe() }
func (session *TabSession) Dialect() *query.Dialect         { return session.shared.Dialect() }
func (session *TabSession) Language() language.Language     { return session.shared.Language() }
func (session *TabSession) Capabilities() core.Capabilities { return session.shared.Capabilities() }
func (session *TabSession) Composer() Composer              { return session.shared.Composer() }

func (session *TabSession) ListSchemas(ctx context.Context) ([]string, error) {
	return session.resolveCatalogSession().ListSchemas(ctx)
}

func (session *TabSession) ListTables(ctx context.Context) ([]TableRef, error) {
	return session.resolveCatalogSession().ListTables(ctx)
}

func (session *TabSession) ListRoles(ctx context.Context) ([]DbRole, error) {
	return session.resolveCatalogSession().ListRoles(ctx)
}

func (session *TabSession) ListSchemaObjects(ctx context.Context) ([]SchemaObject, error) {
	return session.resolveCatalogSession().ListSchemaObjects(ctx)
}

func (session *TabSession) ListRelationships(ctx context.Context) ([]Relationship, error) {
	return session.resolveCatalogSession().ListRelationships(ctx)
}

func (session *TabSession) DescribeTable(ctx context.Context, table TableRef) (TableDetail, error) {
	return session.resolveCatalogSession().DescribeTable(ctx, table)
}

func (session *TabSession) ListIndexes(ctx context.Context, table TableRef) ([]IndexDetail, error) {
	return session.resolveCatalogSession().ListIndexes(ctx, table)
}

func (session *TabSession) ListConstraints(
	ctx context.Context, table TableRef,
) ([]ConstraintDetail, error) {
	return session.resolveCatalogSession().ListConstraints(ctx, table)
}

func (session *TabSession) BuildTableDDL(ctx context.Context, table TableRef) ([]string, error) {
	return session.resolveCatalogSession().BuildTableDDL(ctx, table)
}

func (session *TabSession) BuildObjectDDL(
	ctx context.Context, object SchemaObject,
) ([]string, error) {
	return session.resolveCatalogSession().BuildObjectDDL(ctx, object)
}

func (session *TabSession) ListSequenceValues(
	ctx context.Context, sequences, tables []query.QualifiedName,
) ([]SequenceValue, error) {
	return session.resolveCatalogSession().ListSequenceValues(ctx, sequences, tables)
}

func (session *TabSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (QueryResult, error) {
	inner, err := session.reach(ctx)
	if err != nil {
		return QueryResult{}, err
	}
	return inner.RunQuery(ctx, sql, rowLimit, params)
}

func (session *TabSession) ReadPage(
	ctx context.Context, read ComposedRead, window ReadWindow,
) (QueryResult, error) {
	inner, err := session.reach(ctx)
	if err != nil {
		return QueryResult{}, err
	}
	return inner.ReadPage(ctx, read, window)
}

func (session *TabSession) CountRead(ctx context.Context, read ComposedRead) (int64, bool, error) {
	inner, err := session.reach(ctx)
	if err != nil {
		return 0, false, err
	}
	return inner.CountRead(ctx, read)
}

func (session *TabSession) CheckStatement(ctx context.Context, sql string) (StatementProblem, bool) {
	return session.resolveCatalogSession().CheckStatement(ctx, sql)
}

func (session *TabSession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []ResultColumn) error,
) (int64, error) {
	inner, err := session.reach(ctx)
	if err != nil {
		return 0, err
	}
	return inner.StreamQuery(ctx, sql, params, batchSize, onBatch)
}

func (session *TabSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (QueryPlan, error) {
	inner, err := session.reach(ctx)
	if err != nil {
		return QueryPlan{}, err
	}
	return inner.ExplainQuery(ctx, sql, analyze)
}

func (session *TabSession) ReadTransactionState() TransactionState {
	if inner, open := session.findOpen(); open {
		return inner.ReadTransactionState()
	}
	return TransactionNone
}

func (session *TabSession) BeginTransaction(ctx context.Context) error {
	inner, err := session.reach(ctx)
	if err != nil {
		return err
	}
	return inner.BeginTransaction(ctx)
}

func (session *TabSession) CommitTransaction(ctx context.Context) error {
	inner, err := session.reach(ctx)
	if err != nil {
		return err
	}
	return inner.CommitTransaction(ctx)
}

func (session *TabSession) RollbackTransaction(ctx context.Context) error {
	inner, err := session.reach(ctx)
	if err != nil {
		return err
	}
	return inner.RollbackTransaction(ctx)
}

func (session *TabSession) ApplyChanges(ctx context.Context, changes []Change) error {
	inner, err := session.reach(ctx)
	if err != nil {
		return err
	}
	return inner.ApplyChanges(ctx, changes)
}

func (session *TabSession) ListActivity(ctx context.Context) ([]Activity, error) {
	return session.shared.ListActivity(ctx)
}

func (session *TabSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	return session.shared.CancelBackend(ctx, pid, terminate)
}

// CancelRunningQuery stops the statement of the tab through the connection session. A read
// of the tab that runs on the connection session stops with the statement of the connection
// session.
func (session *TabSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	stopped := false
	if session.borrowed.Load() > 0 {
		held, err := session.shared.CancelRunningQuery(ctx)
		if err != nil {
			return false, err
		}
		stopped = held
	}
	inner, open := session.findOpen()
	if !open {
		return stopped, nil
	}
	capabilities := session.shared.Capabilities()
	var held bool
	var err error
	if backendID, known := FindBackendID(inner); known &&
		capabilities.CancelsRunningQuery && capabilities.HasServerSessions {
		held, err = session.shared.CancelBackend(ctx, backendID, false)
	} else {
		held, err = inner.CancelRunningQuery(ctx)
	}
	return stopped || held, err
}

// CheckIdleConnection checks the user connection of the tab. It checks nothing before the
// tab connects.
func (session *TabSession) CheckIdleConnection(ctx context.Context) error {
	inner, open := session.findOpen()
	if !open {
		return nil
	}
	if checker, checks := FindIdleChecker(inner); checks {
		return checker.CheckIdleConnection(ctx)
	}
	return nil
}

// ReadBackendID returns the server id of the user connection of the tab, and false before
// the tab connects.
func (session *TabSession) ReadBackendID() (int64, bool) {
	inner, open := session.findOpen()
	if !open {
		return 0, false
	}
	return FindBackendID(inner)
}

func (session *TabSession) ListLockWaits(ctx context.Context) ([]LockWait, error) {
	return session.shared.ListLockWaits(ctx)
}

func (session *TabSession) ListSlowStatements(
	ctx context.Context, limit int,
) ([]StatementStat, error) {
	return session.shared.ListSlowStatements(ctx, limit)
}

func (session *TabSession) ReadServerLoad(ctx context.Context) (ServerLoad, error) {
	return session.shared.ReadServerLoad(ctx)
}

func (session *TabSession) Ping(ctx context.Context) error {
	return session.shared.Ping(ctx)
}

// Close closes the connected session. A tab session never connects after Close.
func (session *TabSession) Close() error {
	session.guard.Lock()
	inner := session.inner
	session.inner, session.closed = nil, true
	session.guard.Unlock()
	if inner == nil {
		return nil
	}
	return inner.Close()
}

// Compile-time Session interface check.
var _ Session = (*TabSession)(nil)
