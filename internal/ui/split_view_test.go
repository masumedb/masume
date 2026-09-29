package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/present"
)

var (
	splitViewKey = tea.KeyPressMsg{Code: 'v', Text: "v", Mod: tea.ModAlt}
	flipSplitKey = tea.KeyPressMsg{Code: 'v', ShiftedCode: 'V', Text: "V",
		Mod: tea.ModAlt | tea.ModShift}
	focusPaneKey = tea.KeyPressMsg{Code: 'p', Text: "p", Mod: tea.ModAlt}
	otherSideKey = tea.KeyPressMsg{Code: 'v', Text: "v"}
)

// buildSplitModel returns a model with two query tabs in a split view.
func buildSplitModel(t *testing.T) (*Model, *app.Connection) {
	t.Helper()
	model := buildLoadedModel(t, 1, 3, 40, 3)
	connection := model.Active()
	connection.OpenQueryTab("select 1").Focus = app.PaneEditor
	connection.ActivateTab(0)
	pressKey(t, model, splitViewKey)
	model.render()
	return model, connection
}

// findSideTab returns the tab a side of the split view shows.
func findSideTab(t *testing.T, model *Model, side int) *app.Tab {
	t.Helper()
	_, _, tab, found := model.findSide(model.split.sides[side])
	if !found {
		t.Fatalf("side %d shows no open tab", side)
	}
	return tab
}

func TestASplitShowsTheNextTabBesideTheTabOnScreen(t *testing.T) {
	model, connection := buildSplitModel(t)

	if !model.split.open || !model.layout.divider.drawn {
		t.Fatal("Alt+V drew no split view")
	}
	if findSideTab(t, model, 0) != connection.Tabs[0] ||
		findSideTab(t, model, 1) != connection.Tabs[1] {
		t.Error("the sides do not show the tab on screen and the next one")
	}
	if model.split.focused != 1 || connection.Active() != connection.Tabs[1] {
		t.Error("the second side did not take the focus")
	}
	panes := readFrameRows(model.render())[firstPaneRow]
	if strings.Count(panes, "─ query ") != 2 {
		t.Errorf("the top row of the panes shows one side only: %q", panes)
	}

	pressKey(t, model, splitViewKey)
	model.render()
	if model.split.open || model.layout.divider.drawn {
		t.Error("a second Alt+V left the split open")
	}
	if connection.Active() != connection.Tabs[1] {
		t.Error("closing the split moved the focus off the side that had it")
	}
}

func TestASplitOfOneTabOpensAQueryTab(t *testing.T) {
	model := buildOfflineModel(t, 160, 40)
	connection := model.Active()
	first := connection.Active()
	first.Focus = app.PaneResult

	pressKey(t, model, splitViewKey)
	if len(connection.Tabs) != 2 {
		t.Fatalf("the split left %d tabs, wanted a new one", len(connection.Tabs))
	}
	if findSideTab(t, model, 0) != first || findSideTab(t, model, 1) != connection.Tabs[1] {
		t.Error("the new query tab is not on the second side")
	}
	if connection.Active().Focus != app.PaneEditor {
		t.Error("the new query tab opened without the caret in the editor")
	}
}

func TestTheOtherSideKeyMovesTheFocus(t *testing.T) {
	model, connection := buildSplitModel(t)

	pressKey(t, model, focusPaneKey)
	pressKey(t, model, otherSideKey)
	if model.split.focused != 0 || connection.Active() != connection.Tabs[0] {
		t.Error("Alt+P V left the focus on the second side")
	}
	pressKey(t, model, focusPaneKey)
	pressKey(t, model, otherSideKey)
	if model.split.focused != 1 || connection.Active() != connection.Tabs[1] {
		t.Error("a second Alt+P V left the focus on the first side")
	}
}

func TestSelectingTheTabOfTheOtherSideMovesTheFocus(t *testing.T) {
	model, connection := buildSplitModel(t)

	pressKey(t, model, tea.KeyPressMsg{Code: '1', Text: "1", Mod: tea.ModAlt})
	if model.split.focused != 0 {
		t.Errorf("Alt+1 left the focus on side %d", model.split.focused)
	}
	if findSideTab(t, model, 1) != connection.Tabs[1] {
		t.Error("Alt+1 moved the tab of the second side")
	}
}

func TestANewTabOpensOnTheSideWithTheFocus(t *testing.T) {
	model, connection := buildSplitModel(t)

	pressKey(t, model, tea.KeyPressMsg{Code: 'n', Text: "n", Mod: tea.ModAlt})
	if findSideTab(t, model, 1) != connection.Tabs[2] {
		t.Error("the new tab is not on the side with the focus")
	}
	if findSideTab(t, model, 0) != connection.Tabs[0] {
		t.Error("the new tab moved the first side")
	}
}

func TestClosingTheTabOfASideClosesTheSplit(t *testing.T) {
	for _, held := range []struct {
		name  string
		close func(*testing.T, *Model)
	}{
		{"the side with the focus", func(t *testing.T, model *Model) {
			pressKey(t, model, tea.KeyPressMsg{Code: 'w', Text: "w", Mod: tea.ModAlt})
		}},
		{"the other side", func(t *testing.T, model *Model) {
			hit := model.layout.tabs[0]
			model.Update(tea.MouseClickMsg{
				X: hit.from, Y: model.layout.tabRow, Button: tea.MouseMiddle,
			})
		}},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection := buildSplitModel(t)
			connection.OpenQueryTab("select 2")
			connection.ActivateTab(1)
			model.render()
			kept := findSideTab(t, model, 0)
			if held.name == "the other side" {
				kept = findSideTab(t, model, 1)
			}

			held.close(t, model)
			if model.split.open {
				t.Fatal("the split stayed open without the tab of one side")
			}
			if connection.Active() != kept {
				t.Error("the tab of the side left open did not take the focus")
			}
		})
	}
}

func TestTwoConnectionsSideBySideShowTheirNames(t *testing.T) {
	model, _ := buildSplitModel(t)
	openSecondConnection(t, model)
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	panes := readFrameRows(model.render())[firstPaneRow]
	if !strings.Contains(panes, "offline · ") || !strings.Contains(panes, "second · ") {
		t.Errorf("the sides do not show their connection names: %q", panes)
	}
	if model.Active().Profile().Name != "second" || model.split.focused != 1 {
		t.Error("the new connection is not on the side with the focus")
	}
}

func TestSwitchingToTheConnectionOfTheOtherSideMovesTheFocus(t *testing.T) {
	model, first := buildSplitModel(t)
	second := openSecondConnection(t, model)
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if model.Active() != first || model.split.focused != 0 {
		t.Fatal("Alt+Left did not move the focus to the side of the first connection")
	}
	if findSideTab(t, model, 1) != second.Active() {
		t.Error("Alt+Left moved the tab of the second side")
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	if model.Active() != second || model.split.focused != 1 {
		t.Error("Alt+Right did not move the focus back to the second side")
	}
}

func TestClosingTheConnectionOfASideClosesTheSplit(t *testing.T) {
	model, first := buildSplitModel(t)
	openSecondConnection(t, model)
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	kept := findSideTab(t, model, 0)

	pressKey(t, model, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if model.split.open {
		t.Fatal("the split stayed open without the connection of one side")
	}
	if model.Active() != first || first.Active() != kept {
		t.Error("the side left open did not take the focus")
	}
}

func TestTabStepsThroughTheExplorerAndBothSides(t *testing.T) {
	model, connection := buildSplitModel(t)
	connection.Tabs[0].Focus = app.PaneResult
	connection.Tabs[1].Focus = app.PaneSidebar
	model.render()

	wanted := []struct {
		side int
		pane app.Pane
	}{
		{0, app.PaneEditor},
		{0, app.PaneResult},
		{1, app.PaneEditor},
		{1, app.PaneResult},
		{1, app.PaneSidebar},
	}
	for at, step := range wanted {
		pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
		model.render()
		if model.split.focused != step.side || model.Active().Active().Focus != step.pane {
			t.Fatalf("Tab %d left the focus on side %d, %s; wanted side %d, %s", at+1,
				model.split.focused, model.Active().Active().Focus, step.side, step.pane)
		}
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if model.split.focused != 1 || model.Active().Active().Focus != app.PaneResult {
		t.Error("Shift+Tab from the explorer did not reach the last pane of the second side")
	}
}

func TestAPressOnTheOtherSideGivesItTheFocus(t *testing.T) {
	model, connection := buildSplitModel(t)
	aside := model.layout.aside
	row := aside.toY - 3

	model.Update(tea.MouseClickMsg{X: aside.fromX + 8, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: aside.fromX + 8, Y: row, Button: tea.MouseLeft})
	if model.split.focused != 0 || connection.Active() != connection.Tabs[0] {
		t.Fatal("a press on the first side left the focus on the second side")
	}
	if connection.Tabs[0].Focus != app.PaneResult {
		t.Error("the press did not reach the grid of the first side")
	}
}

func TestTheWheelScrollsTheOtherSideAndKeepsTheFocus(t *testing.T) {
	model, connection := buildSplitModel(t)
	aside := model.layout.aside
	table := connection.Tabs[0]
	before := table.GridRowOffset

	model.Update(tea.MouseWheelMsg{
		X: aside.fromX + 8, Y: aside.fromY + 10, Button: tea.MouseWheelDown,
	})
	if table.GridRowOffset != before+wheelRows {
		t.Errorf("the wheel left the grid of the first side at %d, wanted %d",
			table.GridRowOffset, before+wheelRows)
	}
	if model.split.focused != 1 || connection.Active() != connection.Tabs[1] {
		t.Error("the wheel moved the focus")
	}
}

func TestADragOfTheDividerResizesTheSides(t *testing.T) {
	model, _ := buildSplitModel(t)
	divider := model.layout.divider
	row := divider.from + 4
	firstWidth := divider.at - divider.start + 1

	model.Update(tea.MouseClickMsg{X: divider.at, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: divider.at - 10, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: divider.at - 10, Y: row, Button: tea.MouseLeft})
	model.render()
	if dragged := model.layout.divider.at - divider.start + 1; dragged != firstWidth-10 {
		t.Errorf("a drag of ten columns left the first side %d wide, and it was %d",
			dragged, firstWidth)
	}
	if model.split.focused != 1 {
		t.Error("a drag of the divider moved the focus")
	}

	model.clicks = clickCounter{}
	model.Update(tea.MouseClickMsg{X: model.layout.divider.at, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: 0, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: 0, Y: row, Button: tea.MouseLeft})
	model.render()
	if narrowest := model.layout.divider.at - divider.start + 1; narrowest != sideFloorColumns {
		t.Errorf("a drag past the explorer left the first side %d wide, wanted %d",
			narrowest, sideFloorColumns)
	}

	for range 2 {
		model.Update(tea.MouseClickMsg{
			X: model.layout.divider.at, Y: row, Button: tea.MouseLeft,
		})
		model.Update(tea.MouseReleaseMsg{
			X: model.layout.divider.at, Y: row, Button: tea.MouseLeft,
		})
	}
	model.render()
	if even := model.layout.divider.at - divider.start + 1; even != divider.room/2 {
		t.Errorf("a second press left the first side %d wide, wanted %d",
			even, divider.room/2)
	}
}

func TestAStackedSplitDrawsOneSideOverTheOther(t *testing.T) {
	model, connection := buildSplitModel(t)

	pressKey(t, model, flipSplitKey)
	model.render()
	divider := model.layout.divider
	if !model.split.stacked || !divider.stacked {
		t.Fatal("Alt+Shift+V did not stack the sides")
	}
	if model.layout.editorTop <= firstPaneRow {
		t.Errorf("the second side starts on row %d, over the first side",
			model.layout.editorTop)
	}
	rows := readFrameRows(model.render())
	for _, top := range []int{firstPaneRow, divider.at + 1} {
		if !strings.Contains(rows[top], "╭─ query ") {
			t.Errorf("row %d is not the top of a side: %q", top, rows[top])
		}
	}

	pressKey(t, model, focusPaneKey)
	pressKey(t, model, otherSideKey)
	model.render()
	if model.layout.editorTop != firstPaneRow {
		t.Errorf("the side on top draws its editor on row %d", model.layout.editorTop)
	}
	if connection.Active() != connection.Tabs[0] {
		t.Error("the focus did not move to the side on top")
	}
}

func TestASplitFallsBackToTheRoomOfTheTerminal(t *testing.T) {
	for _, held := range []struct {
		name          string
		width, height int
		wanted        sideArrangement
	}{
		{"wide", 160, 40, arrangeAcross},
		{"narrow", 36, 40, arrangeStacked},
		{"small", 36, 12, arrangeSingle},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, _ := buildSplitModel(t)
			model.Update(tea.WindowSizeMsg{Width: held.width, Height: held.height})
			model.render()
			arranged := model.resolveSideArrangement(model.layout.bodyRows)
			if arranged != held.wanted {
				t.Errorf("the split is drawn as %d, wanted %d", arranged, held.wanted)
			}
			if model.layout.divider.drawn != (held.wanted != arrangeSingle) {
				t.Error("the divider does not follow how the split is drawn")
			}
			for at, drawn := range readFrameRows(model.render()) {
				if measured := measureStyledWidth(drawn); measured != held.width {
					t.Errorf("row %d measures %d, wanted %d", at, measured, held.width)
				}
			}
		})
	}
}

func TestTheOtherSideDrawsNoFocusedBorder(t *testing.T) {
	model, connection := buildSplitModel(t)
	connection.Tabs[0].Focus = app.PaneResult
	theme := model.styles.Theme
	focusedBorder := resolveOpening(theme.BorderFocus, theme.Panel)

	aside := model.layout.aside
	for _, row := range strings.Split(model.render(), "\n")[firstPaneRow:] {
		_, rest, _ := cutRow(row, aside.fromX)
		first, second, _ := cutRow(rest, aside.toX-aside.fromX+1)
		if strings.Contains(first, focusedBorder+borderVertical) {
			t.Fatalf("the first side draws a focused border: %q", first)
		}
		if strings.Contains(second, focusedBorder+borderVertical) {
			return
		}
	}
	t.Error("the side with the focus draws no focused border")
}

func TestTheTabRowMarksTheTabOfTheOtherSide(t *testing.T) {
	model, _ := buildSplitModel(t)
	theme := model.styles.Theme
	row := strings.Split(model.render(), "\n")[tabRowIndex]

	if !strings.Contains(row, resolveOpening(theme.Muted, theme.Selection)+" 1 ") {
		t.Errorf("the tab of the first side is not marked: %q", row)
	}
	if !strings.Contains(row, resolveOpening(theme.OnAccent, theme.Accent)+" 2 ") {
		t.Errorf("the tab with the focus is not drawn as active: %q", row)
	}
}

func TestTheTableMenuLeadsWithTheWaysToOpenTheTable(t *testing.T) {
	for _, held := range []struct {
		name         string
		capabilities core.Capabilities
	}{
		{"a server that writes DDL", core.Capabilities{WritesDDL: true}},
		{"a server that writes no DDL", core.Capabilities{}},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, session := buildTreeDumpModel(t)
			session.offlineSession.capabilities = held.capabilities
			openObjectMenu(t, model, connection, present.NodeTable)

			wanted := []string{app.ObjectOpen, app.ObjectOpenInNewTab, app.ObjectOpenInSplit}
			actions := connection.Overlay.Actions
			if len(actions) < len(wanted) {
				t.Fatalf("the menu has %d rows", len(actions))
			}
			for at, id := range wanted {
				if actions[at].ID != id {
					t.Errorf("row %d is %q, wanted %q", at, actions[at].ID, id)
				}
			}
		})
	}
}

func TestOpenInSplitViewShowsTheTableBesideTheTabOnScreen(t *testing.T) {
	model, connection, _ := buildTreeDumpModel(t)
	first := connection.Active()
	openObjectMenu(t, model, connection, present.NodeTable)
	chooseMenuRow(t, model, connection, app.ObjectOpenInSplit)

	if !model.split.open || model.split.focused != 1 {
		t.Fatal("the menu row opened no split with the focus on the second side")
	}
	table := findSideTab(t, model, 1)
	if findSideTab(t, model, 0) != first || table.Kind != app.TabTable ||
		table.Table.Name != "orders" {
		t.Error("the sides do not show the tab on screen and the table")
	}

	model.render()
	pressKey(t, model, focusPaneKey)
	pressKey(t, model, otherSideKey)
	openObjectMenu(t, model, connection, present.NodeTable)
	chooseMenuRow(t, model, connection, app.ObjectOpenInSplit)
	if model.split.focused != 1 || findSideTab(t, model, 1) != table {
		t.Error("the table did not come back to the other side")
	}
	if findSideTab(t, model, 0) != first {
		t.Error("the side the menu was opened on lost its tab")
	}
}

func TestOpenInSplitViewOfTheTableOnScreenOpensASecondTab(t *testing.T) {
	model, connection, _ := buildTreeDumpModel(t)
	openObjectMenu(t, model, connection, present.NodeTable)
	chooseMenuRow(t, model, connection, app.ObjectOpen)
	table := connection.Active()
	if table.Kind != app.TabTable {
		t.Fatal("Open did not open the table")
	}

	openObjectMenu(t, model, connection, present.NodeTable)
	chooseMenuRow(t, model, connection, app.ObjectOpenInSplit)
	second := findSideTab(t, model, 1)
	if findSideTab(t, model, 0) != table || second == table || second.Table.Name != "orders" {
		t.Error("the table is not on both sides in two tabs")
	}
}
