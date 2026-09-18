package cassandra

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gocql/gocql"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// connectTimeout is how long one attempt at the cluster may take.
const connectTimeout = 15 * time.Second

// cassandraSession is one open session on a Cassandra cluster.
type cassandraSession struct {
	db.NoUserTransactions
	db.NoServerSessions
	db.SessionFacts

	cluster *gocql.Session
}

// buildQuery returns the query of one statement. A statement with values takes them through
// the prepared statement, so each one is marshalled as the type of its column.
func (session *cassandraSession) buildQuery(
	ctx context.Context, written string, params []any,
) *gocql.Query {
	if len(params) == 0 {
		return session.cluster.Query(written).WithContext(ctx)
	}
	return session.cluster.Bind(written, buildBinding(params)).WithContext(ctx)
}

// runStatement runs one statement and reads at most rowLimit rows, plus the row that marks
// a truncated read. A rowLimit below zero reads every row.
func (session *cassandraSession) runStatement(
	ctx context.Context, written string, rowLimit int, params []any,
) (db.QueryResult, error) {
	command := strings.ToUpper(syntax.ReadCommandWord(written, syntax.FlavourStandard))
	held := session.buildQuery(ctx, written, params)
	defer held.Release()

	iterator := held.Iter()
	columns := readColumns(iterator.Columns())
	if len(columns) == 0 {
		if err := iterator.Close(); err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		// The server reports no count for a write, so the result names the command alone.
		return db.QueryResult{Command: command}, nil
	}

	rows := [][]any{}
	truncated := false
	for {
		if rowLimit >= 0 && len(rows) > rowLimit {
			truncated = true
			rows = rows[:rowLimit]
			break
		}
		values, held := readRow(iterator)
		if !held {
			break
		}
		rows = append(rows, values)
	}
	if err := iterator.Close(); err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}

	return db.QueryResult{
		Columns: columns, HoldsResultSet: true, Rows: rows,
		Truncated: truncated, Command: command,
	}, nil
}

func (session *cassandraSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	statements := statement.SplitStatements(sql, syntax.FlavourStandard)
	if len(statements) == 0 {
		return db.QueryResult{}, nil
	}
	// The protocol takes one statement per call, so a buffer of several runs one at a time
	// and the values of the user belong to a buffer that holds one.
	if len(statements) > 1 && len(params) > 0 {
		return db.QueryResult{}, db.NewDatabaseError(
			"only one statement at a time can bind values on this server")
	}

	result := db.QueryResult{}
	for _, held := range statements {
		answered, err := session.runStatement(ctx, held, rowLimit, params)
		if err != nil {
			return db.QueryResult{}, err
		}
		result = answered
	}
	result.Elapsed = time.Since(startedAt)
	return result, nil
}

// ReadPage returns one page of a read. CQL has no OFFSET, so the rows before the page are
// read and dropped here, and a deep page costs the rows above it.
func (session *cassandraSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	answered, err := session.RunQuery(
		ctx, read.Text, window.Offset+window.Limit, read.Params)
	if err != nil {
		return db.QueryResult{}, err
	}
	if window.Offset >= len(answered.Rows) {
		answered.Rows = [][]any{}
		return answered, nil
	}
	answered.Rows = answered.Rows[window.Offset:]
	return answered, nil
}

// CountRead answers nothing. A CQL count reads every partition of the table, which costs
// the same as the read it would count.
func (session *cassandraSession) CountRead(
	context.Context, db.ComposedRead,
) (int64, bool, error) {
	return 0, false, nil
}

// CheckStatement answers nothing. The server has no statement that checks a statement
// without running it.
func (session *cassandraSession) CheckStatement(
	context.Context, string,
) (db.StatementProblem, bool) {
	return db.StatementProblem{}, false
}

func (session *cassandraSession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	held := session.buildQuery(ctx, sql, params)
	defer held.Release()

	iterator := held.Iter()
	columns := readColumns(iterator.Columns())
	if len(columns) == 0 {
		if err := iterator.Close(); err != nil {
			return 0, db.WrapDatabaseError(err)
		}
		return 0, nil
	}

	batcher := db.NewRowBatcher(batchSize, onBatch)
	for {
		values, more := readRow(iterator)
		if !more {
			break
		}
		if err := batcher.AddRow(values, columns); err != nil {
			return batcher.CountRows(), err
		}
	}
	if err := iterator.Close(); err != nil {
		return batcher.CountRows(), db.WrapDatabaseError(err)
	}
	if err := batcher.FlushRows(columns); err != nil {
		return batcher.CountRows(), err
	}
	return batcher.CountRows(), nil
}

// ExplainQuery is refused. CQL has no EXPLAIN.
func (session *cassandraSession) ExplainQuery(
	context.Context, string, bool,
) (db.QueryPlan, error) {
	return db.QueryPlan{}, db.NewUnsupportedError("explain a statement")
}

// ApplyChanges runs every staged change in one logged batch, which the server applies
// whole or not at all.
func (session *cassandraSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	if len(changes) == 0 {
		return nil
	}
	batch := session.cluster.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	for _, change := range changes {
		written, err := db.ReadChangeStatement(change)
		if err != nil {
			return err
		}
		if len(written.Params) == 0 {
			batch.Query(written.SQL)
			continue
		}
		batch.Bind(written.SQL, buildBinding(written.Params))
	}
	if err := session.cluster.ExecuteBatch(batch); err != nil {
		return db.WrapDatabaseError(err)
	}
	return nil
}

func (session *cassandraSession) Ping(ctx context.Context) error {
	return db.WrapDatabaseError(
		session.cluster.Query("select release_version from system.local").
			WithContext(ctx).Exec())
}

func (session *cassandraSession) Close() error {
	session.cluster.Close()
	return nil
}

// heldLogger keeps what the driver writes instead of letting it reach standard error. The
// driver reports the reason a connection failed only through its log, so the last line it
// wrote is added to the connect error.
type heldLogger struct {
	guard sync.Mutex
	last  string
}

func (logger *heldLogger) Print(parts ...any) { logger.keep(fmt.Sprint(parts...)) }

func (logger *heldLogger) Printf(format string, parts ...any) {
	logger.keep(fmt.Sprintf(format, parts...))
}

func (logger *heldLogger) Println(parts ...any) { logger.keep(fmt.Sprintln(parts...)) }

func (logger *heldLogger) keep(written string) {
	logger.guard.Lock()
	defer logger.guard.Unlock()
	logger.last = strings.TrimSpace(written)
}

// ReadLast returns the last line the driver wrote.
func (logger *heldLogger) ReadLast() string {
	logger.guard.Lock()
	defer logger.guard.Unlock()
	return logger.last
}

// readDriverReason returns the reason of a failed connection, which the driver writes as
// the tail of its own log line.
func readDriverReason(written string) string {
	if at := strings.LastIndex(written, "due to error: "); at >= 0 {
		return written[at+len("due to error: "):]
	}
	return written
}

// cassandraAdapter opens a session on a Cassandra cluster.
type cassandraAdapter struct{ support db.EngineSupport }

// NewAdapter returns the adapter that opens a Cassandra cluster.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &cassandraAdapter{support: support}
}

func (adapter *cassandraAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	cluster := gocql.NewCluster(fmt.Sprintf("%s:%d", profile.Host, profile.Port))
	cluster.Keyspace = profile.Database
	// The driver writes its own faults to standard error, which would land on the screen
	// the client draws.
	logger := &heldLogger{}
	cluster.Logger = logger
	cluster.Timeout = connectTimeout
	cluster.ConnectTimeout = connectTimeout
	// The client holds one connection, as it does for every other engine.
	cluster.NumConns = 1
	if profile.User != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: profile.User, Password: password,
		}
	}
	policy := core.ResolveSSLPolicy(profile.SSLMode)
	if held := db.BuildPolicyTLS(policy, profile.Host); held != nil {
		cluster.SslOpts = &gocql.SslOptions{
			Config:                 held,
			EnableHostVerification: core.VerifiesCertificate(policy),
		}
	}

	opened, err := cluster.CreateSession()
	if err != nil {
		reason := err
		if held := logger.ReadLast(); held != "" {
			reason = fmt.Errorf("%s", readDriverReason(held))
		}
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), err)
	}

	session := &cassandraSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: readServerVersion(ctx, opened),
				DefaultSchema: profile.Database,
			},
			Support: adapter.support,
		},
		cluster: opened,
	}
	return session, nil
}

// readServerVersion returns the release the cluster reports for the node this session
// reached.
func readServerVersion(ctx context.Context, opened *gocql.Session) string {
	written := ""
	err := opened.Query("select release_version from system.local").
		WithContext(ctx).Scan(&written)
	if err != nil || written == "" {
		return "unknown"
	}
	return written
}

// Compile-time Session interface check.
var _ db.Session = (*cassandraSession)(nil)
