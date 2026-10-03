package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/query"
)

// movePointer sends one move with no button down, as the terminal reports one for every cell
// the pointer crosses.
func movePointer(model *Model, x, y int) *Model {
	held, _ := model.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseNone})
	next := held.(*Model)
	next.View()
	return next
}

// The row the pointer stands on takes the ground of the header, so a reader sees what a press
// would take before they press it.
func TestThePointerMarksTheRowItStandsOn(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 20, 4)
	connection := model.Active()
	connection.Tree.Expanded[core.BuildSchemaID("schema_00")] = true
	model.View()

	block := model.layout.treeRows
	row := block.top + 2
	before := strings.Split(model.frame.shown, "\n")[row]
	model = movePointer(model, block.from+6, row)
	after := strings.Split(model.frame.shown, "\n")[row]

	if before == after {
		t.Error("the row the pointer stands on was drawn as it was")
	}
	if stripStyles(before) != stripStyles(after) {
		t.Errorf("the mark changed the text of the row: %q became %q",
			stripStyles(before), stripStyles(after))
	}
	if model.frame.hover.kind != hoverRow {
		t.Errorf("the pointer stands on %q", model.frame.hover.kind)
	}
}

// The terminal reports a move for every cell the pointer crosses, so a move that stays on the
// same thing must hand the terminal the view it already has.
func TestAMoveThatChangesNothingHandsBackTheSameView(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 20, 4)
	model.View()
	block := model.layout.treeRows

	model = movePointer(model, block.from+4, block.top+1)
	onRow := model.frame.shown
	model = movePointer(model, block.from+6, block.top+1)
	if model.frame.shown != onRow {
		t.Error("a move along the same row drew the view again")
	}
	model = movePointer(model, block.from+6, block.top+2)
	if model.frame.shown == onRow {
		t.Error("a move onto another row left the view as it was")
	}
}

// Nothing but the pointer moves while it is followed, so the frame under the mark is drawn
// once and the mark alone is laid over it again.
func TestAMoveOfThePointerDrawsNoNewFrame(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 20, 4)
	model.View()
	block := model.layout.treeRows

	model = movePointer(model, block.from+4, block.top+1)
	frame := model.frame.text
	for at := 2; at < block.count && at < 6; at++ {
		model = movePointer(model, block.from+4, block.top+at)
		if model.frame.text != frame {
			t.Fatalf("the frame was drawn again for a move onto row %d", at)
		}
	}
}

// A key the pointer stands on takes a tint of its own ground.
func TestThePointerMarksTheKeyItStandsOn(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select id from orders", 0)
	model.View()

	held := findButtonOfAction(model, ActionShowPalette)
	if held.row < 0 {
		t.Fatal("the title bar recorded no key that opens the palette")
	}
	before := mapCells(strings.Split(model.frame.shown, "\n")[held.row])[held.from]
	model = movePointer(model, held.from, held.row)
	if model.frame.hover.kind != hoverKey {
		t.Fatalf("the pointer stands on %q", model.frame.hover.kind)
	}
	after := mapCells(strings.Split(model.frame.shown, "\n")[held.row])[held.from]
	_, beforeGround := readCellColors(before.sgr)
	_, afterGround := readCellColors(after.sgr)
	if afterGround == nil || WriteHex(afterGround) == WriteHex(beforeGround) {
		t.Errorf("the key under the pointer kept the ground %v", afterGround)
	}
}

// clickMouse sends a press and a release of the left button on one cell.
func clickMouse(model *Model, x, y int) (tea.Model, tea.Cmd) {
	model.readMouse(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return model.readMouseRelease(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// A click lights the key it ran for a moment.
func TestAClickLightsTheKeyItRan(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select id from orders", 0)
	model.View()

	held := findButtonOfAction(model, ActionShowPalette)
	clickMouse(model, held.from, held.row)
	if !model.frame.isFlashing() {
		t.Fatal("a click left the key unlit")
	}
	if model.frame.pressed.action != ActionShowPalette {
		t.Errorf("the click lit the key of %q", model.frame.pressed.action)
	}
}

// A key runs on the release of the button over it. While the button is down the key is drawn
// pressed, and a release off the key runs nothing.
func TestAKeyRunsOnTheReleaseOverIt(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	model.View()

	held := findButtonOfAction(model, ActionShowPalette)
	model.Update(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	model.View()
	if connection.Overlay.IsOpen() {
		t.Fatal("the press ran the key before the release")
	}
	if !model.frame.isHoldingKey() {
		t.Fatal("the key under the pressed button is not drawn pressed")
	}

	model.Update(tea.MouseMotionMsg{X: held.to + 3, Y: held.row + 2, Button: tea.MouseLeft})
	model.View()
	if model.frame.isHoldingKey() {
		t.Error("the key is drawn pressed after the pointer left it")
	}
	model.Update(tea.MouseReleaseMsg{X: held.to + 3, Y: held.row + 2, Button: tea.MouseLeft})
	if connection.Overlay.IsOpen() {
		t.Error("a release off the key ran it")
	}

	model.Update(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if connection.Overlay.Kind != app.OverlayPalette {
		t.Errorf("a release over the key opened %q", connection.Overlay.Kind)
	}
}

// A key press takes the mark off what the pointer stands on, and the next move of the pointer
// puts it back.
func TestAKeyPressHidesTheMarkOfThePointer(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 20, 4)
	connection := model.Active()
	connection.Tree.Expanded[core.BuildSchemaID("schema_00")] = true
	model.View()

	block := model.layout.treeRows
	model = movePointer(model, block.from+6, block.top+2)
	if !model.frame.hover.isSomething() {
		t.Fatal("a resting pointer marked nothing")
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model.View()
	if model.frame.hover.isSomething() {
		t.Error("the mark of the pointer stayed after a key press")
	}
	model = movePointer(model, block.from+6, block.top+3)
	if !model.frame.hover.isSomething() {
		t.Error("a move after the key press marked nothing")
	}
}

// The column the cursor stands in carries the ground of the row it stands on, so the two
// cross on the cell and a wide grid says where the cursor is.
func TestTheGridMarksTheColumnOfTheCursor(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 20, 6)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	tab.GridRow, tab.GridColumn = 3, 2
	frame := strings.Split(model.render(), "\n")

	crossing := columnHit{from: 1, to: 0}
	for _, column := range model.layout.gridColumns {
		if column.index == tab.GridColumn {
			crossing = column
		}
	}
	if crossing.to < crossing.from {
		t.Fatal("the column of the cursor was not drawn")
	}
	other := frame[model.layout.gridRows.top]
	marked := cutRowText(other, crossing.from, crossing.to)
	if strings.TrimSpace(marked) == "" {
		t.Fatalf("the column of the cursor covers %q on another row", marked)
	}
	cells := mapCells(other)
	if !holdsGround(cells[crossing.from].sgr, model.styles.Theme.Header) {
		t.Errorf("a cell of the cursor column on another row is drawn %q",
			cells[crossing.from].sgr)
	}
}

// holdsGround is true where the escapes of a cell lay this ground under it.
func holdsGround(sgr string, ground color.Color) bool {
	red, green, blue, _ := ground.RGBA()
	return strings.Contains(sgr, fmt.Sprintf("48;2;%d;%d;%d", red>>8, green>>8, blue>>8))
}

// The pointer marks a row with a tint of its ground and keeps its ink. The mark never takes
// the colours of the cursor.
func TestThePointerMarkIsNotTheCursor(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 20, 4)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneSidebar
	model.View()

	block := model.layout.treeRows
	cursor := block.top + connection.Tree.Cursor - block.offset
	marked := cursor + 1
	if marked >= block.top+block.count {
		marked = cursor - 1
	}
	before := mapCells(strings.Split(model.frame.shown, "\n")[marked])
	model = movePointer(model, block.from+6, marked)
	pointed := mapCells(strings.Split(model.frame.shown, "\n")[marked])

	accent := WriteHex(model.styles.Theme.Accent)
	for at := block.from + 1; at <= block.to-1 && at < len(pointed); at++ {
		_, ground := readCellColors(pointed[at].sgr)
		if ground == nil || WriteHex(ground) == accent {
			t.Fatalf("cell %d of the row under the pointer is drawn on %v", at, ground)
		}
		beforeInk, _ := readCellColors(before[at].sgr)
		pointedInk, _ := readCellColors(pointed[at].sgr)
		if strings.TrimSpace(pointed[at].text) != "" && beforeInk != nil &&
			WriteHex(beforeInk) == WriteHex(model.styles.Theme.Text) &&
			WriteHex(pointedInk) != WriteHex(beforeInk) {
			t.Errorf("cell %d changed its ink from %s to %s", at,
				WriteHex(beforeInk), WriteHex(pointedInk))
		}
	}
}

// A click that lit a key asks for a frame at the moment the light runs out. Without it the
// light waits for the next turn of the wheel, which rests a whole second, and a press that
// answered at once reads as one that stuck.
func TestAPressAsksForTheFrameThatPutsTheKeyOut(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select id from orders", 0)
	model.View()

	held := findButtonOfAction(model, ActionShowPalette)
	_, command := clickMouse(model, held.from, held.row)
	if command == nil {
		t.Fatal("a press that lit a key asked for no frame to put it out")
	}
	if !model.frame.isFlashing() {
		t.Fatal("the press left the key unlit")
	}

	// The wake carries no work of its own, so it starts no second wheel.
	if _, next := model.Update(wakeMsg{}); next != nil {
		t.Error("a wake started work of its own")
	}
	// The light is out once it has run out, whether a wheel turned or not.
	model.frame.pressedAt = model.frame.pressedAt.Add(-2 * keyFlashWait)
	if model.frame.isFlashing() {
		t.Error("the key is still lit after the light ran out")
	}
	lit := model.paintPressedKey(model.render())
	if lit != model.render() {
		t.Error("a key that ran out is still drawn lit")
	}
}

// The mark of the pointer belongs to a pointer that is resting. A drag belongs to what it
// began on, so nothing is marked while one runs.
func TestADragLeavesNothingMarked(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 20, 4)
	model.Active().Active().Focus = app.PaneEditor
	model.View()

	block := model.layout.treeRows
	model = movePointer(model, block.from+6, block.top+1)
	if !model.frame.hover.isSomething() {
		t.Fatal("a resting pointer marked nothing")
	}

	// A drag over the cells of the frame begins with a press.
	model.readMouse(tea.MouseClickMsg{
		X: model.layout.editorTextLeft, Y: model.layout.editorTextTop,
		Button: tea.MouseLeft,
	})
	model.readMouseMotion(tea.MouseMotionMsg{
		X: model.layout.editorTextLeft + 6, Y: model.layout.editorTextTop,
		Button: tea.MouseLeft,
	})
	model.View()
	if model.frame.hover.isSomething() {
		t.Errorf("a drag left %+v marked", model.frame.hover)
	}
}

// A key that ran something may take its own row off the frame. The light is dropped then,
// because the cells it covered now belong to whatever was drawn in their place.
func TestTheLightIsDroppedWhereTheKeyIsGone(t *testing.T) {
	model, connection, _ := buildMenuModel(t)
	model.View()

	held, found := findCardButton(model, ActionClose)
	if !found {
		t.Fatal("the card recorded no key that closes it")
	}
	clickMouse(model, held.from, held.row)
	if connection.Overlay.IsOpen() {
		t.Fatal("the press left the card open")
	}
	model.View()

	frame := model.render()
	if model.paintPressedKey(frame) != frame {
		t.Error("the light is drawn over cells the key no longer covers")
	}
}

// A key that stays where it was is lit, so a press that ran something in place still answers.
func TestTheLightStaysWhereTheKeyDoes(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	model.Active().Active().Focus = app.PaneResult
	model.View()

	held := findButtonOfAction(model, ActionSearchColumns)
	if held.row < 0 {
		t.Skip("the bar named no key that stays where it is")
	}
	clickMouse(model, held.from, held.row)
	model.View()
	if !strings.Contains(model.frame.shown, buildSgr(
		model.styles.Theme.OnAccent, model.styles.Theme.Accent)) {
		t.Error("the key that was pressed is not lit")
	}
}

// A drag over the cells of the frame draws nothing of its own: the cells it covers are laid
// over the frame afterwards, as the mark of the pointer is. Drawing the frame again for every
// cell the drag crosses is what a reader feels as a drag that drags.
func TestADragOverTheCellsDrawsNoNewFrame(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 40, 4)
	model.Active().Active().Focus = app.PaneResult
	model.View()

	block := model.layout.gridRows
	model.readMouse(tea.MouseClickMsg{
		X: block.from + 4, Y: block.top, Button: tea.MouseLeft,
	})
	model.View()

	// The first step is the one that gives the status bar a selection to report, so it
	// draws the frame. Every step after it lays the cells over the frame it left.
	model.readMouseMotion(tea.MouseMotionMsg{
		X: block.from + 20, Y: block.top + 1, Button: tea.MouseLeft,
	})
	model.View()
	frame := model.frame.text

	views := map[string]bool{}
	for at := 2; at < 7; at++ {
		model.readMouseMotion(tea.MouseMotionMsg{
			X: block.from + 20, Y: block.top + at, Button: tea.MouseLeft,
		})
		model.View()
		if model.frame.text != frame {
			t.Fatalf("the frame was drawn again for the step onto row %d", at)
		}
		views[model.frame.shown] = true
	}
	if len(views) != 5 {
		t.Errorf("five steps of the drag drew %d views", len(views))
	}
}

// The step that gives the status bar a selection to report changes the frame itself, so that
// one is drawn again.
func TestTheFirstStepOfADragDrawsTheFrame(t *testing.T) {
	model := buildLoadedModel(t, 2, 5, 40, 4)
	model.Active().Active().Focus = app.PaneResult
	model.View()

	block := model.layout.gridRows
	model.readMouse(tea.MouseClickMsg{
		X: block.from + 4, Y: block.top, Button: tea.MouseLeft,
	})
	model.View()
	frame := model.frame.text

	model.readMouseMotion(tea.MouseMotionMsg{
		X: block.from + 20, Y: block.top + 1, Button: tea.MouseLeft,
	})
	model.View()
	if model.frame.text == frame {
		t.Error("the step that gave the bar a selection to report drew no new frame")
	}
	if !strings.Contains(stripStyles(
		strings.Split(model.frame.shown, "\n")[model.layout.hintRow]), "copy selection") {
		t.Error("the bar does not offer the copy of the selection the drag made")
	}
}

// A card owns the pointer while it is open. Nothing behind it is marked, and nothing behind it
// answers a press: the panes are covered by the card or stand beside it, and a mark on one of
// them reads as a second thing happening at once.
func TestACardOnShowKeepsThePointerToItself(t *testing.T) {
	model := buildLoadedModel(t, 3, 6, 40, 6)
	connection := model.Active()
	connection.Tree.Expanded[core.BuildSchemaID("schema_00")] = true
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPalette, Draft: app.NewEditorBuffer("", 0),
		Palette: model.buildPaletteActions(connection),
	}
	model.View()

	// Every key and every bar the frame recorded belongs to the card or to the bar under
	// it, and none of them to a pane behind it.
	card := model.layout.overlayRows
	for _, held := range model.layout.buttons {
		if held.row == model.layout.hintRow {
			continue
		}
		if held.from < card.from || held.to > card.to {
			t.Errorf("the key of %q on row %d covers cells %d to %d, and the card holds "+
				"%d to %d", held.action, held.row, held.from, held.to, card.from, card.to)
		}
	}
	for _, bar := range model.layout.scrollbars {
		if bar.column < card.from || bar.column > card.to {
			t.Errorf("a bar stands in column %d, and the card holds %d to %d",
				bar.column, card.from, card.to)
		}
	}

	// A row of the card marks the cells of the card and nothing outside them, whatever the
	// pane behind it drew on the same row.
	for at := 0; at < 6 && at < card.count; at++ {
		row := card.top + at
		before := mapCells(strings.Split(model.frame.text, "\n")[row])
		model = movePointer(model, card.from+8, row)
		after := mapCells(strings.Split(model.frame.shown, "\n")[row])
		for cell := range after {
			if cell >= len(before) || before[cell].sgr == after[cell].sgr {
				continue
			}
			if cell < card.from || cell > card.to {
				t.Fatalf("hovering row %d of the card marked cell %d, which is outside "+
					"the card at %d to %d", at, cell, card.from, card.to)
			}
		}
	}
}

func TestAScrolledBlockTakesTheHoverOnEveryDrawnRow(t *testing.T) {
	block := rowsHit{top: 10, count: 5, offset: 20, from: 0, to: 50}
	if target := resolveRowHover(block, block.count, noFilledRow, 5, 14); !target.isSomething() {
		t.Error("the last drawn row of a scrolled block takes no hover")
	}
	if target := resolveRowHover(block, block.count, 22, 5, 12); target.isSomething() {
		t.Error("the row under the cursor takes the hover over its own ground")
	}
}

func TestAColumnCutAtThePaneEdgeCoversOnlyItsDrawnCells(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	shape := model.buildGridShape(connection, tab)
	model.recordGridColumns(shape, []int{0, 1}, 3, 20, 3, 0)
	last := model.layout.gridColumns[len(model.layout.gridColumns)-1]
	if last.to > model.editorLeft+20 {
		t.Errorf("the last column reaches cell %d, past the pane edge at %d",
			last.to, model.editorLeft+20)
	}
}

func TestTheRawPlanRecordsItsBarUnderTheStrip(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	tab := model.Active().Active()
	tab.RawPlan = true
	raw := make([]string, 0, 80)
	for range 80 {
		raw = append(raw, "Seq Scan on orders")
	}
	model.layout.detailTop = 12
	model.layout.scrollbars = nil
	model.renderPlan(tab, query.QueryPlan{Raw: strings.Join(raw, "\n")}, 80, 20, 12)
	if len(model.layout.scrollbars) != 1 || model.layout.scrollbars[0].top != 13 {
		t.Errorf("the bar was recorded at %+v, wanted the row under the strip", model.layout.scrollbars)
	}
}
