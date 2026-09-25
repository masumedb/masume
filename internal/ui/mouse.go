package ui

import (
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// Where each part of the frame was drawn last, so a press can be read as a press on a row,
// a cell or a tab. The layout is written while the frame is drawn, because only the render
// knows how wide a column came out.

// tabHit is one tab of the row: the cells it covers, and the cells of its close mark.
type tabHit struct {
	index     int
	from, to  int
	closeFrom int
	closeTo   int
}

// columnHit is one column of the grid: the cells it covers, and which column it is.
type columnHit struct {
	index    int
	from, to int
}

// findColumnUnder returns the column of the grid the pointer stands over, and whether it
// stands over one at all.
func findColumnUnder(columns []columnHit, x int) (int, bool) {
	for _, column := range columns {
		if x >= column.from && x <= column.to {
			return column.index, true
		}
	}
	return 0, false
}

// holds returns whether the pointer stands on this range of a row.
func (span columnHit) holds(x, y, row int) bool {
	return span.to >= span.from && y == row && x >= span.from && x <= span.to
}

// rowsHit is a block of rows drawn one item per row.
type rowsHit struct {
	// top is the screen row of the first item, counted from zero.
	top int
	// count is how many items were drawn.
	count int
	// offset is the item the first drawn row holds.
	offset int
	// from and to are the cells the block covers.
	from, to int
	// gap is the item that has a heading row drawn above it, or zero for none. The heading
	// row is counted in count.
	gap int
	// skip is the items under the heading that are scrolled out of view.
	skip int
}

// holds returns which item a press landed on.
func (block rowsHit) holds(x, y int) (int, bool) {
	if block.count <= 0 || y < block.top || y >= block.top+block.count ||
		x < block.from || x > block.to {
		return 0, false
	}
	item := y - block.top
	if block.gap > 0 && item >= block.gap {
		if item == block.gap {
			return 0, false
		}
		item += block.skip - 1
	}
	return block.offset + item, true
}

// blockRect is the cells of one block of the frame that holds text of its own, such as the
// inside of a pane or of a card. A drag stays inside the block it began in, so a selection
// never carries the border of a pane or the rows of another one.
type blockRect struct {
	fromX, toX int
	fromY, toY int
}

// holds returns whether the cell is inside this block.
func (block blockRect) holds(x, y int) bool {
	return x >= block.fromX && x <= block.toX && y >= block.fromY && y <= block.toY
}

// frameLayout is where the parts of the last frame were drawn.
type frameLayout struct {
	// The row of the tab row, and the tabs on it.
	tabRow int
	tabs   []tabHit

	// The tree pane, and the rows inside it.
	treeFrom, treeTo int
	connections      rowsHit
	treeRows         rowsHit
	// The rows of the cell list of a notebook, so a press reaches the cell it drew.
	cellRows rowsHit

	// The screen row the body of a view of the result starts on, which is under the strips
	// the pane draws above it.
	detailTop int
	// documentRows is how many rows the document tree drew, which the lookahead of its
	// paging is measured against.
	documentRows int
	// documentRowsHit is where the rows of the document tree were drawn, so a press lands
	// on the row it looks like.
	documentRowsHit rowsHit

	// The rows each pane covers, so a press moves the keyboard to it.
	editorTop, editorRows int
	resultTop, resultRows int

	// Where the text of the statement is drawn, and which part of it is on show, so a press
	// of the pointer can be read as an offset in the buffer.
	editorTextLeft, editorTextTop       int
	editorTextWidth, editorTextRows     int
	editorFirstLine, editorColumnOffset int

	// The grid: the rows, the columns beside the gutter, and the row of names above them.
	gridRows    rowsHit
	gridColumns []columnHit
	// The border after each column, which a drag on the row of names sets its width by.
	columnEdges   []columnHit
	gridHeaderRow int

	// The marks of the tab row that step to the tab before or after the ones on screen.
	scrollTabsBack, scrollTabsOn columnHit

	// The close mark of a row of the connection list, which every row draws in the same
	// columns.
	closeConnectionFrom, closeConnectionTo int

	// The chips of the strips of the result: the statements, and the views under them.
	statementChips []chipHit
	viewChips      []chipHit

	// The rows of a card of a screen, and of the list inside an overlay.
	pickerRows       rowsHit
	builderRows      rowsHit
	overlayRows      rowsHit
	settingsRows     rowsHit
	settingsSections rowsHit

	// The rows of the fields of a form: the connection form, and the export card.
	formRows rowsHit
	// The marks a field that steps through a list of values draws on each side of it.
	formChoices []choiceHit
	// The rows of the list that stands over the statement, so a press takes a candidate.
	completionRows rowsHit

	// Where the body of a card was drawn, so a renderer records a chip or a row of its
	// own from the line it wrote.
	cardBodyTop, cardBodyLeft int
	// The lines a card that scrolls without a cursor holds, and the rows it shows of
	// them, so a key that scrolls it stops where the lines run out.
	cardLines, cardBody int
	// The columns a diagram card shows.
	cardRoom int
	// The chips a card draws as returns, such as the two of a question.
	overlayChips []chipHit

	// The blocks a drag can select inside, the one on top first.
	selectionBlocks []blockRect

	// The row of the title bar and the row of the status bar, so a test can read them back.
	titleRow int
	hintRow  int
	// The scroll bars the frame drew, so a press or a drag on one moves the view it stands
	// for. The bar on top comes first, because a card is drawn over a pane.
	scrollbars []scrollHit

	// Every key a renderer drew as a word a reader can press: the two bars, the row that
	// reports a fault, and the marks of the banner. A press on one runs the action the key
	// itself runs.
	buttons []buttonHit
}

// scrollHit is where one scroll bar was drawn: the column it stands in, the rows of its
// track, how many rows the view holds, and how to move it.
type scrollHit struct {
	column int
	top    int
	rows   int
	total  int
	// offset is the first row the view showed when the bar was drawn, so a press on the
	// thumb finds where the thumb stands.
	offset int
	moveTo func(offset int) tea.Cmd
}

// holds returns whether the pointer stands on the track of this bar.
func (bar scrollHit) holds(x, y int) bool {
	return x == bar.column && y >= bar.top && y < bar.top+bar.rows
}

// findScrollbar returns the bar the pointer stands on, and whether it stands on one. The
// bars are read in the order they were drawn, so a card wins the column of a pane under it.
func findScrollbar(bars []scrollHit, x, y int) (scrollHit, bool) {
	for _, bar := range slices.Backward(bars) {
		if bar.holds(x, y) {
			return bar, true
		}
	}
	return scrollHit{}, false
}

// buttonHit is one key drawn as a word: the cells it covers, and what a press on it runs. A
// key of a pair holds both actions, and the half that was pressed decides which one.
type buttonHit struct {
	row      int
	from, to int
	// keyTo is the last cell of the key itself, before its label. A press on the first half
	// of a pair runs the first action.
	keyTo  int
	scope  cfg.KeyScope
	action ActionID
	second ActionID
}

// recordButton keeps the cells one key covers, so a press on the word runs the action the
// key itself runs.
func (model *Model) recordButton(row, from, width int, scope cfg.KeyScope, action ActionID) {
	if width < 1 {
		return
	}
	model.layout.buttons = append(model.layout.buttons, buttonHit{
		row: row, from: from, to: from + width - 1, scope: scope, action: action,
	})
}

// findButton returns what a press runs, the cells the key covers, and whether it landed on a
// key at all.
func findButton(buttons []buttonHit, x, y int) (cfg.KeyScope, ActionID, buttonHit, bool) {
	for _, held := range buttons {
		if y != held.row || x < held.from || x > held.to {
			continue
		}
		if held.second != "" && x > (held.from+held.keyTo)/2 {
			return held.scope, held.second, held, true
		}
		return held.scope, held.action, held, true
	}
	return "", "", buttonHit{}, false
}

// findSelectionBlock returns the block a drag that began on this cell stays inside, and
// whether it began in one.
func (layout frameLayout) findSelectionBlock(x, y int) (blockRect, bool) {
	for _, block := range layout.selectionBlocks {
		if block.holds(x, y) {
			return block, true
		}
	}
	return blockRect{}, false
}

// choiceHit is where the two marks of a field that steps through a list of values were drawn,
// so a press on one steps the field. The mark that steps back stands before the value and the
// one that steps on stands after it.
type choiceHit struct {
	field    int
	row      int
	back, on int
}

// findChoiceMark returns which field a press steps and which way, and whether it landed on a
// mark at all.
func findChoiceMark(marks []choiceHit, x, y int) (int, int, bool) {
	for _, held := range marks {
		if y != held.row {
			continue
		}
		switch x {
		case held.back:
			return held.field, -1, true
		case held.on:
			return held.field, 1, true
		}
	}
	return 0, 0, false
}

// chipHit is where one chip of a strip was drawn, so a press lands on the chip it looks like.
type chipHit struct {
	index    int
	row      int
	from, to int
}

// findChip returns the chip the pointer stands on, and whether it stands on one.
func findChip(chips []chipHit, x, y int) (int, bool) {
	for _, held := range chips {
		if y == held.row && x >= held.from && x <= held.to {
			return held.index, true
		}
	}
	return 0, false
}

// readMouse returns what one press of a button does. A press that lit a key asks for a frame
// at the moment the light runs out, so the light lasts as long as it says and not until the
// next turn of the wheel.
func (model *Model) readMouse(press tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	next, command := model.readPress(press)
	if model.frame.isFlashing() {
		return next, tea.Batch(command, wake(keyFlashWait))
	}
	return next, command
}

// readPress returns what one press does, before the light of a key is taken into account.
func (model *Model) readPress(press tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := press.Mouse()
	switch mouse.Button {
	case tea.MouseLeft, tea.MouseRight:
	case tea.MouseMiddle:
		// The middle button closes a tab, which is what it does on the tabs of a browser
		// and of every terminal that has them.
		return model.pressMiddleButton(mouse)
	default:
		return model, nil
	}

	// A press with the left button begins a drag, and lets the last selection go. The
	// press itself still acts, because a drag of one cell is a press.
	if mouse.Button == tea.MouseLeft {
		block, bounded := model.layout.findSelectionBlock(mouse.X, mouse.Y)
		model.selection = screenSelection{
			fromX: mouse.X, fromY: mouse.Y, toX: mouse.X, toY: mouse.Y, dragging: true,
			block: block, bounded: bounded,
		}
		// A terminal that lost a release leaves the last drag open, and this press ends it
		// whether it began on the statement or on a bar.
		model.drag.stop()
	}

	// The bar of a view is drawn over the rows of the view, so a press reaches it before
	// them.
	if mouse.Button == tea.MouseLeft {
		if next, command, pressed := model.pressScrollbar(mouse); pressed {
			return next, command
		}
	}

	if model.confirm != nil {
		if _, action, _, pressed := findButton(
			model.layout.buttons, mouse.X, mouse.Y); pressed && mouse.Button == tea.MouseLeft {
			return model.runConfirmAction(action)
		}
		return model, nil
	}

	// Every key a renderer drew as a word is a button, on the workspace, where each one
	// names an action the workspace or the card on show returns.
	if model.screen == ScreenWorking && mouse.Button == tea.MouseLeft {
		if next, command, pressed := model.pressKey(mouse); pressed {
			return next, command
		}
	}

	switch model.screen {
	case ScreenPickingProfile:
		return model.pressPicker(mouse)
	case ScreenEditingConnection:
		return model.pressForm(mouse)
	case ScreenPromptingPassword:
		return model.pressPassword(mouse)
	case ScreenSettings:
		return model.pressSettings(mouse)
	case ScreenWorking:
		return model.pressWorkspace(mouse)
	}
	return model, nil
}

// pressScrollbar returns a press on the track of a scroll bar. A press on the thumb takes
// hold of it, and one anywhere else on the track brings it there. It reports whether the
// press landed on a bar at all.
func (model *Model) pressScrollbar(mouse tea.Mouse) (tea.Model, tea.Cmd, bool) {
	bar, found := findScrollbar(model.layout.scrollbars, mouse.X, mouse.Y)
	if !found {
		return model, nil, false
	}
	// A press lets the last drag over the cells of the frame go, as a press anywhere else
	// does.
	model.selection = screenSelection{}

	start, size, drawn := resolveThumbSpan(bar.offset, bar.rows, bar.total)
	if !drawn || bar.moveTo == nil {
		return model, nil, true
	}
	// The track is measured in half cells, so the thumb covers these rows of it.
	from, to := start/2, (start+size-1)/2
	row := mouse.Y - bar.top

	grab := row - from
	command := tea.Cmd(nil)
	if row < from || row > to {
		// The press missed the thumb, so the thumb comes to the pointer with its middle
		// under it, and the head and the foot of the track reach the ends of the view.
		grab = (to - from) / 2
		command = bar.moveTo(resolveTrackOffset(row-grab, bar.rows, bar.total))
	}
	model.drag.takeScrollbar(bar, grab)
	return model, command, true
}

// dragSplit moves the line between the editor and the result to where the pointer stands. A
// drag that takes the line past either pane stops at the fewest rows a pane is drawn in.
func (model *Model) dragSplit(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		return model, nil
	}
	// The line is the last row of the editor, so the rows down to it are the rows the
	// editor keeps.
	rows := mouse.Y - model.drag.lineGrab - model.layout.editorTop + 1
	room := model.layout.editorRows + model.layout.resultRows
	if rows > room-minPaneHeight {
		rows = room - minPaneHeight
	}
	if rows < minPaneHeight {
		rows = minPaneHeight
	}
	// A drag brings the result back, because a line that is dragged is a line between two
	// panes and not the foot of one.
	connection.ResultVisible = true
	if tab := connection.Active(); tab != nil {
		tab.PaneHeight = rows
	}
	model.drag.moved = true
	return model, nil
}

// dragTreeEdge sets the width of the object tree from where the pointer stands. The width
// the drag asks for is clamped when the frame is drawn, so a drag past either end stops at
// the narrowest tree and the narrowest pane.
func (model *Model) dragTreeEdge(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		return model, nil
	}
	// The border is the last column of the tree, so the columns up to it are its width.
	connection.SidebarWidth = max(mouse.X-model.drag.lineGrab+1, present.MinSidebarWidth)
	model.drag.moved = true
	return model, nil
}

// dragScrollbar moves the view of the bar being dragged to where the pointer stands. The
// pointer may wander off the track and the drag holds, which is what a scroll bar does.
func (model *Model) dragScrollbar(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	bar := model.drag.bar
	if bar.moveTo == nil || bar.rows < 1 {
		return model, nil
	}
	return model, bar.moveTo(resolveTrackOffset(
		mouse.Y-bar.top-model.drag.grab, bar.rows, bar.total))
}

// pressMiddleButton returns a press of the middle button: on a tab it closes it, and nowhere
// else does it do anything.
func (model *Model) pressMiddleButton(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if model.screen != ScreenWorking || connection == nil || connection.Overlay.IsOpen() {
		return model, nil
	}
	if mouse.Y != model.layout.tabRow {
		return model, nil
	}
	for _, held := range model.layout.tabs {
		if mouse.X < held.from || mouse.X > held.to {
			continue
		}
		connection.ActivateTab(held.index)
		return model.requestCloseTab(connection)
	}
	return model, nil
}

// pressKey returns a press on a key a renderer drew as a word: on the two bars, on a strip
// of a pane, or at the foot of the card on show. It reports whether the press landed on one.
func (model *Model) pressKey(mouse tea.Mouse) (tea.Model, tea.Cmd, bool) {
	connection := model.Active()
	if connection == nil {
		return model, nil, false
	}
	scope, action, held, pressed := findButton(model.layout.buttons, mouse.X, mouse.Y)
	if !pressed {
		return model, nil, false
	}
	model.frame.flashKey(held)
	// A press lets the last drag go, as a press anywhere else does.
	model.selection = screenSelection{}

	// A card on show returns the keys itself, because the keys the frame drew are its own.
	if connection.Overlay.IsOpen() {
		overlay := &connection.Overlay
		handled, held, command := model.runOverlayAction(connection, connection.Active(),
			overlay, Match{Action: action, Scope: scope})
		if !handled {
			return model, nil, true
		}
		return held, command, true
	}
	next, command := model.runAction(
		connection, connection.Active(), Match{Action: action, Scope: scope})
	return next, command, true
}

// pressForm returns a press on the connection form: a press on a mark of a picked field steps
// its value, and a press anywhere else marks the field the press landed on and leaves what was
// typed into the one before it.
func (model *Model) pressForm(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	// The file picker covers the rows of the form, so a press belongs to the picker and
	// never to what it hides.
	if model.form == nil || model.formPicker != nil {
		return model, nil
	}
	// Every key the form names is a button, and it is drawn over the rows, so it is read
	// before them.
	if scope, action, key, pressed := findButton(
		model.layout.buttons, mouse.X, mouse.Y); pressed {
		model.frame.flashKey(key)
		if held, command, ran := model.runFormAction(
			Match{Action: action, Scope: scope}); ran {
			return held, command
		}
		return model, nil
	}
	if field, step, onMark := findChoiceMark(
		model.layout.formChoices, mouse.X, mouse.Y); onMark {
		model.form.StepField(field - model.form.Cursor)
		model.form.StepChoice(step)
		return model, nil
	}
	row, found := model.layout.formRows.holds(mouse.X, mouse.Y)
	if !found {
		return model, nil
	}
	if row != model.form.Cursor {
		model.form.StepField(row - model.form.Cursor)
		return model, nil
	}
	// A press on the field it already stands on opens the picker of a file path.
	if field, focused := model.form.findFocusedField(); focused &&
		cfg.TakesFilePath(model.form.Fields, field.Key) {
		model.form.keepField()
		return model, model.openFormFilePicker(field)
	}
	return model, nil
}

// wheelRows is how many rows one turn of the wheel moves a view, and wheelColumns how many
// columns one turn of its other axis moves one.
const (
	wheelRows    = 1
	wheelColumns = 4
)

// readMouseWheel returns one turn of the wheel. It moves the rows the view under the pointer
// shows, and leaves the cursor where it stands.
func (model *Model) readMouseWheel(turned tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := turned.Mouse()
	// Shift with the wheel turns it sideways. A pointer that scrolls sideways of its own,
	// such as a trackpad, sends the two buttons below instead.
	sideways := mouse.Mod.Contains(uv.ModShift)
	switch {
	case mouse.Button == tea.MouseWheelUp && sideways:
		return model.rollWheelSideways(mouse, -1)
	case mouse.Button == tea.MouseWheelDown && sideways:
		return model.rollWheelSideways(mouse, 1)
	case mouse.Button == tea.MouseWheelUp:
		return model.rollWheel(mouse, -wheelRows)
	case mouse.Button == tea.MouseWheelDown:
		return model.rollWheel(mouse, wheelRows)
	case mouse.Button == tea.MouseWheelLeft:
		return model.rollWheelSideways(mouse, -1)
	case mouse.Button == tea.MouseWheelRight:
		return model.rollWheelSideways(mouse, 1)
	}
	return model, nil
}

// readMouseMotion follows a drag, so the cells between where it began and where it stands
// are marked as selected.
func (model *Model) readMouseMotion(moved tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	mouse := moved.Mouse()
	// A move with no button down marks what the pointer stands on, and nothing else. The
	// terminal reports one for every cell the pointer crosses, so a move that stays on the
	// same thing keeps the frame that is already on screen.
	if mouse.Button == tea.MouseNone {
		// Nothing but the pointer moved, so the frame itself still stands. What the frame
		// draws under the pointer is worked out when it is handed over, and a move that
		// marks the same thing hands over the view already on screen.
		model.frame.followPointer(mouse.X, mouse.Y)
		model.frame.held = true
		return model, nil
	}
	model.frame.followPointer(mouse.X, mouse.Y)
	// A drag belongs to whatever the press took hold of, and to no other button than the
	// one that took it.
	if model.drag.running() {
		if mouse.Button != tea.MouseLeft {
			return model, nil
		}
		switch model.drag.kind {
		case dragColumnEdge:
			return model.dragColumnEdge(mouse)
		case dragSplitLine:
			return model.dragSplit(mouse)
		case dragTreeEdge:
			return model.dragTreeEdge(mouse)
		case dragScrollbar:
			return model.dragScrollbar(mouse)
		case dragEditorText:
			return model.dragEditor(mouse)
		}
		return model, nil
	}
	if !model.selection.dragging || mouse.Button != tea.MouseLeft {
		return model, nil
	}
	reported := model.holdsSelection()
	model.selection.toX, model.selection.toY = mouse.X, mouse.Y
	// A drag of one cell selects nothing, so a plain press is not a selection.
	model.selection.held = model.selection.fromX != mouse.X || model.selection.fromY != mouse.Y
	// A drag over the cells of the frame draws nothing of its own: it is laid over the frame
	// afterwards, as the mark of the pointer is. Only the step that gives the status bar a
	// selection to report changes the frame itself.
	model.frame.held = model.holdsSelection() == reported
	return model, nil
}

// readMouseRelease keeps what the drag covered, so the copy that follows reads it.
func (model *Model) readMouseRelease(released tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	if held := model.drag; held.running() {
		model.drag.stop()
		if held.moved {
			return model, nil
		}
		connection := model.Active()
		if connection == nil {
			return model, nil
		}
		tab := connection.Active()
		if !held.holds(dragTreeEdge) && !held.holds(dragSplitLine) {
			return model, nil
		}
		// A press on a border that never moved reaches the pane it belongs to, as a press
		// inside that pane does.
		if held.pane != "" {
			if tab != nil {
				tab.Focus = held.pane
			}
			return model, nil
		}
		// A press on the foot of the editor that never moved hides the result, or brings
		// it back.
		return model.runAction(connection, tab,
			Match{Action: ActionToggleResult, Scope: cfg.ScopeGlobal})
	}
	if !model.selection.dragging {
		return model, nil
	}
	model.selection.dragging = false
	if !model.selection.held {
		model.selection = screenSelection{}
		return model, nil
	}
	// Nothing but blanks under the pointer is not a selection.
	if model.readSelectedText(model.frame.text) == "" {
		model.selection = screenSelection{}
	}
	return model, nil
}

// rollWheel moves the rows the view under the pointer shows. The cursor stays where it is,
// so it may scroll off screen.
func (model *Model) rollWheel(mouse tea.Mouse, step int) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if model.screen != ScreenWorking || connection == nil {
		return model, nil
	}
	if connection.Overlay.IsOpen() {
		return model.rollOverlay(connection, step)
	}
	tab := connection.Active()
	if _, inTree := model.layout.treeRows.holds(mouse.X, mouse.Y); inTree {
		connection.Tree.Offset, connection.Tree.Rolled = connection.Tree.Offset+step, true
		return model, nil
	}
	if mouse.Y >= model.layout.editorTop &&
		mouse.Y < model.layout.editorTop+model.layout.editorRows {
		if tab.BuildsQuery() {
			tab.Builder.Roll(step, 0)
			return model, nil
		}
		if tab.ListsCells() {
			tab.Notebook.Offset += step
			tab.Notebook.Rolled = true
			return model, nil
		}
		tab.EditorRowOffset, tab.EditorRolled = tab.EditorRowOffset+step, true
		return model, nil
	}
	if mouse.Y >= model.layout.resultTop &&
		mouse.Y < model.layout.resultTop+model.layout.resultRows {
		tab.GridRowOffset, tab.GridRolled = tab.GridRowOffset+step, true
		tab.DetailOffset += step
		// The tree scrolls its own rows, because a folded document is one row of it and
		// not one row of the result.
		tab.TreeRowOffset, tab.TreeRolled = max(tab.TreeRowOffset+step, 0), true
		return model, model.approachDrawnResultEnd(connection, tab)
	}
	return model, nil
}

// rollOverlay moves the rows the card on show holds. The conversation of the chat and the
// rows of the help both scroll without a cursor, so each one moves its own offset.
func (model *Model) rollOverlay(connection *app.Connection, step int) (tea.Model, tea.Cmd) {
	overlay := &connection.Overlay
	switch overlay.Kind {
	case app.OverlayAiChat:
		return model.rollChat(connection, step)
	case app.OverlayHelp:
		count := model.overlayRowCount(connection, *overlay)
		overlay.List.Cursor = clamp(overlay.List.Cursor+step, count)
		overlay.List.Offset = overlay.List.Cursor
		return model, nil
	case app.OverlayDiagram:
		overlay.List.Cursor = clamp(overlay.List.Cursor+step, len(overlay.Diagram.Lines))
		return model, nil
	}
	overlay.List.Offset, overlay.List.Rolled = overlay.List.Offset+step, true
	return model, nil
}

// pressPicker returns a press on the connection picker: one press marks a row, two open it,
// and a press on a key the card names runs it.
func (model *Model) pressPicker(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if scope, action, key, pressed := findButton(
		model.layout.buttons, mouse.X, mouse.Y); pressed {
		model.frame.flashKey(key)
		return model.runPickerAction(Match{Action: action, Scope: scope})
	}
	profiles := model.shownProfiles()
	row, found := model.layout.pickerRows.holds(mouse.X, mouse.Y)
	if !found || row >= len(profiles) {
		return model, nil
	}
	model.picker.focus(row, len(profiles))
	if model.clicks.count("picker-"+strconv.Itoa(row), time.Now()) < 2 {
		return model, nil
	}
	return model.chooseProfile(profiles[row])
}

// pressWorkspace returns a press on the workspace: the tab row, the tree, the panes, or the
// list of an overlay.
func (model *Model) pressWorkspace(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		return model, nil
	}
	if connection.Overlay.IsOpen() {
		return model.pressOverlay(connection, mouse)
	}
	tab := connection.Active()

	// The list over the statement is drawn last, so a press reaches it before the pane it
	// stands on.
	if row, onList := model.layout.completionRows.holds(mouse.X, mouse.Y); onList {
		if row < len(tab.Completion.Candidates) {
			tab.Completion.Selected = row
			model.acceptCompletion(connection, tab)
		}
		return model, nil
	}

	if mouse.Y == model.layout.tabRow {
		return model.pressTabRow(connection, mouse)
	}
	if model.pressPaneBorder(mouse) {
		return model, nil
	}
	if row, found := model.layout.connections.holds(mouse.X, mouse.Y); found {
		if row >= model.connections.count() {
			return model, nil
		}
		model.connections.focus(row)
		if mouse.Button == tea.MouseRight {
			return model.openConnectionMenu(model.connections.at(row))
		}
		if mouse.X >= model.layout.closeConnectionFrom &&
			mouse.X <= model.layout.closeConnectionTo {
			return model.requestCloseConnection(model.connections.at(row))
		}
		return model, nil
	}
	if row, found := model.layout.treeRows.holds(mouse.X, mouse.Y); found {
		return model.pressTreeRow(connection, tab, mouse, row)
	}
	if row, found := model.layout.cellRows.holds(mouse.X, mouse.Y); found {
		return model.pressCellRow(connection, tab, mouse, row)
	}
	if row, found := model.layout.builderRows.holds(mouse.X, mouse.Y); found {
		return model.pressBuilderRow(connection, tab, mouse, row)
	}
	if mouse.X <= model.layout.treeTo && mouse.X >= model.layout.treeFrom {
		tab.Focus = app.PaneSidebar
		return model, nil
	}
	if mouse.Y >= model.layout.editorTop &&
		mouse.Y < model.layout.editorTop+model.layout.editorRows {
		if mouse.Y == model.layout.editorTop+model.layout.editorRows-1 {
			return model, nil
		}
		return model.pressEditor(connection, tab, mouse)
	}
	if mouse.Y >= model.layout.resultTop &&
		mouse.Y < model.layout.resultTop+model.layout.resultRows {
		return model.pressResultPane(connection, tab, mouse)
	}
	return model, nil
}

// pressPaneBorder starts a divider drag on a left-button press, and returns false off a
// divider. A press without motion acts on the release.
func (model *Model) pressPaneBorder(mouse tea.Mouse) bool {
	if mouse.Button != tea.MouseLeft {
		return false
	}
	if grab, pane, onLine := model.findSplitLine(mouse.X, mouse.Y); onLine {
		model.selection = screenSelection{}
		model.drag.takeSplitLine(grab, pane)
		return true
	}
	if grab, pane, onEdge := model.findTreeEdge(mouse.X, mouse.Y); onEdge {
		model.selection = screenSelection{}
		model.drag.takeTreeEdge(grab, pane)
		return true
	}
	return false
}

// findSplitLine returns the grab offset and the focus target of the editor-result divider
// under the pointer. The divider is two rows: the editor foot and the result head.
func (model *Model) findSplitLine(x, y int) (int, app.Pane, bool) {
	layout := model.layout
	if layout.editorRows < 1 || x < model.editorLeft {
		return 0, "", false
	}
	if y == layout.editorTop+layout.editorRows-1 {
		return 0, "", true
	}
	if layout.resultRows > 0 && y == layout.resultTop {
		return 1, app.PaneResult, true
	}
	return 0, "", false
}

// findTreeEdge returns the grab offset and the focus target of the tree divider under the
// pointer. The divider is two columns: the tree border and the pane border.
func (model *Model) findTreeEdge(x, y int) (int, app.Pane, bool) {
	layout := model.layout
	if layout.treeTo < layout.treeFrom ||
		y < firstPaneRow || y >= firstPaneRow+layout.editorRows+layout.resultRows {
		return 0, "", false
	}
	switch {
	case x == layout.treeTo:
		return 0, app.PaneSidebar, true
	case x == layout.treeTo+1 && y < firstPaneRow+layout.editorRows:
		return 1, app.PaneEditor, true
	case x == layout.treeTo+1:
		return 1, app.PaneResult, true
	}
	return 0, "", false
}

// pressTabRow returns a press on the row of tabs: the close mark closes a tab, the marks at
// the ends step to a tab that is off screen, and the rest of a tab opens it.
func (model *Model) pressTabRow(
	connection *app.Connection, mouse tea.Mouse,
) (tea.Model, tea.Cmd) {
	if model.layout.scrollTabsBack.holds(mouse.X, mouse.Y, model.layout.tabRow) {
		connection.StepTab(-1)
		return model, nil
	}
	if model.layout.scrollTabsOn.holds(mouse.X, mouse.Y, model.layout.tabRow) {
		connection.StepTab(1)
		return model, nil
	}
	for _, held := range model.layout.tabs {
		if mouse.X < held.from || mouse.X > held.to {
			continue
		}
		connection.ActivateTab(held.index)
		if mouse.Button == tea.MouseRight {
			return model.openTabMenu(connection)
		}
		if held.closeTo >= held.closeFrom &&
			mouse.X >= held.closeFrom && mouse.X <= held.closeTo {
			return model.requestCloseTab(connection)
		}
		return model, nil
	}
	return model, nil
}

// The indent of one level of the tree, and the fold mark with the space after it.
const (
	treeIndentPerLevel = 2
	treeMarkerWidth    = 2
	// treeRowPaddingLeft is the blank column the row opens with.
	treeRowPaddingLeft = 1
)

// pressTreeRow returns a press on one row of the object tree: one press marks it, two open it,
// a press on the fold mark folds it, and the right button opens its menu.
func (model *Model) pressTreeRow(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse, row int,
) (tea.Model, tea.Cmd) {
	rows := model.treeRows(connection)
	if row >= len(rows) {
		return model, nil
	}
	tab.Focus = app.PaneSidebar
	connection.Tree.Cursor = row
	held := rows[row]

	if mouse.Button == tea.MouseRight {
		return model.runTreeAction(connection, Match{
			Action: ActionObjectMenu, Scope: cfg.ScopeTree,
		})
	}
	// Only the mark folds the row. A press on the name marks it, so reading the size of a
	// relation does not open its columns.
	// The border of the pane takes the column before the row, and the row opens with a blank.
	offset := mouse.X - model.layout.treeFrom - 1 - treeRowPaddingLeft
	if present.IsOnFoldMarker(offset, held.Depth, treeIndentPerLevel, treeMarkerWidth) {
		if held.Expandable {
			return model.toggleFold(connection, held)
		}
		return model, nil
	}
	if model.clicks.count("tree-"+held.ID, time.Now()) < 2 {
		return model, nil
	}
	return model.openTreeNode(connection, held)
}

// pressResultPane returns a press on the result: it moves the cursor to the cell, and a
// second press on the same cell opens the row. The right button opens the menu of the cell.
func (model *Model) pressResultPane(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse,
) (tea.Model, tea.Cmd) {
	tab.Focus = app.PaneResult
	// A statement belongs to the run above the views of it, so the strips are read first.
	if at, found := findChip(model.layout.statementChips, mouse.X, mouse.Y); found {
		tab.Results.SelectResult(at)
		return model.showSelectedResult(connection, tab)
	}
	if at, found := findChip(model.layout.viewChips, mouse.X, mouse.Y); found {
		return model.selectViewAt(connection, tab, at)
	}
	if tab.View == app.ViewData && mouse.Y == model.layout.gridHeaderRow {
		return model.pressColumnHeader(connection, tab, mouse)
	}
	if tab.ActiveView(connection.Session) == app.ViewTree {
		return model.pressDocumentTree(connection, tab, mouse)
	}
	row, found := model.layout.gridRows.holds(mouse.X, mouse.Y)
	if !found {
		return model, nil
	}
	if row >= len(model.buildGridShape(connection, tab).Text) {
		return model, nil
	}
	tab.GridRow = row
	if column, over := findColumnUnder(model.layout.gridColumns, mouse.X); over {
		tab.GridColumn, tab.GridColumnRolled = column, false
	}
	if mouse.Button == tea.MouseRight {
		return model.runGridAction(connection, tab,
			Match{Action: ActionOpenMenu, Scope: cfg.ScopeGrid})
	}
	if model.clicks.count("grid-"+strconv.Itoa(row), time.Now()) < 2 {
		return model, nil
	}
	return model.runGridAction(connection, tab,
		Match{Action: ActionOpenRow, Scope: cfg.ScopeGrid})
}

// answerOverlayChip returns the question of a card with the chip the press landed on. The
// first chip is the yes and the second the no.
func (model *Model) answerOverlayChip(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay, at int,
) (tea.Model, tea.Cmd) {
	action := ActionAnswerYes
	if at != 0 {
		action = ActionAnswerNo
	}
	if overlay.Kind == app.OverlayAiChat {
		_, held, command := model.runChatAction(connection, tab,
			Match{Action: action, Scope: cfg.ScopeDialog})
		return held, command
	}
	if action == ActionAnswerYes {
		return model.chooseOverlayRow(connection, tab, overlay, chooseInSameTab)
	}
	answer := overlay.Answers.Answer
	connection.CloseEveryOverlay()
	return model, model.runAnswer(answer, false)
}

// pressOverlayFormRow marks the row of a form or of a list of returns, and takes the answer
// where the row is one.
func (model *Model) pressOverlayFormRow(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay, row int,
) (tea.Model, tea.Cmd) {
	switch overlay.Kind {
	case app.OverlayChoice:
		if row >= len(overlay.Choices) {
			return model, nil
		}
		answer, chosen := overlay.Answers.ID, overlay.Choices[row].ID
		connection.CloseEveryOverlay()
		return model, model.runIDAnswer(answer, chosen)
	case app.OverlayExport, app.OverlayDump:
		overlay.Field = row
	case app.OverlayBuilderField:
		overlay.Field = clamp(row, builderFieldRows)
	case app.OverlayBuilderJoin:
		overlay.List.Cursor = clamp(row, builderJoinRows)
	}
	return model, nil
}

// pressExportChoice steps a field of the export card or of the dump card that runs through a
// list of values, and reports whether the press landed on one of its marks.
func (model *Model) pressExportChoice(
	connection *app.Connection, overlay *app.Overlay, mouse tea.Mouse,
) bool {
	field, step, onMark := findChoiceMark(model.layout.formChoices, mouse.X, mouse.Y)
	if !onMark {
		return false
	}
	switch overlay.Kind {
	case app.OverlayExport:
		overlay.Field = field
		StepExportChoice(overlay, step)
	case app.OverlayDump:
		overlay.Field = field
		StepDumpChoice(overlay, step)
	case app.OverlayBuilderField:
		tab := connection.Active()
		if !tab.BuildsQuery() {
			return false
		}
		overlay.Field = field
		stepBuilderField(tab.Builder, field, step)
	case app.OverlayBuilderJoin:
		tab := connection.Active()
		if !tab.BuildsQuery() {
			return false
		}
		overlay.List.Cursor = builderJoinKindRow
		stepJoinKind(tab.Builder, overlay.Field, step)
	default:
		return false
	}
	return true
}

// pressColumnHeader returns a press on the row of names above the grid: it moves the column
// cursor to the name and orders the read by it. With Shift the column is added to the order
// the read already has.
func (model *Model) pressColumnHeader(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse,
) (tea.Model, tea.Cmd) {
	// The border after a column is drawn over the name beside it, so it is read first.
	if mouse.Button == tea.MouseLeft {
		if next, held := model.pressColumnEdge(tab, mouse); held {
			return next, nil
		}
	}
	column, over := findColumnUnder(model.layout.gridColumns, mouse.X)
	if !over {
		return model, nil
	}
	tab.GridColumn, tab.GridColumnRolled = column, false
	// The right button opens what a reader does to the column itself, which is more than
	// the order of the read.
	if mouse.Button == tea.MouseRight {
		return model.openColumnMenu(connection, tab, model.buildGridShape(connection, tab))
	}
	action := ActionSortColumn
	if mouse.Mod.Contains(tea.ModShift) {
		action = ActionAddSortColumn
	}
	return model.runGridAction(connection, tab, Match{Action: action, Scope: cfg.ScopeGrid})
}

// pressColumnEdge returns a press on the border after a column: it takes hold of the border
// so a drag sets the width, and a second press gives the column back to its widest value. It
// reports whether the press landed on a border at all.
func (model *Model) pressColumnEdge(tab *app.Tab, mouse tea.Mouse) (tea.Model, bool) {
	edge, found := findColumnEdge(model.layout.columnEdges, mouse.X, mouse.Y,
		model.layout.gridHeaderRow)
	if !found {
		return model, false
	}
	// A drag over a border belongs to the border, so the cells of the frame keep no
	// selection of their own.
	model.selection = screenSelection{}

	// A second press gives the column back to the width of its widest value.
	if model.clicks.count("edge-"+strconv.Itoa(edge.index), time.Now()) >= 2 {
		delete(tab.ColumnWidths, edge.index)
		return model, true
	}
	shape := model.buildGridShape(model.Active(), tab)
	if edge.index >= len(shape.Widths) {
		return model, true
	}
	model.drag.takeColumnEdge(edge.index, shape.Widths[edge.index], mouse.X)
	return model, true
}

// findColumnEdge returns the border the pointer stands on, and whether it stands on one.
func findColumnEdge(edges []columnHit, x, y, row int) (columnHit, bool) {
	for _, held := range edges {
		if held.holds(x, y, row) {
			return held, true
		}
	}
	return columnHit{}, false
}

// dragColumnEdge sets how wide the column is from where the pointer stands.
func (model *Model) dragColumnEdge(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		return model, nil
	}
	tab := connection.Active()
	if tab.ColumnWidths == nil {
		tab.ColumnWidths = map[int]int{}
	}
	tab.ColumnWidths[model.drag.column] = clampColumnWidth(
		model.drag.columnWidth + mouse.X - model.drag.columnFrom)
	return model, nil
}

// pressOverlay returns a press on the list of an overlay: one press marks a row, and the
// same press chooses it, as a menu does.
func (model *Model) pressOverlay(
	connection *app.Connection, mouse tea.Mouse,
) (tea.Model, tea.Cmd) {
	overlay := &connection.Overlay
	tab := connection.Active()

	// A card that asks a question draws its answers as chips, and a press on one returns
	// it, so the yes never needs the keyboard.
	if at, onChip := findChip(model.layout.overlayChips, mouse.X, mouse.Y); onChip {
		return model.answerOverlayChip(connection, tab, overlay, at)
	}
	// A field that steps through a list of values draws a mark on each side, and a press
	// on one steps it.
	if model.pressExportChoice(connection, overlay, mouse) {
		return model, nil
	}
	// A card with a form or a list of returns marks the row the press landed on.
	if row, onRow := model.layout.formRows.holds(mouse.X, mouse.Y); onRow {
		return model.pressOverlayFormRow(connection, tab, overlay, row)
	}

	row, found := model.layout.overlayRows.holds(mouse.X, mouse.Y)
	if !found {
		return model, nil
	}
	count := model.overlayRowCount(connection, *overlay)
	if row >= count || model.isMenuDivider(*overlay, row) {
		return model, nil
	}
	overlay.List.Cursor = row
	if mouse.Button == tea.MouseRight {
		if handled, held, command := model.runOverlayAction(connection, tab, overlay,
			Match{Action: ActionListSecondary, Scope: cfg.ScopeDialog}); handled {
			return held, command
		}
		return model, nil
	}
	return model.chooseOverlayRow(connection, tab, overlay, chooseInSameTab)
}

// pressEditor returns a press on the statement: one press puts the caret where the pointer
// stands, two take the word under it, and three take the whole line.
func (model *Model) pressEditor(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse,
) (tea.Model, tea.Cmd) {
	tab.Focus = app.PaneEditor
	if mouse.Button == tea.MouseRight {
		return model.openEditorMenu(connection, tab)
	}
	if !tab.EditorVisible() || mouse.Button != tea.MouseLeft {
		return model, nil
	}
	offset, onText := model.resolveEditorOffset(tab, mouse.X, mouse.Y)
	if !onText {
		return model, nil
	}
	// A drag over the statement belongs to the buffer, so the cells of the frame keep no
	// selection of their own and only one of the two is ever drawn.
	model.selection = screenSelection{}
	model.drag.takeEditorText()

	switch model.clicks.count("editor-"+strconv.Itoa(offset), time.Now()) {
	case 1:
		tab.Editor.PlaceCaret(offset, mouse.Mod.Contains(uv.ModShift))
	case 2:
		tab.Editor.SelectWordAt(offset)
	default:
		tab.Editor.SelectLineAt(offset)
	}
	tab.EditorRolled = false
	return model, nil
}

// dragEditor grows the selection of the statement to where the pointer stands.
func (model *Model) dragEditor(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		return model, nil
	}
	tab := connection.Active()
	offset, onText := model.resolveEditorOffset(tab, mouse.X, mouse.Y)
	if !onText {
		return model, nil
	}
	tab.Editor.PlaceCaret(offset, true)
	return model, nil
}

// resolveEditorOffset returns the offset in the buffer the pointer stands on, and whether it
// stands on the statement at all. A press on the gutter returns the start of that line.
func (model *Model) resolveEditorOffset(tab *app.Tab, x, y int) (int, bool) {
	layout := model.layout
	if layout.editorTextRows < 1 || layout.editorTextWidth < 1 {
		return 0, false
	}
	if y < layout.editorTextTop || y >= layout.editorTextTop+layout.editorTextRows {
		return 0, false
	}
	if x > layout.editorTextLeft+layout.editorTextWidth {
		x = layout.editorTextLeft + layout.editorTextWidth
	}
	cell := layout.editorColumnOffset + (x - layout.editorTextLeft)
	if x < layout.editorTextLeft {
		cell = 0
	}
	line := layout.editorFirstLine + (y - layout.editorTextTop)
	return tab.Editor.FindOffsetAt(line, cell), true
}

// rollWheelSideways returns one turn of the other axis of the wheel, one step to the left or
// to the right. The diagram of a builder scrolls along its boxes, the statement along the
// cells of its lines, and the grid along its columns. The cursor stays where it is, so it may
// scroll off screen.
func (model *Model) rollWheelSideways(mouse tea.Mouse, step int) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if model.screen != ScreenWorking || connection == nil || connection.Overlay.IsOpen() {
		return model, nil
	}
	tab := connection.Active()
	if tab == nil {
		return model, nil
	}
	if mouse.Y >= model.layout.editorTop &&
		mouse.Y < model.layout.editorTop+model.layout.editorRows {
		if tab.BuildsQuery() {
			tab.Builder.Roll(0, step*wheelColumns)
			return model, nil
		}
		if tab.ListsCells() {
			return model, nil
		}
		tab.EditorColumnOffset = max(tab.EditorColumnOffset+step*wheelColumns, 0)
		tab.EditorRolled = true
		return model, nil
	}
	if mouse.Y >= model.layout.resultTop &&
		mouse.Y < model.layout.resultTop+model.layout.resultRows {
		tab.GridColumnOffset = max(tab.GridColumnOffset+step, 0)
		tab.GridColumnRolled = true
		return model, nil
	}
	return model, nil
}
