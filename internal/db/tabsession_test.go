package db

import (
	"context"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

// recordingSession records the calls it takes under its own name.
type recordingSession struct {
	Session
	name        string
	calls       *[]string
	transaction TransactionState
	backendID   int64
	profile     cfg.Profile
}

func (session *recordingSession) record(call string) {
	*session.calls = append(*session.calls, session.name+" "+call)
}

func (session *recordingSession) Describe() SessionDescriptor {
	return SessionDescriptor{Profile: session.profile}
}

func (session *recordingSession) Capabilities() core.Capabilities {
	return core.Capabilities{CancelsRunningQuery: true, HasServerSessions: true}
}

func (session *recordingSession) ReadTransactionState() TransactionState {
	if session.transaction == "" {
		return TransactionNone
	}
	return session.transaction
}

func (session *recordingSession) ReadBackendID() (int64, bool) {
	return session.backendID, session.backendID > 0
}

func (session *recordingSession) RunQuery(context.Context, string, int, []any) (QueryResult, error) {
	session.record("run")
	return QueryResult{}, nil
}

func (session *recordingSession) ListTables(context.Context) ([]TableRef, error) {
	session.record("tables")
	return nil, nil
}

func (session *recordingSession) CheckStatement(context.Context, string) (StatementProblem, bool) {
	session.record("check")
	return StatementProblem{}, false
}

func (session *recordingSession) ListActivity(context.Context) ([]Activity, error) {
	session.record("activity")
	return nil, nil
}

func (session *recordingSession) Ping(context.Context) error {
	session.record("ping")
	return nil
}

func (session *recordingSession) CancelBackend(_ context.Context, pid int64, terminate bool) (bool, error) {
	session.record("cancel backend " + map[bool]string{false: "query", true: "session"}[terminate])
	return pid == 42, nil
}

func (session *recordingSession) CancelRunningQuery(context.Context) (bool, error) {
	session.record("cancel running")
	return true, nil
}

func TestTabSessionRoutesEachCall(t *testing.T) {
	calls := []string{}
	shared := &recordingSession{name: "shared", calls: &calls}
	inner := &recordingSession{name: "tab", calls: &calls, backendID: 42}
	tab := NewTabSession(shared, func(context.Context) (Session, error) { return inner, nil })
	ctx := context.Background()

	_, _ = tab.ListTables(ctx)
	_, _ = tab.CheckStatement(ctx, "select 1")
	_, _ = tab.RunQuery(ctx, "select 1", 1, nil)
	_, _ = tab.ListTables(ctx)
	inner.transaction = TransactionOpen
	_, _ = tab.ListTables(ctx)
	_, _ = tab.CheckStatement(ctx, "select 1")
	_, _ = tab.ListActivity(ctx)
	_ = tab.Ping(ctx)
	stopped, _ := tab.CancelRunningQuery(ctx)

	wanted := []string{
		"shared tables", "shared check", "tab run", "shared tables",
		"tab tables", "tab check", "shared activity", "shared ping",
		"shared cancel backend query",
	}
	if !reflect.DeepEqual(calls, wanted) {
		t.Errorf("the calls ran as\n%q\nwanted\n%q", calls, wanted)
	}
	if !stopped {
		t.Error("the cancel did not stop backend 42")
	}
}

func TestTabSessionWithoutABackendIDCancelsOnItsOwnSession(t *testing.T) {
	calls := []string{}
	shared := &recordingSession{name: "shared", calls: &calls}
	inner := &recordingSession{name: "tab", calls: &calls}
	tab := NewTabSession(shared, func(context.Context) (Session, error) { return inner, nil })
	ctx := context.Background()

	_, _ = tab.RunQuery(ctx, "select 1", 1, nil)
	_, _ = tab.CancelRunningQuery(ctx)

	if wanted := []string{"tab run", "tab cancel running"}; !reflect.DeepEqual(calls, wanted) {
		t.Errorf("the calls ran as %q, wanted %q", calls, wanted)
	}
}

func TestFindBackendIDReadsThroughTheWrappers(t *testing.T) {
	inner := &recordingSession{
		calls: &[]string{}, backendID: 7, profile: cfg.Profile{AccessMode: cfg.AccessReadOnly},
	}
	wrapped := MakeReadOnly(inner)
	if wrapped == Session(inner) {
		t.Fatal("the session was not wrapped")
	}
	if held, known := FindBackendID(wrapped); !known || held != 7 {
		t.Errorf("the backend id reads %d, %v; wanted 7", held, known)
	}
}

func (session *recordingSession) BeginTransaction(context.Context) error {
	session.record("begin")
	return nil
}

func (session *recordingSession) CheckIdleConnection(context.Context) error {
	session.record("idle check")
	return nil
}

func TestTabSessionChecksItsIdleConnectionOnceConnected(t *testing.T) {
	calls := []string{}
	shared := &recordingSession{name: "shared", calls: &calls}
	inner := &recordingSession{
		name: "tab", calls: &calls, profile: cfg.Profile{AccessMode: cfg.AccessReadOnly},
	}
	tab := NewTabSession(shared, func(context.Context) (Session, error) {
		return MakeReadOnly(inner), nil
	})
	ctx := context.Background()

	_ = tab.CheckIdleConnection(ctx)
	_ = tab.BeginTransaction(ctx)
	_ = tab.CheckIdleConnection(ctx)

	if wanted := []string{"tab begin", "tab idle check"}; !reflect.DeepEqual(calls, wanted) {
		t.Errorf("the calls ran as %q, wanted %q", calls, wanted)
	}
}

func (session *recordingSession) OpenSibling(context.Context, string) (Session, error) {
	session.record("open sibling")
	return &recordingSession{name: session.name + " sibling", calls: session.calls}, nil
}

func TestFindSiblingOpenerReadsThroughTheWrappers(t *testing.T) {
	calls := []string{}
	inner := &recordingSession{
		name: "connection", calls: &calls, profile: cfg.Profile{AccessMode: cfg.AccessReadOnly},
	}
	opener, opens := FindSiblingOpener(MakeReadOnly(inner))
	if !opens {
		t.Fatal("the wrapped session has no sibling opener")
	}
	if _, err := opener.OpenSibling(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if wanted := []string{"connection open sibling"}; !reflect.DeepEqual(calls, wanted) {
		t.Errorf("the calls ran as %q, wanted %q", calls, wanted)
	}
}

func (session *recordingSession) Close() error {
	session.record("close")
	return nil
}

// gatedOpener opens a recording session once its gate is released, and counts the opens.
type gatedOpener struct {
	calls *[]string
	gate  chan struct{}
	opens atomic.Int32
	fails bool
}

func (opener *gatedOpener) open(context.Context) (Session, error) {
	<-opener.gate
	opener.opens.Add(1)
	if opener.fails {
		opener.fails = false
		return nil, NewDatabaseError("the server refused the login")
	}
	return &recordingSession{name: "tab", calls: opener.calls}, nil
}

func TestTwoClaimsOpenOneConnection(t *testing.T) {
	calls := []string{}
	opener := &gatedOpener{calls: &calls, gate: make(chan struct{})}
	close(opener.gate)
	tab := NewTabSession(&recordingSession{name: "shared", calls: &calls}, opener.open)

	if !tab.ClaimOpen() {
		t.Fatal("the first claim was refused")
	}
	if tab.ClaimOpen() {
		t.Error("a second claim was granted while the first one runs")
	}
	if err := tab.OpenAhead(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tab.ClaimOpen() {
		t.Error("a claim was granted on a connected tab")
	}
	if opener.opens.Load() != 1 {
		t.Errorf("the tab connected %d times, wanted 1", opener.opens.Load())
	}
}

func TestARunDuringABackgroundOpenWaitsAndUsesThatSession(t *testing.T) {
	calls := []string{}
	opener := &gatedOpener{calls: &calls, gate: make(chan struct{})}
	tab := NewTabSession(&recordingSession{name: "shared", calls: &calls}, opener.open)
	tab.ClaimOpen()
	opened := make(chan error, 1)
	go func() { opened <- tab.OpenAhead(context.Background()) }()
	for tab.opening.TryLock() {
		tab.opening.Unlock()
		runtime.Gosched()
	}

	ran := make(chan error, 1)
	go func() {
		_, err := tab.RunQuery(context.Background(), "select 1", 1, nil)
		ran <- err
	}()
	close(opener.gate)
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	if err := <-ran; err != nil {
		t.Fatal(err)
	}
	if opener.opens.Load() != 1 || !reflect.DeepEqual(calls, []string{"tab run"}) {
		t.Errorf("the tab connected %d times and ran %q", opener.opens.Load(), calls)
	}
}

func TestAFailedBackgroundOpenIsTriedAgainByTheNextRun(t *testing.T) {
	calls := []string{}
	opener := &gatedOpener{calls: &calls, gate: make(chan struct{}), fails: true}
	close(opener.gate)
	tab := NewTabSession(&recordingSession{name: "shared", calls: &calls}, opener.open)

	tab.ClaimOpen()
	if err := tab.OpenAhead(context.Background()); err == nil {
		t.Fatal("the failed open reported no error")
	}
	if _, err := tab.RunQuery(context.Background(), "select 1", 1, nil); err != nil {
		t.Fatalf("the run after the failed open answered %v", err)
	}
	if opener.opens.Load() != 2 {
		t.Errorf("the tab tried to connect %d times, wanted 2", opener.opens.Load())
	}
}

func TestClosingATabDuringAnOpenClosesTheOpenedSession(t *testing.T) {
	calls := []string{}
	opener := &gatedOpener{calls: &calls, gate: make(chan struct{})}
	tab := NewTabSession(&recordingSession{name: "shared", calls: &calls}, opener.open)
	tab.ClaimOpen()
	opened := make(chan error, 1)
	go func() { opened <- tab.OpenAhead(context.Background()) }()
	for tab.opening.TryLock() {
		tab.opening.Unlock()
		runtime.Gosched()
	}

	_ = tab.Close()
	close(opener.gate)
	if err := <-opened; err == nil {
		t.Error("the open of a closed tab reported no error")
	}
	if !reflect.DeepEqual(calls, []string{"tab close"}) {
		t.Errorf("the calls ran as %q, wanted the opened session closed", calls)
	}
}

func TestBorrowedReadsRunOnTheConnectionSessionUntilTheTabConnects(t *testing.T) {
	calls := []string{}
	opener := &gatedOpener{calls: &calls, gate: make(chan struct{})}
	close(opener.gate)
	tab := NewTabSession(&recordingSession{name: "shared", calls: &calls}, opener.open)
	borrowing := tab.BorrowShared()
	ctx := context.Background()

	_, _ = borrowing.RunQuery(ctx, "select 1", 1, nil)
	tab.ClaimOpen()
	_ = tab.OpenAhead(ctx)
	_, _ = borrowing.RunQuery(ctx, "select 1", 1, nil)

	if wanted := []string{"shared run", "tab run"}; !reflect.DeepEqual(calls, wanted) {
		t.Errorf("the calls ran as %q, wanted %q", calls, wanted)
	}
}

func TestAFailedBackgroundOpenClaimsNoOtherOpen(t *testing.T) {
	calls := []string{}
	opener := &gatedOpener{calls: &calls, gate: make(chan struct{}), fails: true}
	close(opener.gate)
	tab := NewTabSession(&recordingSession{name: "shared", calls: &calls}, opener.open)

	tab.ClaimOpen()
	_ = tab.OpenAhead(context.Background())
	for range 5 {
		if tab.ClaimOpen() {
			t.Fatal("a claim was granted after a failed background open")
		}
	}
	if _, err := tab.RunQuery(context.Background(), "select 1", 1, nil); err != nil {
		t.Fatal(err)
	}
	tab.Detach()
	if !tab.ClaimOpen() {
		t.Error("a connect did not clear the failed open")
	}
}
