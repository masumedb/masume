package ui

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// hoverKind says what the pointer stands on, and how the frame marks it.
type hoverKind string

const (
	// hoverNothing is the pointer on a cell that returns no press.
	hoverNothing hoverKind = ""
	// hoverRow marks a whole row a press would take: a tab, a row of a list, a row of the
	// grid, a chip of a strip.
	hoverRow hoverKind = "row"
	// hoverKey marks a key or a button a press would run.
	hoverKey hoverKind = "key"
)

// hoverTarget is what the pointer stands on: the cells it covers, and how they are marked.
// The whole of it is compared, so a move that stays on the same thing costs no frame.
type hoverTarget struct {
	kind     hoverKind
	row      int
	from, to int
	// glyph is drawn in the cell at glyphAt, such as the close mark of a connection row.
	glyph   string
	glyphAt int
}

// insetBy pulls the mark in from each end, so a row that spans a whole pane does not mark the
// border of the pane itself.
func (target hoverTarget) insetBy(cells int) hoverTarget {
	if !target.isSomething() {
		return target
	}
	target.from += cells
	target.to -= cells
	return target
}

// isSomething is true where the pointer stands on a thing a press would take.
func (target hoverTarget) isSomething() bool {
	return target.kind != hoverNothing && target.to >= target.from
}

// Tint weights toward the text colour: hoverTint under the pointer, pressedTint on a key under
// the held left button.
const (
	hoverTint   = 0.14
	pressedTint = 0.3
)

// raisedContrast is the contrast an ink is raised to under a tint, a step over the floor.
const raisedContrast = TextContrastFloor + 0.1

// paintHover marks what the pointer stands on with a tint of each cell's own ground.
func (model *Model) paintHover(frame string) string {
	target := model.frame.hover
	if !target.isSomething() {
		return frame
	}
	return model.tintCells(frame, target.row, target.from, target.to, hoverTint,
		target.glyph, target.glyphAt)
}

// tintCells mixes the ground of a run of cells of one row toward the text colour. An ink that
// the new ground leaves below the text contrast floor is raised to it. A glyph, where one is
// given, is drawn in the cell at glyphAt.
func (model *Model) tintCells(
	frame string, row, from, to int, weight float64, glyph string, glyphAt int,
) string {
	rows := strings.Split(frame, "\n")
	if row < 0 || row >= len(rows) || to < from {
		return frame
	}
	cells := mapCells(rows[row])
	model.tintRowCells(cells, from, to, model.styles.Theme.Text, weight)
	if glyph != "" && glyphAt >= 0 && glyphAt < len(cells) {
		cells[glyphAt].text = glyph
	}
	rows[row] = writeCells(cells)
	return strings.Join(rows, "\n")
}

// tintRowCells mixes the ground of a run of cells toward a colour. An ink that the new ground
// leaves below the text contrast floor is raised to it.
func (model *Model) tintRowCells(
	cells []styledCell, from, to int, toward color.Color, weight float64,
) {
	theme := model.styles.Theme
	for at := max(from, 0); at <= to && at < len(cells); at++ {
		ink, ground := readCellColors(cells[at].sgr)
		if ground == nil {
			ground = theme.Panel
		}
		ground = MixColors(ground, toward, weight)
		cells[at].sgr += writeColorSequence("48", ground)
		if ink != nil && CalculateContrastRatio(ink, ground) < TextContrastFloor {
			cells[at].sgr += writeColorSequence("38", ResolveColorAtContrast(
				PickInkFor(ground, theme.Background, theme.Text, raisedContrast), ink, ground,
				raisedContrast))
		}
	}
}

// readCellColors returns the last true colour ink and ground the escapes of a cell set. A
// colour the escapes do not set is nil.
func readCellColors(sgr string) (color.Color, color.Color) {
	var ink, ground color.Color
	for sequence := range strings.SplitSeq(sgr, "\x1b[") {
		sequence = strings.TrimSuffix(sequence, "m")
		if sequence == "" {
			continue
		}
		fields := strings.Split(sequence, ";")
		for at := 0; at < len(fields); at++ {
			switch fields[at] {
			case "", "0":
				ink, ground = nil, nil
			case "39":
				ink = nil
			case "49":
				ground = nil
			case "38":
				ink = parseTrueColor(fields[at+1:])
				at += skipColorFields(fields[at+1:])
			case "48":
				ground = parseTrueColor(fields[at+1:])
				at += skipColorFields(fields[at+1:])
			case "58":
				at += skipColorFields(fields[at+1:])
			}
		}
	}
	return ink, ground
}

// parseTrueColor returns the colour of the fields "2;r;g;b" after a colour code, and nil for
// any other form.
func parseTrueColor(fields []string) color.Color {
	if len(fields) < 4 || fields[0] != "2" {
		return nil
	}
	channels := [3]uint8{}
	for at := range channels {
		value, err := strconv.Atoi(fields[at+1])
		if err != nil || value < 0 || value > 255 {
			return nil
		}
		channels[at] = uint8(value)
	}
	return color.RGBA{R: channels[0], G: channels[1], B: channels[2], A: 0xff}
}

// skipColorFields returns how many fields after a colour code belong to that colour.
func skipColorFields(fields []string) int {
	if len(fields) == 0 {
		return 0
	}
	switch fields[0] {
	case "2":
		return min(4, len(fields))
	case "5":
		return min(2, len(fields))
	}
	return 0
}

// writeColorSequence returns the escape that sets this true colour: code 38 for the ink, 48
// for the ground.
func writeColorSequence(code string, held color.Color) string {
	red, green, blue := readChannels(held)
	return "\x1b[" + code + ";2;" + strconv.Itoa(red) + ";" + strconv.Itoa(green) + ";" +
		strconv.Itoa(blue) + "m"
}

// resolveHover returns what the pointer stands on. The parts of the frame are read in the
// order a press reads them, so a card wins the cells of the pane under it.
func (model *Model) resolveHover(x, y int) hoverTarget {
	if x < 0 || y < 0 || x >= model.width || y >= model.height {
		return hoverTarget{}
	}
	// A drag belongs to what it began on, so nothing is marked while one runs. The mark
	// would follow the pointer over the cells the drag covers and read as a second thing
	// happening at once.
	if model.isDragging() || model.frame.isArmed {
		return hoverTarget{}
	}
	// Every key a renderer drew as a word is drawn over the rows, so it is read first.
	for _, held := range model.layout.buttons {
		if y == held.row && x >= held.from && x <= held.to {
			return hoverTarget{kind: hoverKey, row: held.row, from: held.from, to: held.to}
		}
	}
	switch model.screen {
	case ScreenPickingProfile:
		return resolveRowHover(model.layout.pickerRows, len(model.shownPickerRows()),
			model.picker.cursor, x, y)
	case ScreenEditingConnection:
		return resolveRowHover(
			model.layout.formRows, model.countFormFields(), noFilledRow, x, y)
	case ScreenSettings:
		return model.resolveSettingsHover(x, y)
	case ScreenWorking:
		return model.resolveWorkspaceHover(x, y)
	}
	return hoverTarget{}
}

// countFormFields returns how many rows the connection form shows.
func (model *Model) countFormFields() int {
	if model.form == nil {
		return 0
	}
	return len(model.form.Shown())
}

// resolveSettingsHover returns the row of the settings screen the pointer stands on.
func (model *Model) resolveSettingsHover(x, y int) hoverTarget {
	held := model.settingsForm
	if held == nil {
		return hoverTarget{}
	}
	filled := noFilledRow
	if held.Pane == paneSections {
		filled = held.Section
	}
	if found := resolveRowHover(model.layout.settingsSections,
		len(held.Sections), filled, x, y); found.kind != hoverNothing {
		return found
	}
	filled = noFilledRow
	if held.Pane == paneItems {
		filled = held.Item
	}
	return resolveRowHover(model.layout.settingsRows, len(held.Items), filled, x, y)
}

// resolveRowHover returns the row of a block the pointer stands on, and nothing where the row
// under it holds no item.
//
// The row named by filled is the one already drawn on a ground of its own, such as the row
// under the cursor. It takes no mark: its ink was chosen against that ground, and laying
// another one under it would leave the ink unreadable. A block whose rows are never filled
// names none of them, with a row of minus one.
func resolveRowHover(block rowsHit, items, filled, x, y int) hoverTarget {
	row, found := block.holds(x, y)
	if !found || row-block.offset >= items || row == filled {
		return hoverTarget{}
	}
	return hoverTarget{kind: hoverRow, row: y, from: block.from, to: block.to}
}

// noFilledRow says that no row of a block is drawn on a ground of its own, so every row it
// holds takes the mark of the pointer.
const noFilledRow = -1

// resolveWorkspaceHover returns what the pointer stands on over the workspace.
func (model *Model) resolveWorkspaceHover(x, y int) hoverTarget {
	connection := model.Active()
	if connection == nil {
		return hoverTarget{}
	}
	layout := model.layout
	overlay := connection.Overlay
	if overlay.IsOpen() {
		// The two answers of a question are both drawn on a ground of their own.
		if _, onChip := findChip(layout.overlayChips, x, y); onChip {
			return hoverTarget{}
		}
		if target := resolveRowHover(layout.formRows, layout.formRows.count,
			noFilledRow, x, y); target.isSomething() {
			return target
		}
		if row, found := layout.overlayRows.holds(x, y); found &&
			model.isMenuDivider(overlay, row) {
			return hoverTarget{}
		}
		return resolveRowHover(layout.overlayRows,
			model.overlayRowCount(connection, overlay), overlay.List.Cursor, x, y)
	}

	tab := connection.Active()
	if target := resolveRowHover(layout.completionRows, len(tab.Completion.Candidates),
		tab.Completion.Selected, x, y); target.isSomething() {
		return target
	}
	// A block of the workspace draws one row per item, so the rows it recorded are the
	// items it holds and the count of them returns for both.
	if y == layout.tabRow {
		return resolveTabHover(layout.tabs, connection.ActiveIndex, x, y)
	}
	if _, onDivider := layout.divider.holds(x, y); onDivider || model.isOnAside(x, y) {
		return hoverTarget{}
	}
	if _, _, onLine := model.findSplitLine(x, y); onLine {
		return hoverTarget{}
	}
	if _, _, onEdge := model.findTreeEdge(x, y); onEdge {
		return hoverTarget{}
	}
	// The rows of the tree pane reach both of its borders, so the mark is pulled in from
	// each end and the border keeps its shape.
	if target := resolveRowHover(layout.connections, model.connections.count(),
		noFilledRow, x, y); target.isSomething() {
		target = target.insetBy(1)
		if glyph := model.icons.Icon(cfg.IconClose); present.MeasureText(glyph) == 1 {
			target.glyph, target.glyphAt = glyph, layout.closeConnectionTo
		}
		return target
	}
	if target := resolveRowHover(layout.treeRows, layout.treeRows.count,
		filledRowOf(connection.Tree.Cursor, tab.Focus == app.PaneSidebar),
		x, y); target.isSomething() {
		return target.insetBy(1)
	}
	if target := resolveChipHover(layout.statementChips, tab.Results.ActiveIndex(),
		x, y); target.isSomething() {
		return target
	}
	if target := resolveChipHover(layout.viewChips,
		findViewIndex(tab.Views(connection.Session), tab.ActiveView(connection.Session)),
		x, y); target.isSomething() {
		return target
	}
	if y == layout.gridHeaderRow && tab.View == app.ViewData {
		return resolveColumnHover(layout.gridColumns,
			filledRowOf(tab.GridColumn, tab.Focus == app.PaneResult), x, y)
	}
	return resolveRowHover(layout.gridRows, layout.gridRows.count,
		filledRowOf(tab.GridRow, tab.Focus == app.PaneResult), x, y)
}

// buildSpanHover returns the mark for a run of cells, and nothing where the item is the one
// already drawn on a ground of its own.
func buildSpanHover(index, filled, row, from, to int) hoverTarget {
	if index == filled {
		return hoverTarget{}
	}
	return hoverTarget{kind: hoverRow, row: row, from: from, to: to}
}

// resolveChipHover returns the chip of a strip the pointer stands on.
func resolveChipHover(chips []chipHit, filled, x, y int) hoverTarget {
	for _, held := range chips {
		if y == held.row && x >= held.from && x <= held.to {
			return buildSpanHover(held.index, filled, y, held.from, held.to)
		}
	}
	return hoverTarget{}
}

// resolveTabHover returns the tab the pointer stands on, on the row the tabs are drawn.
func resolveTabHover(tabs []tabHit, filled, x, y int) hoverTarget {
	for _, held := range tabs {
		if x >= held.from && x <= held.to {
			return buildSpanHover(held.index, filled, y, held.from, held.to)
		}
	}
	return hoverTarget{}
}

// resolveColumnHover returns the column the pointer stands on, on a row it is known to be on.
func resolveColumnHover(columns []columnHit, filled, x, y int) hoverTarget {
	for _, held := range columns {
		if x >= held.from && x <= held.to {
			return buildSpanHover(held.index, filled, y, held.from, held.to)
		}
	}
	return hoverTarget{}
}

// filledRowOf returns the row drawn on a ground of its own, and none where the pane it belongs
// to does not hold the keyboard: a cursor of a pane that is not focused is drawn quietly, so
// the mark of the pointer reads against it.
func filledRowOf(cursor int, focused bool) int {
	if !focused {
		return noFilledRow
	}
	return cursor
}

// findViewIndex returns which of the views offered is this one.
func findViewIndex(offered []app.ResultView, drawn app.ResultView) int {
	for at, view := range offered {
		if view == drawn {
			return at
		}
	}
	return noFilledRow
}

// isDragging is true while a press is being dragged, whichever thing it took hold of.
func (model *Model) isDragging() bool {
	return model.drag.running() || model.selection.dragging
}

// keyFlashWait is how long a key stays lit after a press. It is one frame of the wheel, which
// is long enough to be seen and short enough that it never looks like a state of its own.
const keyFlashWait = 120 * time.Millisecond

// paintPressedKey draws the pressed look on the key the left button is down on, and for a
// moment on the key a click ran.
func (model *Model) paintPressedKey(frame string) string {
	held := model.frame.armed
	if !model.frame.isHoldingKey() {
		if !model.frame.isFlashing() {
			return frame
		}
		held = model.frame.pressed
	}
	// A key that ran something may have taken its own row off the frame, and the cells it
	// covered belong to whatever was drawn there instead.
	if !model.holdsKeyStill(held) {
		return frame
	}
	return model.tintCells(frame, held.row, held.from, held.to, pressedTint, "", 0)
}

// holdsKeyStill is true where the frame still draws this key in the cells it was pressed in.
func (model *Model) holdsKeyStill(held buttonHit) bool {
	for _, drawn := range model.layout.buttons {
		if drawn.row == held.row && drawn.from == held.from && drawn.to == held.to &&
			drawn.action == held.action {
			return true
		}
	}
	return false
}
