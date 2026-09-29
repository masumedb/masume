package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// splitView is the state of the split view: the two tabs on screen and the side with the focus.
type splitView struct {
	open bool
	// sides are the two tabs on screen: left and right, or top and bottom.
	sides [2]tabKey
	// focused is the side with the focus.
	focused int
	// stacked is true while one side is drawn over the other.
	stacked bool
	// share is the room of the first side, in thousandths of the room of both. Zero is half.
	share int
}

// resolveAside returns the side without the focus.
func (split splitView) resolveAside() int { return 1 - split.focused }

// sideArrangement is the layout of the split view on screen.
type sideArrangement int

const (
	arrangeSingle sideArrangement = iota
	arrangeAcross
	arrangeStacked
)

// The smallest side a drag of the divider leaves.
const (
	sideFloorColumns = 32
	sideFloorRows    = 2 * minPaneHeight
)

// shareScale is the room of both sides in the unit of splitView.share.
const shareScale = 1000

// sideRect is the cells one side covers.
type sideRect struct {
	left, top, width, height int
}

// sideDivider is the two borders between the sides: the last column or row of the first side
// and the first one of the second side.
type sideDivider struct {
	drawn   bool
	stacked bool
	// at is the last column or row of the first side.
	at int
	// from and to are the rows or the columns the divider covers.
	from, to int
	// start and room are the first cell and the size of both sides.
	start, room int
}

// holds returns whether the pointer stands on the divider, and on which of its two borders.
func (divider sideDivider) holds(x, y int) (int, bool) {
	if !divider.drawn {
		return 0, false
	}
	along, across := x, y
	if divider.stacked {
		along, across = y, x
	}
	if across < divider.from || across > divider.to {
		return 0, false
	}
	switch along {
	case divider.at:
		return 0, true
	case divider.at + 1:
		return 1, true
	}
	return 0, false
}

// findSide returns the connection a side shows, its position, and the tab. It returns false
// where the connection or the tab is closed.
func (model *Model) findSide(key tabKey) (*app.Connection, int, *app.Tab, bool) {
	connection, at, open := model.connections.find(key.connection)
	if !open {
		return nil, 0, nil, false
	}
	index := connection.IndexOfTab(key.tab)
	if index < 0 {
		return nil, 0, nil, false
	}
	return connection, at, connection.Tabs[index], true
}

// findActiveKey returns the key of the tab with the focus.
func (model *Model) findActiveKey() (tabKey, bool) {
	connection := model.Active()
	if connection == nil || connection.Active() == nil {
		return tabKey{}, false
	}
	return model.buildTabKey(connection, connection.Active()), true
}

// activateKey moves the focus to the tab of that key.
func (model *Model) activateKey(key tabKey) bool {
	connection, at, _, found := model.findSide(key)
	if !found {
		return false
	}
	model.connections.focus(at)
	connection.ActivateTab(connection.IndexOfTab(key.tab))
	return true
}

// openSplitView puts the tab with the focus on the first side and the next tab on the second
// side, which takes the focus. A connection with one tab opens a query tab for the second side.
func (model *Model) openSplitView(connection *app.Connection) {
	first, found := model.findActiveKey()
	if !found {
		return
	}
	if len(connection.Tabs) > 1 {
		next := connection.ActiveIndex + 1
		if next >= len(connection.Tabs) {
			next = connection.ActiveIndex - 1
		}
		connection.ActivateTab(next)
	} else {
		connection.OpenQueryTab("").Focus = app.PaneEditor
	}
	second, _ := model.findActiveKey()
	model.split.open = true
	model.split.sides = [2]tabKey{first, second}
	model.split.focused = 1
}

// focusSide moves the focus to that side of the split view.
func (model *Model) focusSide(side int) {
	if !model.split.open || side == model.split.focused {
		return
	}
	if model.activateKey(model.split.sides[side]) {
		model.split.focused = side
	}
}

// enterSide gives that side the focus until the returned function restores the previous focus.
func (model *Model) enterSide(side int) (func(), bool) {
	connection, at, _, found := model.findSide(model.split.sides[side])
	if !found {
		return nil, false
	}
	previousConnection, previousTab := model.connections.activeIndex(), connection.ActiveIndex
	previousFocus := model.split.focused
	model.connections.focus(at)
	connection.ActivateTab(connection.IndexOfTab(model.split.sides[side].tab))
	model.split.focused = side
	return func() {
		connection.ActiveIndex = previousTab
		model.connections.focus(previousConnection)
		model.split.focused = previousFocus
	}, true
}

// settleSplitView keeps the split view in step with the tab that has the focus. The side with
// the focus shows that tab. A tab or a connection the other side shows moves the focus to that
// side. A tab or a connection that closed on either side closes the split view.
func (model *Model) settleSplitView() {
	split := &model.split
	if !split.open {
		return
	}
	focused, aside := split.sides[split.focused], split.sides[split.resolveAside()]
	focusedOpen := model.holdsSide(focused)
	if !model.holdsSide(aside) {
		split.open = false
		if focusedOpen {
			model.activateKey(focused)
		}
		return
	}
	current, found := model.findActiveKey()
	if !found {
		split.open = false
		return
	}
	switch {
	case current == aside && focusedOpen:
		split.focused = split.resolveAside()
	case current.connection == aside.connection && focused.connection != aside.connection &&
		focusedOpen:
		model.activateKey(aside)
		split.focused = split.resolveAside()
	case current == aside:
		split.open = false
	case !focusedOpen && model.isSideClosed(focused):
		split.open = false
		model.activateKey(aside)
	default:
		split.sides[split.focused] = current
	}
}

// holdsSide is true while the tab of that key is open.
func (model *Model) holdsSide(key tabKey) bool {
	_, _, _, found := model.findSide(key)
	return found
}

// isSideClosed is true where the tab of that key was closed, or its connection was.
func (model *Model) isSideClosed(key tabKey) bool {
	connection, _, open := model.connections.find(key.connection)
	return !open || connection.WasClosed(key.tab)
}

// readShownSides starts the first read of a restored tab on either side.
func (model *Model) readShownSides() tea.Cmd {
	if !model.split.open || model.screen != ScreenWorking {
		return nil
	}
	commands := []tea.Cmd{}
	for side := range model.split.sides {
		restore, entered := model.enterSide(side)
		if !entered {
			continue
		}
		if _, read := model.readWhenShown(model.Active()); read != nil {
			commands = append(commands, read)
		}
		restore()
	}
	return tea.Batch(commands...)
}

// resolveSideArrangement returns the layout of the split view in the room of the panes. A split
// the room cannot hold across is stacked, and one it cannot hold either way shows the side
// with the focus alone.
func (model *Model) resolveSideArrangement(height int) sideArrangement {
	if !model.split.open {
		return arrangeSingle
	}
	across := model.width >= 2*narrowestPaneWidth
	stacked := height >= 2*sideFloorRows
	switch {
	case model.split.stacked && stacked:
		return arrangeStacked
	case across:
		return arrangeAcross
	case stacked:
		return arrangeStacked
	}
	return arrangeSingle
}

// planSides returns the cells each side covers in the room of the panes.
func (model *Model) planSides(
	arrangement sideArrangement, left, top, width, height int,
) [2]sideRect {
	switch arrangement {
	case arrangeAcross:
		first := resolveFirstSide(width, model.split.share, sideFloorColumns)
		return [2]sideRect{
			{left: left, top: top, width: first, height: height},
			{left: left + first, top: top, width: width - first, height: height},
		}
	case arrangeStacked:
		first := resolveFirstSide(height, model.split.share, sideFloorRows)
		return [2]sideRect{
			{left: left, top: top, width: width, height: first},
			{left: left, top: top + first, width: width, height: height - first},
		}
	}
	return [2]sideRect{{left: left, top: top, width: width, height: height}}
}

// resolveFirstSide returns the room of the first side. Each side keeps the floor, or half the
// room where the room is less than two floors.
func resolveFirstSide(room, share, floor int) int {
	first := room / 2
	if share > 0 {
		first = (room*share + shareScale/2) / shareScale
	}
	floor = min(floor, room/2)
	return min(max(first, floor), room-floor)
}

// buildSideDivider returns the position of the borders between the two sides.
func buildSideDivider(arrangement sideArrangement, sides [2]sideRect) sideDivider {
	switch arrangement {
	case arrangeAcross:
		return sideDivider{
			drawn: true, at: sides[0].left + sides[0].width - 1,
			from: sides[0].top, to: sides[0].top + sides[0].height - 1,
			start: sides[0].left, room: sides[0].width + sides[1].width,
		}
	case arrangeStacked:
		return sideDivider{
			drawn: true, stacked: true, at: sides[0].top + sides[0].height - 1,
			from: sides[0].left, to: sides[0].left + sides[0].width - 1,
			start: sides[0].top, room: sides[0].height + sides[1].height,
		}
	}
	return sideDivider{}
}

// joinSides puts the rows of the two sides together, the first side left of or over the
// second one.
func (model *Model) joinSides(
	arrangement sideArrangement, sides [2]sideRect, first, second []string,
) []string {
	if arrangement == arrangeStacked {
		return append(append(make([]string, 0, len(first)+len(second)), first...), second...)
	}
	return joinSideBySide(first, sides[0].width, second, model.styles.Theme.Background)
}

// paneState is the model state that drawing a pane writes. Drawing the side without the focus
// restores it.
type paneState struct {
	layout      frameLayout
	editorLeft  int
	paneTop     int
	caretRow    int
	caretColumn int
	faultRow    int
	cellsOfRows []int
	builderRows []builderRow
	cardKeys    *KeyLine
	sideLabel   string
}

func (model *Model) keepPaneState() paneState {
	return paneState{
		layout: model.layout, editorLeft: model.editorLeft, paneTop: model.paneTop,
		caretRow: model.caretRow, caretColumn: model.caretColumn, faultRow: model.faultRow,
		cellsOfRows: model.cellsOfRows, builderRows: model.builderRows,
		cardKeys: model.cardKeys, sideLabel: model.sideLabel,
	}
}

func (model *Model) restorePaneState(state paneState) {
	model.layout, model.editorLeft, model.paneTop = state.layout, state.editorLeft, state.paneTop
	model.caretRow, model.caretColumn, model.faultRow =
		state.caretRow, state.caretColumn, state.faultRow
	model.cellsOfRows, model.builderRows = state.cellsOfRows, state.builderRows
	model.cardKeys, model.sideLabel = state.cardKeys, state.sideLabel
}

// renderAside draws the side without the focus. No pane of it has the focus, and it records no
// hit boxes.
func (model *Model) renderAside(rect sideRect) []string {
	saved := model.keepPaneState()
	restore, entered := model.enterSide(model.split.resolveAside())
	if !entered {
		return nil
	}
	model.drawingAside = true
	connection := model.Active()
	rows := model.renderSide(connection, connection.Active(), rect)
	model.drawingAside = false
	restore()
	model.restorePaneState(saved)
	return rows
}

// labelSides returns the connection name of each side while the two sides show two connections,
// and empty names otherwise.
func (model *Model) labelSides() [2]string {
	first, _, _, _ := model.findSide(model.split.sides[0])
	second, _, _, _ := model.findSide(model.split.sides[1])
	if first == nil || second == nil || first == second {
		return [2]string{}
	}
	return [2]string{first.Profile().Name, second.Profile().Name}
}

// labelSideTitle returns the title with the connection name of the side before it.
func (model *Model) labelSideTitle(title string) string {
	if model.sideLabel == "" {
		return title
	}
	return " " + model.sideLabel + " ·" + ensureLeadingBlank(title)
}

func ensureLeadingBlank(title string) string {
	if strings.HasPrefix(title, " ") {
		return title
	}
	return " " + title
}

// findAsideTab returns the id of the tab of this connection on the side without the focus.
func (model *Model) findAsideTab(connection *app.Connection) (int, bool) {
	if !model.layout.divider.drawn {
		return 0, false
	}
	aside := model.split.sides[model.split.resolveAside()]
	if aside.connection != model.connections.idOf(connection) {
		return 0, false
	}
	return aside.tab, true
}

// isOnAside is true where the pointer stands on the side without the focus, off the divider.
func (model *Model) isOnAside(x, y int) bool {
	if _, onDivider := model.layout.divider.holds(x, y); onDivider {
		return false
	}
	return model.layout.divider.drawn && model.layout.aside.holds(x, y)
}

// pressSideDivider takes hold of the divider between the sides, and reports whether the press
// landed on it. A second press gives both sides the same room.
func (model *Model) pressSideDivider(mouse tea.Mouse) bool {
	if mouse.Button != tea.MouseLeft {
		return false
	}
	grab, onDivider := model.layout.divider.holds(mouse.X, mouse.Y)
	if !onDivider {
		return false
	}
	model.selection = screenSelection{}
	if model.clicks.count("side-divider", time.Now()) >= 2 {
		model.split.share = 0
		return true
	}
	model.drag.takeSideDivider(grab)
	return true
}

// dragSideDivider moves the divider between the sides to where the pointer stands.
func (model *Model) dragSideDivider(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	divider := model.layout.divider
	if !divider.drawn || divider.room < 1 {
		return model, nil
	}
	at, floor := mouse.X, sideFloorColumns
	if divider.stacked {
		at, floor = mouse.Y, sideFloorRows
	}
	floor = min(floor, divider.room/2)
	first := min(max(at-model.drag.lineGrab-divider.start+1, floor), divider.room-floor)
	model.split.share = max((first*shareScale+divider.room/2)/divider.room, 1)
	model.drag.moved = true
	return model, nil
}

// pressAside gives the focus to the side under the pointer and draws the frame again, so the
// press reads the hit boxes of that side. It reports whether the pointer was on that side.
func (model *Model) pressAside(mouse tea.Mouse) bool {
	if model.screen != ScreenWorking || model.confirm != nil {
		return false
	}
	connection := model.Active()
	if connection == nil || connection.Overlay.IsOpen() {
		return false
	}
	if !model.isOnAside(mouse.X, mouse.Y) {
		return false
	}
	if _, onList := model.layout.completionRows.holds(mouse.X, mouse.Y); onList {
		return false
	}
	model.focusSide(model.split.resolveAside())
	model.render()
	return true
}

// rollAside scrolls the side without the focus under the pointer, and keeps the focus. It
// reports whether the pointer was on that side.
func (model *Model) rollAside(
	mouse tea.Mouse, roll func(tea.Mouse) (tea.Model, tea.Cmd),
) (tea.Cmd, bool) {
	connection := model.Active()
	if model.screen != ScreenWorking || connection == nil || connection.Overlay.IsOpen() ||
		!model.isOnAside(mouse.X, mouse.Y) {
		return nil, false
	}
	restore, entered := model.enterSide(model.split.resolveAside())
	if !entered {
		return nil, false
	}
	model.render()
	_, command := roll(mouse)
	restore()
	return command, true
}

// stepSidePane moves the focus to the next pane in the order the frame draws them: the
// explorer, then the panes of the first side, then those of the second side. It reports
// whether the split view took the step.
func (model *Model) stepSidePane(connection *app.Connection, tab *app.Tab, step int) bool {
	if !model.layout.divider.drawn {
		return false
	}
	type stop struct {
		side int
		pane app.Pane
	}
	order := []stop{}
	if connection.SidebarVisible && model.layout.treeTo >= model.layout.treeFrom {
		order = append(order, stop{side: -1, pane: app.PaneSidebar})
	}
	for side, key := range model.split.sides {
		held, _, shown, found := model.findSide(key)
		if !found {
			return false
		}
		if shown.EditorVisible() {
			order = append(order, stop{side: side, pane: app.PaneEditor})
		}
		if held.ResultVisible {
			order = append(order, stop{side: side, pane: app.PaneResult})
		}
	}
	at := 0
	for index, held := range order {
		if held.side == model.split.focused && held.pane == tab.Focus {
			at = index
		}
	}
	if len(order) == 0 {
		return true
	}
	next := order[wrap(at+step, len(order))]
	if next.side < 0 {
		tab.Focus = app.PaneSidebar
		return true
	}
	model.focusSide(next.side)
	if focused := model.Active(); focused != nil && focused.Active() != nil {
		focused.Active().Focus = next.pane
	}
	return true
}

// findPanePrompt returns the prompt drawn at the foot of a pane. The side without the focus
// draws none.
func (model *Model) findPanePrompt(
	connection *app.Connection, kinds ...app.PromptKind,
) (app.Overlay, bool) {
	if model.drawingAside {
		return app.Overlay{}, false
	}
	return findPromptBar(connection, kinds...)
}

// holdsFocus is true while the pane of the tab being drawn has the focus.
func (model *Model) holdsFocus(tab *app.Tab, pane app.Pane) bool {
	return !model.drawingAside && tab.Focus == pane
}

// labelResultTitle returns the result title with the connection name before it, where the
// result is the top box of its side.
func (model *Model) labelResultTitle(tab *app.Tab, title string) string {
	if tab.EditorVisible() {
		return title
	}
	return model.labelSideTitle(title)
}

// buildSplitMenuEntry returns the row of the tab menu that splits the view or closes the split.
func (model *Model) buildSplitMenuEntry() menuEntry {
	if model.split.open {
		return menuEntry{ActionToggleSplitView, "Close the split view", "", cfg.IconColumn, true}
	}
	return menuEntry{
		ActionToggleSplitView, "Split view", "this tab beside the next one", cfg.IconColumn, true,
	}
}

// buildOpenActions returns the rows of the table menu that open the table.
func (model *Model) buildOpenActions() []app.MenuAction {
	split := app.MenuAction{
		ID: app.ObjectOpenInSplit, Label: "Open in split view",
		Detail: "beside the tab on screen", Icon: cfg.IconColumn,
	}
	if model.split.open {
		split.Detail = "on the other side"
	}
	return []app.MenuAction{
		{
			ID: app.ObjectOpen, Label: "Open", Icon: cfg.IconTable,
			Chord: model.registry.FormatFirstActionChordName(cfg.ScopeTree, ActionOpenNode),
		},
		{
			ID: app.ObjectOpenInNewTab, Label: "Open in new tab", Icon: cfg.IconNewTab,
			Chord: model.registry.FormatFirstActionChordName(cfg.ScopeTree, ActionOpenInNewTab),
		},
		split,
	}
}

// openTableBeside shows the table on the other side of the split view, and opens the split
// with the tab on screen on the first side where it is closed. The table takes the focus.
func (model *Model) openTableBeside(
	connection *app.Connection, table db.TableRef,
) (tea.Model, tea.Cmd) {
	first, found := model.findActiveKey()
	if !found {
		return model, nil
	}
	preview := connection.Session.Composer().ComposeRelationRead(
		table, core.ReadRewrite{}).Display
	tab, created := connection.OpenTableBeside(table, preview)
	shown := model.buildTabKey(connection, tab)
	if model.split.open {
		aside := model.split.resolveAside()
		model.split.sides[aside], model.split.focused = shown, aside
	} else {
		model.split.open = true
		model.split.sides = [2]tabKey{first, shown}
		model.split.focused = 1
	}
	if !created {
		return model, nil
	}
	return model.runTabRead(connection, tab)
}
