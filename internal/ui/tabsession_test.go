package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/engines"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/writeplan"
)

// openTabSessionModel returns a model on a SQLite connection whose tabs open sessions of
// their own, and the count of the tab sessions opened.
func openTabSessionModel(t *testing.T, database string, shares bool) (*Model, *atomic.Int32) {
	t.Helper()
	if database == "" {
		database = filepath.Join(t.TempDir(), "tabs.db")
		if err := os.WriteFile(database, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	profile := cfg.Profile{
		Name: "tabs", Engine: core.EngineSqlite, Database: database,
		AccessMode: cfg.AccessWrite, PageSize: 100, Autocommit: true,
		WritePlan: cfg.PlanOff, ConfirmWrites: cfg.ConfirmOff, SharesSession: shares,
	}
	adapters := engines.CreateAdapters()
	session, err := adapters.Open(context.Background(), profile, "")
	if err != nil {
		t.Fatal(err)
	}
	opens := &atomic.Int32{}
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Session = session
	connection.OpenTabSession = func(ctx context.Context) (db.Session, error) {
		opens.Add(1)
		return adapters.Open(ctx, profile, "")
	}
	connection.Tabs = append(connection.Tabs, app.NewQueryTab(connection.Tabs[0].ID+1, ""))
	t.Cleanup(func() {
		for _, held := range connection.TakeTabSessions() {
			_ = held.Close()
		}
		_ = session.Close()
	})
	if _, err := session.RunQuery(context.Background(),
		"create table entries (id integer primary key)", 100, nil); err != nil {
		t.Fatal(err)
	}
	return model, opens
}

// runTabStatement runs one statement in the tab and returns its answer.
func runTabStatement(t *testing.T, connection *app.Connection, tab *app.Tab, sql string) queryRanMsg {
	t.Helper()
	session := connection.ResolveTabSession(tab)
	read := session.Composer().ComposeStatementRead(db.BoundText{Text: sql}, core.ReadRewrite{})
	return runStatements(connection.RunContext(tab), 1, tab.ID, 1, 0, session,
		[]db.ComposedRead{read}, 100, writeplan.UndoPlan{}, nil, "tabs", true)().(queryRanMsg)
}

func TestTwoTabsRunOnTwoSessions(t *testing.T) {
	model, opens := openTabSessionModel(t, "", false)
	connection := model.Active()
	first, second := connection.Tabs[0], connection.Tabs[1]

	if opens.Load() != 0 {
		t.Fatalf("%d tab sessions opened before a run, wanted none", opens.Load())
	}
	_, command := model.runTransaction(connection, first, ActionBeginTransaction)
	if answered := command().(transactionRanMsg); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if answered := runTabStatement(t, connection, second, "select 1"); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}

	if opens.Load() != 2 {
		t.Errorf("%d tab sessions opened, wanted 2", opens.Load())
	}
	if connection.ResolveTabSession(first).ReadTransactionState() != db.TransactionOpen {
		t.Error("the first tab holds no transaction")
	}
	if connection.ResolveTabSession(second).ReadTransactionState() != db.TransactionNone {
		t.Error("the second tab shares the transaction of the first tab")
	}
	if connection.Session.ReadTransactionState() != db.TransactionNone {
		t.Error("the connection session holds the transaction of a tab")
	}
}

func TestTableColumnsAndCatalogReadOnTheConnectionSession(t *testing.T) {
	model, opens := openTabSessionModel(t, "", false)
	connection := model.Active()
	tab := connection.OpenTable(db.TableRef{
		Schema: "main", Name: "entries", Kind: db.RelationTable,
	}, "")

	_, readColumns := model.runTabRead(connection, tab)
	if readColumns == nil {
		t.Fatal("the table tab asked for no columns")
	}
	if answered := readColumns().(tableDetailMsg); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if answered := readCatalog(model.ActiveID(), connection.Session, quietCatalogRead)().(catalogReadMsg); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if opens.Load() != 0 {
		t.Errorf("%d tab sessions opened for the columns and the catalog, wanted none", opens.Load())
	}
}

func TestCancelInOneTabLeavesTheOtherTabRunning(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	first, second := connection.Tabs[0], connection.Tabs[1]

	first.Results.Start([]string{"slow"}, 100)
	slow := make(chan queryRanMsg, 1)
	go func() {
		slow <- runTabStatement(t, connection, first, "with recursive n(i) as (select 1 "+
			"union all select i + 1 from n where i < 1000000000) select count(*) from n")
	}()
	time.Sleep(100 * time.Millisecond)

	quick := make(chan queryRanMsg, 1)
	go func() { quick <- runTabStatement(t, connection, second, "select 1") }()
	select {
	case answered := <-quick:
		if answered.Problem != "" {
			t.Fatal(answered.Problem)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second tab waited for the statement of the first tab")
	}

	connection.ActivateTab(1)
	model.cancelQuery(connection)
	select {
	case answered := <-slow:
		t.Fatalf("the cancel in the second tab stopped the first tab: %q", answered.Problem)
	case <-time.After(200 * time.Millisecond):
	}

	connection.ActivateTab(0)
	model.cancelQuery(connection)
	select {
	case answered := <-slow:
		if answered.Problem == "" {
			t.Error("the cancelled statement reported no problem")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancel in the first tab did not stop its statement")
	}
}

func TestClosingATabWithAnOpenTransactionAsksAndRollsBack(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	connection.ActivateTab(1)
	tab := connection.Active()
	_, command := model.runTransaction(connection, tab, ActionBeginTransaction)
	if answered := command().(transactionRanMsg); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if answered := runTabStatement(t, connection, tab, "insert into entries values (1)"); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}

	if _, closed := model.requestCloseTab(connection); closed != nil {
		t.Fatal("the tab closed without a question")
	}
	if connection.Overlay.Kind != app.OverlayConfirm || len(connection.Tabs) != 2 {
		t.Fatalf("the card is %q with %d tabs, wanted a question and 2 tabs",
			connection.Overlay.Kind, len(connection.Tabs))
	}
	started := connection.Overlay.Answers.Answer(true)
	if len(connection.Tabs) != 1 {
		t.Fatalf("%d tabs are open after the answer, wanted 1", len(connection.Tabs))
	}
	if started == nil {
		t.Fatal("the answer started nothing, so the tab session was never closed")
	}
	started()

	counted, err := connection.Session.RunQuery(context.Background(),
		"select count(*) from entries", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if held := counted.Rows[0][0]; held != int64(0) {
		t.Errorf("the table holds %v rows after the close, wanted the insert rolled back", held)
	}
}

func TestSharedSessionKeepsEveryTabOnTheConnectionSession(t *testing.T) {
	model, opens := openTabSessionModel(t, "", true)
	connection := model.Active()

	for _, tab := range connection.Tabs {
		if answered := runTabStatement(t, connection, tab, "select 1"); answered.Problem != "" {
			t.Fatal(answered.Problem)
		}
		if connection.ResolveTabSession(tab) != connection.Session {
			t.Errorf("tab %d runs on a session of its own", tab.ID)
		}
	}
	if opens.Load() != 0 {
		t.Errorf("%d tab sessions opened, wanted none", opens.Load())
	}
}

func TestMemoryDatabaseKeepsEveryTabOnTheConnectionSession(t *testing.T) {
	model, opens := openTabSessionModel(t, ":memory:", false)
	connection := model.Active()

	for _, tab := range connection.Tabs {
		if answered := runTabStatement(t, connection, tab, "select count(*) from entries"); answered.Problem != "" {
			t.Fatal(answered.Problem)
		}
		if connection.ResolveTabSession(tab) != connection.Session {
			t.Errorf("tab %d runs on a session of its own", tab.ID)
		}
	}
	if opens.Load() != 0 {
		t.Errorf("%d tab sessions opened, wanted none", opens.Load())
	}
}

func TestUndoRunsOnTheTabOfTheWrite(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	first, second := connection.Tabs[0], connection.Tabs[1]
	_, command := model.runTransaction(connection, first, ActionBeginTransaction)
	if answered := command().(transactionRanMsg); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	connection.KeepUndo(first.ID, writeplan.Undo{Changes: []db.Change{{
		Payload: query.BoundStatement{SQL: "insert into entries values (5)"},
	}}}, "delete from entries where id = 5", time.Now())
	connection.ActivateTab(1)

	if answered := applyUndo(connection, 1, *connection.Undo)().(undoWrittenMsg); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if _, found := connection.FindTabSession(second); found {
		t.Error("the undo opened a session for the active tab")
	}
	counted := runTabStatement(t, connection, first, "select count(*) from entries")
	if counted.Problem != "" {
		t.Fatal(counted.Problem)
	}
	if held := counted.Result.Rows[0][0]; held != int64(1) {
		t.Errorf("the tab of the write reads %v rows, wanted the undo inside its transaction", held)
	}
}

func TestUndoOfAClosedTabRunsInTheActiveTab(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	held := app.HeldUndo{TabID: connection.Tabs[1].ID + 10}

	if tab := connection.FindUndoTab(held); tab != connection.Active() {
		t.Errorf("the undo of a closed tab runs in tab %v, wanted the active tab", tab)
	}
	if tab := connection.FindUndoTab(app.HeldUndo{}); tab != nil {
		t.Errorf("the undo of a write on the connection session runs in tab %d", tab.ID)
	}
}

func TestReconnectKeepsTheSessionOfATabThatWrites(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	tab := connection.Tabs[0]
	if answered := runTabStatement(t, connection, tab, "select 1"); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}

	connection.BeginWrite(tab, saveWriteLabel)
	if detached := connection.DetachIdleTabSessions(); len(detached) != 0 {
		t.Errorf("%d sessions detached under a running write, wanted none", len(detached))
	}
	connection.EndWrite(saveWriteLabel)
	detached := connection.DetachIdleTabSessions()
	if len(detached) != 1 {
		t.Fatalf("%d sessions detached after the write, wanted 1", len(detached))
	}
	_ = detached[0].Close()
}

// backendSession is an offline session with a server id.
type backendSession struct {
	*offlineSession
	backendID int64
}

func (session *backendSession) ReadBackendID() (int64, bool) { return session.backendID, true }

func (session *backendSession) RunQuery(context.Context, string, int, []any) (db.QueryResult, error) {
	return db.QueryResult{}, nil
}

func TestDashboardMarksTheSessionsOfThisClient(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	offline := connection.Session.(*offlineSession)
	offline.capabilities.OpensTabSessions = true
	connection.Session = &backendSession{offlineSession: offline, backendID: 7}
	connection.OpenTabSession = func(context.Context) (db.Session, error) {
		return &backendSession{offlineSession: offline, backendID: 9}, nil
	}
	tab := connection.Active()
	if _, err := connection.ResolveTabSession(tab).RunQuery(context.Background(), "select 1", 1, nil); err != nil {
		t.Fatal(err)
	}

	sessions := []db.Activity{
		{PID: 7, User: "shop", ApplicationName: "masume"},
		{PID: 9, User: "shop", ApplicationName: "masume"},
		{PID: 11, User: "ada", ApplicationName: "psql"},
	}
	model.readActivityAnswer(activityReadMsg{ConnectionID: model.ActiveID(), Sessions: sessions})

	own := connection.Overlay.Server.Own
	if others := app.CountOtherSessions(connection.Overlay.Sessions, own); others != 1 {
		t.Errorf("the dashboard counts %d other sessions, wanted 1", others)
	}
	for at, wanted := range []string{"connection", "tab " + tab.Label(), "ada@psql"} {
		if written := describeActivityUser(sessions[at], own); written != wanted {
			t.Errorf("session %d reads %q, wanted %q", sessions[at].PID, written, wanted)
		}
	}
}

func TestUndoReadsEveryTableTabOfTheTableAgain(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	orders := db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}
	connection.Active().Editor.SetText("select 1")
	shown := connection.OpenTableInNewTab(orders, "")
	other := connection.OpenTableInNewTab(orders, "")
	connection.ActivateTab(0)

	model.readUndoAnswer(undoWrittenMsg{ConnectionID: model.ActiveID(), Table: orders, Rows: 1})

	if !connection.Unread[shown.ID] || !connection.Unread[other.ID] {
		t.Fatalf("the table tabs are not marked for a read: %v", connection.Unread)
	}
	connection.ActivateTab(connection.IndexOfTab(shown.ID))
	if _, read := model.readWhenShown(connection); read == nil {
		t.Error("the table tab was not read again when it was shown")
	}
	if connection.Unread[shown.ID] {
		t.Error("the shown tab is still marked for a read")
	}
}

// cancelCountingSession counts the cancels it takes.
type cancelCountingSession struct {
	*offlineSession
	cancels int
}

func (session *cancelCountingSession) CancelRunningQuery(context.Context) (bool, error) {
	session.cancels++
	return true, nil
}

func TestCancelInATabThatRunsNothingSendsNothing(t *testing.T) {
	for _, shared := range []bool{false, true} {
		model := buildOfflineModel(t, 160, 48)
		connection := model.Active()
		offline := connection.Session.(*offlineSession)
		offline.capabilities.CancelsRunningQuery = true
		offline.capabilities.OpensTabSessions = !shared
		session := &cancelCountingSession{offlineSession: offline}
		connection.Session = session
		connection.OpenTabSession = func(context.Context) (db.Session, error) { return session, nil }
		connection.Tabs = append(connection.Tabs, app.NewQueryTab(connection.Tabs[0].ID+1, ""))
		connection.Tabs[0].Results.Start([]string{"select pg_sleep(30)"}, 100)
		connection.ActivateTab(1)

		_, command := model.cancelQuery(connection)
		if shared {
			if command == nil {
				t.Fatal("the cancel on a shared session sent nothing while another tab runs")
			}
			command()
			if session.cancels != 1 {
				t.Errorf("the shared session took %d cancels, wanted 1", session.cancels)
			}
			continue
		}
		if command != nil || session.cancels != 0 {
			t.Errorf("the idle tab sent a cancel")
		}
		if connection.Notice == nil || connection.Notice.Text != "nothing to cancel" {
			t.Errorf("the bar reads %+v, wanted nothing to cancel", connection.Notice)
		}
	}
}

func TestTheActiveTabOpensItsSessionInTheBackground(t *testing.T) {
	model, opens := openTabSessionModel(t, "", false)
	connection := model.Active()
	first, second := connection.Tabs[0], connection.Tabs[1]

	open := model.openActiveTabSession()
	if open == nil {
		t.Fatal("the active tab started no open")
	}
	open()
	if again := model.openActiveTabSession(); again != nil {
		t.Error("a connected tab started a second open")
	}
	if _, found := connection.FindTabSession(second); found {
		t.Error("a tab that was never shown has a session")
	}

	connection.ActivateTab(1)
	open = model.openActiveTabSession()
	if open == nil {
		t.Fatal("the tab shown for the first time started no open")
	}
	open()
	if opens.Load() != 2 {
		t.Errorf("%d tab sessions opened, wanted 2", opens.Load())
	}
	for _, tab := range []*app.Tab{first, second} {
		held, _ := connection.FindTabSession(tab)
		if held.ClaimOpen() {
			t.Errorf("tab %d is not connected", tab.ID)
		}
	}
}

func TestATableTabStartsNoOpenWhenShown(t *testing.T) {
	model, opens := openTabSessionModel(t, "", false)
	connection := model.Active()
	connection.Active().Editor.SetText("select 1")
	connection.OpenTableInNewTab(db.TableRef{
		Schema: "main", Name: "entries", Kind: db.RelationTable,
	}, "")

	if open := model.openActiveTabSession(); open != nil {
		t.Error("the table tab started an open on show")
	}
	if opens.Load() != 0 {
		t.Errorf("%d tab sessions opened, wanted none", opens.Load())
	}
}

func TestATableTabReadsOnTheConnectionSessionOnlyWithAutocommit(t *testing.T) {
	for _, autocommit := range []bool{true, false} {
		model, opens := openTabSessionModel(t, "", false)
		connection := model.Active()
		connection.Autocommit = autocommit
		tab := connection.OpenTableInNewTab(db.TableRef{
			Schema: "main", Name: "entries", Kind: db.RelationTable,
		}, "")

		if _, err := connection.ResolveTableReadSession(tab).RunQuery(context.Background(),
			"select count(*) from entries", 100, nil); err != nil {
			t.Fatal(err)
		}
		wanted := int32(1)
		if autocommit {
			wanted = 0
		}
		if opens.Load() != wanted {
			t.Errorf("autocommit %v: the read opened %d tab sessions, wanted %d",
				autocommit, opens.Load(), wanted)
		}
	}
}

func TestTabOpensRunThePasswordSourceOnlyAfterAFailure(t *testing.T) {
	resolved := 0
	accepted := "old"
	used := []string{}
	opener := &tabSessionOpener{
		password: "old",
		resolve: func(string) (string, error) {
			resolved++
			return "new", nil
		},
		open: func(_ context.Context, password string) (db.Session, error) {
			used = append(used, password)
			if password != accepted {
				return nil, db.NewDatabaseError("password authentication failed")
			}
			return &offlineSession{}, nil
		},
	}
	ctx := context.Background()

	for range 3 {
		if _, err := opener.openTab(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if resolved != 0 {
		t.Errorf("3 tab opens ran the password source %d times, wanted 0", resolved)
	}

	accepted = "new"
	if _, err := opener.openTab(ctx); err != nil {
		t.Fatalf("the open after the password changed answered %v", err)
	}
	if _, err := opener.openTab(ctx); err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Errorf("the password source ran %d times, wanted 1", resolved)
	}
	wanted := []string{"old", "old", "old", "old", "new", "new"}
	if strings.Join(used, " ") != strings.Join(wanted, " ") {
		t.Errorf("the opens used %q, wanted %q", used, wanted)
	}
}

func TestAFailedBackgroundOpenStartsNoOtherOpenUntilARun(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	opens := 0
	open := connection.OpenTabSession
	connection.OpenTabSession = func(ctx context.Context) (db.Session, error) {
		opens++
		if opens == 1 {
			return nil, db.NewDatabaseError("too many clients")
		}
		return open(ctx)
	}

	model.openActiveTabSession()()
	for range 5 {
		if again := model.openActiveTabSession(); again != nil {
			t.Fatal("a failed background open started another open")
		}
	}
	if opens != 1 {
		t.Fatalf("%d opens ran, wanted 1", opens)
	}
	if answered := runTabStatement(t, connection, connection.Active(), "select 1"); answered.Problem != "" {
		t.Fatal(answered.Problem)
	}
	if opens != 2 {
		t.Errorf("%d opens ran after the run, wanted 2", opens)
	}
}

func TestTheEditorStaysFastWhileTheTabConnects(t *testing.T) {
	model, _ := openTabSessionModel(t, "", false)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneEditor
	entered, release := make(chan struct{}), make(chan struct{})
	open := connection.OpenTabSession
	connection.OpenTabSession = func(ctx context.Context) (db.Session, error) {
		close(entered)
		<-release
		return open(ctx)
	}
	defer close(release)

	opened := make(chan struct{})
	go func() {
		model.openActiveTabSession()()
		close(opened)
	}()
	<-entered

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
			_ = model.View()
		}
		_ = connection.ReadTabTransactionState(tab)
		_ = connection.ListOwnBackends()
		tab.Results.Start([]string{"select 1"}, 100)
		if _, cancel := model.cancelQuery(connection); cancel != nil {
			cancel()
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the UI waited for the tab to connect")
	}
	if !strings.Contains(tab.Editor.Text, strings.Repeat("a", 50)) {
		t.Errorf("the editor holds %q, wanted 50 typed letters", tab.Editor.Text)
	}
	select {
	case <-opened:
		t.Error("the background open finished before the test released it")
	default:
	}
}
