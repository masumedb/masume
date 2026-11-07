package app

import (
	"context"
	"io"
	"slices"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/notebook"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/writeplan"
)

// HealthState is the connection health status.
type HealthState string

// The three states a connection can be in.
const (
	HealthOk           HealthState = "ok"
	HealthReconnecting HealthState = "reconnecting"
	HealthDown         HealthState = "down"
)

// NoticeTone is the notice display category.
type NoticeTone string

// Notice categories. Active notices describe current state, such as staged changes.
const (
	NoticeInfo   NoticeTone = "info"
	NoticeActive NoticeTone = "active"
	NoticeError  NoticeTone = "error"
)

// Notice is a temporary status bar message.
type Notice struct {
	Text    string
	Tone    NoticeTone
	ShownAt time.Time
}

// Status bar notice lifetimes.
const (
	NoticeLife      = 4 * time.Second
	NoticeErrorLife = 8 * time.Second
)

// ReadLife returns how long this report stays in the bar.
func (notice *Notice) ReadLife() time.Duration {
	if notice.Tone == NoticeError {
		return NoticeErrorLife
	}
	return NoticeLife
}

// Catalog is the loaded server metadata.
type Catalog struct {
	// The schemas the server holds, including one that holds no relation and no object.
	Schemas []string
	Tables  []db.TableRef
	Objects []db.SchemaObject
	Roles   []db.DbRole
	// The columns of each table, once read. Keyed by the table row id.
	Details map[string]present.TableDetailState
	// The last catalog refresh time.
	ReadAt time.Time
	// True while the first read is still running.
	Loading bool
	// The catalog loading error, if any.
	Problem string
}

// NewCatalog creates an unloaded catalog.
func NewCatalog() *Catalog {
	return &Catalog{Details: map[string]present.TableDetailState{}, Loading: true}
}

// IsStale is true where the table list is old enough to read again.
func (catalog *Catalog) IsStale(now time.Time) bool {
	return catalog.ReadAt.IsZero() || now.Sub(catalog.ReadAt) > core.CatalogTTL
}

// FindTable returns the relation of that name, and whether the catalog holds it.
func (catalog *Catalog) FindTable(schema, name string) (db.TableRef, bool) {
	for _, table := range catalog.Tables {
		if table.Schema == schema && table.Name == name {
			return table, true
		}
	}
	return db.TableRef{}, false
}

// Marks is the profile favourites and recently visited schemas.
type Marks struct {
	Favourites []core.Favourite
	Recent     []core.RecentSchema
}

// ToggleFavourite marks the object, or takes the mark off.
func (marks *Marks) ToggleFavourite(favourite core.Favourite) {
	id := core.BuildFavouriteID(favourite)
	for at, held := range marks.Favourites {
		if core.BuildFavouriteID(held) == id {
			marks.Favourites = append(marks.Favourites[:at], marks.Favourites[at+1:]...)
			return
		}
	}
	marks.Favourites = append(marks.Favourites, favourite)
}

// VisitSchema updates the recent schema list.
func (marks *Marks) VisitSchema(schema string, now time.Time) {
	kept := make([]core.RecentSchema, 0, len(marks.Recent)+1)
	kept = append(kept, core.RecentSchema{Schema: schema, VisitedAt: now})
	for _, entry := range marks.Recent {
		if entry.Schema != schema {
			kept = append(kept, entry)
		}
	}
	if len(kept) > core.RecentLimit {
		kept = kept[:core.RecentLimit]
	}
	marks.Recent = kept
}

// Connection is one open connection, with the tabs and the state that belong to it.
type Connection struct {
	Session db.Session
	// OpenTabSession opens the session of one tab. With nil, every tab runs on Session.
	OpenTabSession func(ctx context.Context) (db.Session, error)
	tabSessions    map[int]*db.TabSession
	// What was started to reach the server, which is stopped with the connection.
	PreConnect *cfg.PreConnectHandle
	Catalog    *Catalog
	Marks      *Marks
	Health     HealthState
	// The last health error and consecutive failure count.
	HealthProblem  string
	HealthFailures int
	Notice         *Notice

	Tabs        []*Tab
	ActiveIndex int
	// Restored tabs awaiting their first data request.
	Unread    map[int]bool
	nextTabID int
	// The last query tab number handed out.
	queryTabCount int
	// The increasing snapshot sequence for rejecting stale saves.
	workspaceChange uint64
	// Recently closed tabs available for reopening.
	closed []*Tab

	// The first visible tab index.
	TabOffset int

	// True while the object tree is drawn. `Alt+S` gives its columns to the grid.
	SidebarVisible bool
	// The columns the object tree asks for, which a drag of its border sets. Zero asks for
	// the width the client opens with.
	SidebarWidth int
	// False while the result is hidden and the editor has the whole pane.
	ResultVisible bool
	// True when statements use autocommit mode.
	Autocommit bool

	// Undo data or an unavailable reason for the last recorded write.
	Undo *HeldUndo

	// The cell a copy or a cut of the cell list took, which a paste inserts.
	NotebookClip *NotebookCell

	// The overlay on top, which owns the keyboard while it is open.
	Overlay Overlay
	// covered holds the cards this one was opened over, the outermost first. A card raised
	// from another card returns to it when it closes.
	covered []Overlay
	// The tree of this connection: which rows are folded, and where the cursor is.
	Tree TreeState
	// The connection chat and current response state.
	Chat *Chat
	// stopExport is the active export cancellation function.
	stopExport func()
	stopImport func()
	// stopDump is the way to stop the dump or the restore that runs now.
	stopDump func()
	// The context of the statements the user runs in each tab, by tab id.
	runs map[int]tabRun
	// The writes and transaction steps that run now.
	writes []heldWrite

	// The cached object tree and its input fingerprint.
	treeAt     treeFingerprint
	treeResult present.TreeResult
	treeBuilt  bool
}

// tabRun is the context of the statements the user runs in one tab, and its cancel function.
type tabRun struct {
	ctx  context.Context
	stop context.CancelFunc
}

// heldWrite is one write or transaction step that runs now.
type heldWrite struct {
	tabID int
	label string
}

// HeldUndo is the undo of one write, and the write it reverses.
type HeldUndo struct {
	Undo  writeplan.Undo
	SQL   string
	RanAt time.Time
	// The tab the write ran in. Zero for a write on the connection session.
	TabID int
}

// KeepUndo replaces the stored undo with the latest write, including the reason when undo is unavailable.
func (connection *Connection) KeepUndo(
	tabID int, undo writeplan.Undo, sql string, now time.Time,
) {
	connection.Undo = &HeldUndo{Undo: undo, SQL: sql, RanAt: now, TabID: tabID}
}

// FindUndoTab returns the tab the undo runs in: the tab of the write, or the active tab where
// that tab is closed. A write on the connection session returns nil.
func (connection *Connection) FindUndoTab(held HeldUndo) *Tab {
	if held.TabID == 0 {
		return nil
	}
	if at := connection.IndexOfTab(held.TabID); at >= 0 {
		return connection.Tabs[at]
	}
	return connection.Active()
}

// TreeState holds the view state of the object tree, which is not application state.
type TreeState struct {
	Expanded map[string]bool
	Cursor   int
	Offset   int
	// True after scrolling independently of the cursor.
	Rolled bool
	Filter string
	// The schema restriction for the tree filter.
	FilterScope string
	// True while the filter field holds the keyboard.
	Filtering bool
	// True while the system schemas are folded away.
	HideSystemSchemas bool
	// True once the first read seeded the folds.
	seeded bool
}

// NewConnection opens the state of one connection, with one query tab.
func NewConnection(
	session db.Session, preConnect *cfg.PreConnectHandle, hideSystemSchemas bool,
) *Connection {
	connection := &Connection{
		Session: session, PreConnect: preConnect,
		Catalog: NewCatalog(), Marks: &Marks{}, Health: HealthOk,
		SidebarVisible: true, ResultVisible: true,
		Autocommit: session.Describe().Profile.Autocommit,
		Tree: TreeState{
			Expanded: map[string]bool{}, HideSystemSchemas: hideSystemSchemas,
		},
		Chat: NewChat(),
	}
	connection.nextTabID = 1
	connection.Tabs = []*Tab{connection.numberQueryTab(NewQueryTab(1, ""))}
	connection.Unread = map[int]bool{}
	return connection
}

// Profile returns the profile this connection was opened from.
func (connection *Connection) Profile() cfg.Profile {
	return connection.Session.Describe().Profile
}

// Active returns the selected tab, or nil for an empty tab list.
func (connection *Connection) Active() *Tab {
	if len(connection.Tabs) == 0 {
		return nil
	}
	return connection.Tabs[core.ClampIndex(connection.ActiveIndex, len(connection.Tabs))]
}

// BeginExport keeps the way to stop the export that starts now, and ends the one before it.
func (connection *Connection) BeginExport(stop func()) {
	connection.StopExport()
	connection.stopExport = stop
}

// EndExport forgets the export that finished.
func (connection *Connection) EndExport() { connection.stopExport = nil }

// IsExporting is true while an export streams.
func (connection *Connection) IsExporting() bool { return connection.stopExport != nil }

// StopExport ends the export that streams now, where one does.
func (connection *Connection) StopExport() {
	if connection.stopExport == nil {
		return
	}
	connection.stopExport()
	connection.stopExport = nil
}

// BeginImport replaces the active import cancellation function and stops the previous import.
func (connection *Connection) BeginImport(stop func()) {
	connection.StopImport()
	connection.stopImport = stop
}

// StopImport requests cancellation of the active import.
func (connection *Connection) StopImport() {
	if connection.stopImport == nil {
		return
	}
	connection.stopImport()
	connection.stopImport = nil
}

// BeginDump keeps the way to stop the dump or the restore that starts now, and ends the one
// before it.
func (connection *Connection) BeginDump(stop func()) {
	connection.StopDump()
	connection.stopDump = stop
}

// StopDump ends the dump or the restore that runs now, where one does.
func (connection *Connection) StopDump() {
	if connection.stopDump == nil {
		return
	}
	connection.stopDump()
	connection.stopDump = nil
}

// HasTabSessions is true where each tab runs on a session of its own.
func (connection *Connection) HasTabSessions() bool {
	return connection.OpenTabSession != nil && !connection.Profile().SharesSession &&
		connection.Session.Capabilities().OpensTabSessions
}

// ResolveTabSession returns the session the work of the tab runs on.
func (connection *Connection) ResolveTabSession(tab *Tab) db.Session {
	if held, own := connection.ResolveOwnTabSession(tab); own {
		return held
	}
	return connection.Session
}

// ResolveOwnTabSession returns the session of the tab itself, and false where the tab runs on
// the connection session.
func (connection *Connection) ResolveOwnTabSession(tab *Tab) (*db.TabSession, bool) {
	if tab == nil || !connection.HasTabSessions() {
		return nil, false
	}
	held, found := connection.tabSessions[tab.ID]
	if !found {
		held = db.NewTabSession(connection.Session, connection.OpenTabSession)
		if connection.tabSessions == nil {
			connection.tabSessions = map[int]*db.TabSession{}
		}
		connection.tabSessions[tab.ID] = held
	}
	return held, true
}

// ResolveTableReadSession returns the session the reads of a table tab run on. With
// autocommit on, they run on the connection session until the tab connects.
func (connection *Connection) ResolveTableReadSession(tab *Tab) db.Session {
	held, own := connection.ResolveOwnTabSession(tab)
	if !own {
		return connection.Session
	}
	if tab.Kind != TabTable || !connection.Autocommit {
		return held
	}
	return held.BorrowShared()
}

// FindTabSession returns the session of the tab, and false where the tab has none yet.
func (connection *Connection) FindTabSession(tab *Tab) (*db.TabSession, bool) {
	if tab == nil || !connection.HasTabSessions() {
		return nil, false
	}
	held, found := connection.tabSessions[tab.ID]
	return held, found
}

// ReadTabTransactionState returns the transaction state of the session the tab runs on. A tab
// without a session yet has no transaction.
func (connection *Connection) ReadTabTransactionState(tab *Tab) db.TransactionState {
	if tab == nil || !connection.HasTabSessions() {
		return connection.Session.ReadTransactionState()
	}
	held, found := connection.tabSessions[tab.ID]
	if !found {
		return db.TransactionNone
	}
	return held.ReadTransactionState()
}

// ListOwnBackends returns the server id of each user connection of this client, with its
// label: the tab title for a tab session, and "connection" for the connection session.
func (connection *Connection) ListOwnBackends() map[int64]string {
	own := map[int64]string{}
	if backendID, known := db.FindBackendID(connection.Session); known {
		own[backendID] = "connection"
	}
	for _, tab := range connection.Tabs {
		held, found := connection.FindTabSession(tab)
		if !found {
			continue
		}
		if backendID, known := held.ReadBackendID(); known {
			own[backendID] = "tab " + tab.Label()
		}
	}
	return own
}

// ListIdleCheckers returns the idle check of the connection session and of every tab
// session.
func (connection *Connection) ListIdleCheckers() []db.IdleChecker {
	checkers := []db.IdleChecker{}
	if checker, checks := db.FindIdleChecker(connection.Session); checks {
		checkers = append(checkers, checker)
	}
	for _, tab := range connection.Tabs {
		if held, found := connection.FindTabSession(tab); found {
			checkers = append(checkers, held)
		}
	}
	return checkers
}

// TakeTabSessions removes every tab session and returns them for closing.
func (connection *Connection) TakeTabSessions() []io.Closer {
	taken := make([]io.Closer, 0, len(connection.tabSessions))
	for _, held := range connection.tabSessions {
		taken = append(taken, held)
	}
	connection.tabSessions = nil
	return taken
}

// DetachIdleTabSessions returns for closing the connected session of every tab with no
// transaction and no running statement. The next run of such a tab connects again.
func (connection *Connection) DetachIdleTabSessions() []io.Closer {
	detached := []io.Closer{}
	for _, tab := range connection.Tabs {
		held, found := connection.tabSessions[tab.ID]
		if !found || tab.Results.IsRunning() || connection.HasTabWrite(tab) ||
			held.ReadTransactionState() != db.TransactionNone {
			continue
		}
		if inner, open := held.Detach(); open {
			detached = append(detached, inner)
		}
	}
	return detached
}

// RunContext returns the context of the statements the user runs in the tab.
func (connection *Connection) RunContext(tab *Tab) context.Context {
	id := findTabID(tab)
	held, found := connection.runs[id]
	if !found {
		held.ctx, held.stop = context.WithCancel(context.Background())
		if connection.runs == nil {
			connection.runs = map[int]tabRun{}
		}
		connection.runs[id] = held
	}
	return held.ctx
}

// StopTabRuns cancels every statement started with RunContext of the tab. The next
// RunContext is a new one.
func (connection *Connection) StopTabRuns(tab *Tab) {
	id := findTabID(tab)
	if held, found := connection.runs[id]; found {
		held.stop()
		delete(connection.runs, id)
	}
}

// StopRuns cancels every statement started with RunContext, in every tab.
func (connection *Connection) StopRuns() {
	for _, held := range connection.runs {
		held.stop()
	}
	connection.runs = nil
}

// findTabID returns the id of the tab, and zero for no tab.
func findTabID(tab *Tab) int {
	if tab == nil {
		return 0
	}
	return tab.ID
}

// BeginWrite marks a write or a transaction step of the tab as running, and returns the run
// context it runs on. StopTabRuns cancels it.
func (connection *Connection) BeginWrite(tab *Tab, label string) context.Context {
	connection.writes = append(connection.writes, heldWrite{tabID: findTabID(tab), label: label})
	return connection.RunContext(tab)
}

// EndWrite takes one running write with this label off.
func (connection *Connection) EndWrite(label string) {
	for at, held := range connection.writes {
		if held.label == label {
			connection.writes = append(connection.writes[:at], connection.writes[at+1:]...)
			return
		}
	}
}

// FindWrite returns the label of the write that runs now.
func (connection *Connection) FindWrite() (string, bool) {
	if len(connection.writes) == 0 {
		return "", false
	}
	return connection.writes[0].label, true
}

// HasTabWrite is true while a write or a transaction step of the tab runs.
func (connection *Connection) HasTabWrite(tab *Tab) bool {
	id := findTabID(tab)
	for _, held := range connection.writes {
		if held.tabID == id {
			return true
		}
	}
	return false
}

// Show displays an informational notice.
func (connection *Connection) Show(text string) {
	connection.Notice = &Notice{Text: text, Tone: NoticeInfo, ShownAt: time.Now()}
}

// ShowError reports a failure in the bar.
func (connection *Connection) ShowError(text string) {
	connection.Notice = &Notice{Text: text, Tone: NoticeError, ShownAt: time.Now()}
}

// DropInfoNotice takes a report that is not an error away.
func (connection *Connection) DropInfoNotice() {
	if connection.Notice != nil && connection.Notice.Tone == NoticeInfo {
		connection.Notice = nil
	}
}

// DropStaleNotice takes the report away once it has been on screen long enough.
func (connection *Connection) DropStaleNotice(now time.Time) {
	if connection.Notice != nil && now.Sub(connection.Notice.ShownAt) > connection.Notice.ReadLife() {
		connection.Notice = nil
	}
}

// appendTab puts the tab after the last one and moves to it.
func (connection *Connection) appendTab(tab *Tab) *Tab {
	connection.Tabs = append(connection.Tabs, tab)
	connection.ActiveIndex = len(connection.Tabs) - 1
	return tab
}

// showTab replaces a blank active tab or appends a new tab.
func (connection *Connection) showTab(tab *Tab) *Tab {
	standing := connection.Active()
	if standing == nil || !standing.IsBlank() {
		return connection.appendTab(tab)
	}
	connection.Tabs[connection.ActiveIndex] = tab
	connection.moveTabSession(standing, tab)
	return tab
}

// moveTabSession gives the session and the run context of one tab to another tab.
func (connection *Connection) moveTabSession(from, to *Tab) {
	if held, found := connection.tabSessions[from.ID]; found {
		delete(connection.tabSessions, from.ID)
		connection.tabSessions[to.ID] = held
	}
	if held, found := connection.runs[from.ID]; found {
		delete(connection.runs, from.ID)
		connection.runs[to.ID] = held
	}
}

// numberQueryTab gives the query tab the next query tab number.
func (connection *Connection) numberQueryTab(tab *Tab) *Tab {
	connection.queryTabCount++
	tab.Number = connection.queryTabCount
	return tab
}

// OpenQueryTab opens a query tab. Nonempty statements can replace a blank active tab.
func (connection *Connection) OpenQueryTab(sql string) *Tab {
	connection.nextTabID++
	tab := connection.numberQueryTab(NewQueryTab(connection.nextTabID, sql))
	if sql == "" {
		return connection.appendTab(tab)
	}
	return connection.showTab(tab)
}

// OpenNotebook opens a notebook tab. A notebook already open in a tab comes forward.
func (connection *Connection) OpenNotebook(
	book notebook.Notebook, path string, origin notebook.Origin,
) *Tab {
	if path != "" {
		for at, tab := range connection.Tabs {
			if tab.Kind == TabNotebook && tab.Notebook != nil && tab.Notebook.Path == path {
				connection.ActiveIndex = at
				return tab
			}
		}
	}
	connection.nextTabID++
	return connection.showTab(NewNotebookTab(connection.nextTabID, book, path, origin))
}

// OpenNotebookInNewTab opens a notebook in a tab of its own, even where another tab holds
// the same notebook.
func (connection *Connection) OpenNotebookInNewTab(
	book notebook.Notebook, path string, origin notebook.Origin,
) *Tab {
	connection.nextTabID++
	return connection.showTab(NewNotebookTab(connection.nextTabID, book, path, origin))
}

// OpenBuilder opens a query builder tab.
func (connection *Connection) OpenBuilder() *Tab {
	connection.nextTabID++
	return connection.showTab(NewBuilderTab(connection.nextTabID))
}

// OpenTable focuses an existing table tab or opens a new table tab.
func (connection *Connection) OpenTable(table db.TableRef, preview string) *Tab {
	for at, tab := range connection.Tabs {
		if tab.Kind == TabTable && tab.Table.Schema == table.Schema && tab.Table.Name == table.Name {
			connection.ActiveIndex = at
			return tab
		}
	}
	return connection.OpenTableInNewTab(table, preview)
}

// OpenTableInNewTab opens a separate table tab even when another tab has the same table.
func (connection *Connection) OpenTableInNewTab(table db.TableRef, preview string) *Tab {
	connection.nextTabID++
	return connection.showTab(NewTableTab(connection.nextTabID, table, preview))
}

// OpenTableBeside moves to a tab of the table other than the active tab, or appends a new one.
// It reports whether the tab is new.
func (connection *Connection) OpenTableBeside(table db.TableRef, preview string) (*Tab, bool) {
	for at, tab := range connection.Tabs {
		if at != connection.ActiveIndex && tab.Kind == TabTable &&
			tab.Table.Schema == table.Schema && tab.Table.Name == table.Name {
			connection.ActiveIndex = at
			return tab, false
		}
	}
	connection.nextTabID++
	return connection.appendTab(NewTableTab(connection.nextTabID, table, preview)), true
}

// OpenObject opens a tab that shows the definition of one schema object.
func (connection *Connection) OpenObject(object db.SchemaObject) *Tab {
	for at, tab := range connection.Tabs {
		if tab.Kind == TabObject && tab.Object.Schema == object.Schema &&
			tab.Object.Name == object.Name && tab.Object.Kind == object.Kind {
			connection.ActiveIndex = at
			return tab
		}
	}
	connection.nextTabID++
	return connection.showTab(NewObjectTab(connection.nextTabID, object))
}

// closedTabDepth is the maximum retained closed tabs, including their results.
const closedTabDepth = 10

// CloseTab closes the tab at that position, and returns the session of the tab for closing.
// The last tab cannot be closed.
func (connection *Connection) CloseTab(index int) io.Closer {
	if len(connection.Tabs) <= 1 || index < 0 || index >= len(connection.Tabs) {
		return nil
	}
	tab := connection.Tabs[index]
	connection.StopTabRuns(tab)
	var session io.Closer
	if held, found := connection.tabSessions[tab.ID]; found {
		delete(connection.tabSessions, tab.ID)
		session = held
	}
	connection.closed = append(connection.closed, tab)
	if len(connection.closed) > closedTabDepth {
		copy(connection.closed, connection.closed[1:])
		connection.closed[len(connection.closed)-1] = nil
		connection.closed = connection.closed[:len(connection.closed)-1]
	}
	connection.Tabs = append(connection.Tabs[:index], connection.Tabs[index+1:]...)
	connection.ActiveIndex = core.ClampIndex(connection.ActiveIndex, len(connection.Tabs))
	return session
}

// IndexOfTab returns where the tab of that id stands, and -1 where the connection has none.
func (connection *Connection) IndexOfTab(id int) int {
	for at, tab := range connection.Tabs {
		if tab.ID == id {
			return at
		}
	}
	return -1
}

// HasClosedTab is true where a tab was closed and can be opened again.
func (connection *Connection) HasClosedTab() bool {
	return len(connection.closed) > 0
}

// WasClosed is true for a tab of that id among the tabs that can be opened again.
func (connection *Connection) WasClosed(id int) bool {
	for _, tab := range connection.closed {
		if tab.ID == id {
			return true
		}
	}
	return false
}

// ReopenTab opens the tab closed last, with the statement it held.
func (connection *Connection) ReopenTab() bool {
	if len(connection.closed) == 0 {
		return false
	}
	tab := connection.closed[len(connection.closed)-1]
	connection.closed[len(connection.closed)-1] = nil
	connection.closed = connection.closed[:len(connection.closed)-1]
	connection.showTab(tab)
	return true
}

// ActivateTab moves to the tab at that position.
func (connection *Connection) ActivateTab(index int) {
	connection.ActiveIndex = core.ClampIndex(index, len(connection.Tabs))
}

// MoveTab moves the tab at from to the position to. The active tab stays active.
func (connection *Connection) MoveTab(from, to int) {
	count := len(connection.Tabs)
	if from < 0 || from >= count || to < 0 || to >= count || from == to {
		return
	}
	active := connection.Tabs[connection.ActiveIndex]
	moved := connection.Tabs[from]
	connection.Tabs = slices.Delete(connection.Tabs, from, from+1)
	connection.Tabs = slices.Insert(connection.Tabs, to, moved)
	connection.ActiveIndex = slices.Index(connection.Tabs, active)
}

// StepTab moves to the tab before or after the one on screen, and wraps at the ends.
func (connection *Connection) StepTab(step int) {
	connection.ActiveIndex = core.WrapIndex(connection.ActiveIndex+step, len(connection.Tabs))
}

// SeedTree opens the folds a first read of the catalog asks for.
func (connection *Connection) SeedTree() {
	if connection.Tree.seeded {
		return
	}
	if len(connection.Catalog.Tables) == 0 && len(connection.Catalog.Objects) == 0 {
		return
	}
	connection.Tree.seeded = true
	connection.Tree.Expanded = present.CollectDefaultExpanded(
		connection.Catalog.Tables, connection.Catalog.Objects,
		connection.Marks.Favourites, connection.Marks.Recent)
}

// CompletionSources returns schema, table, and routine suggestions from the connection catalog.
func (connection *Connection) CompletionSources() (schemas, tables, functions []string) {
	offered := func(schema string) bool {
		if !connection.Tree.HideSystemSchemas {
			return true
		}
		return !present.IsSystemSchema(schema, connection.Profile().Engine)
	}

	seenSchemas := map[string]bool{}
	for _, table := range connection.Catalog.Tables {
		if !offered(table.Schema) {
			continue
		}
		if !seenSchemas[table.Schema] {
			seenSchemas[table.Schema] = true
			schemas = append(schemas, table.Schema)
		}
		// Include qualified and unqualified table names.
		tables = append(tables, table.Name, table.Schema+"."+table.Name)
	}
	slices.Sort(schemas)

	for _, object := range connection.Catalog.Objects {
		if object.Kind != db.ObjectFunction || !offered(object.Schema) {
			continue
		}
		// Include qualified and unqualified routine names.
		functions = append(functions, object.Name, object.Schema+"."+object.Name)
	}
	return schemas, tables, functions
}

// Open opens a card in place of whatever is on show, and of everything under it. It is what
// a pane, a menu or a finished piece of work opens a card with.
func (connection *Connection) Open(overlay Overlay) {
	connection.covered = nil
	connection.Overlay = overlay
}

// OpenOver opens a card over the one on show, which returns when this one closes. It is what
// a card raises another card with. A card opened over nothing has nothing to return to.
func (connection *Connection) OpenOver(overlay Overlay) {
	// A card of the same kind takes the place of the one on show, so opening one twice
	// stacks nothing.
	if connection.Overlay.IsOpen() && connection.Overlay.Kind != overlay.Kind {
		connection.covered = append(connection.covered, connection.Overlay)
	}
	connection.Overlay = overlay
}

// CloseOverlay closes the card on show and returns to the one it was opened over, or to the
// workspace where it covered nothing.
func (connection *Connection) CloseOverlay() {
	at := len(connection.covered) - 1
	if at < 0 {
		connection.Overlay = Overlay{}
		return
	}
	connection.Overlay = connection.covered[at]
	connection.covered = connection.covered[:at]
}

// CloseEveryOverlay closes the card on show and every card under it.
func (connection *Connection) CloseEveryOverlay() {
	connection.covered = nil
	connection.Overlay = Overlay{}
}

// CoversAnotherOverlay is true where the card on show was opened over another one.
func (connection *Connection) CoversAnotherOverlay() bool {
	return len(connection.covered) > 0
}
