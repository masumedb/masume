package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
)

// dragPointer presses at one cell, moves to another and releases.
func dragPointer(t *testing.T, model *Model, fromX, fromY, toX, toY int) *Model {
	t.Helper()
	held, _ := model.Update(tea.MouseClickMsg{X: fromX, Y: fromY, Button: tea.MouseLeft})
	model = held.(*Model)
	held, _ = model.Update(tea.MouseMotionMsg{X: toX, Y: toY, Button: tea.MouseLeft})
	model = held.(*Model)
	held, _ = model.Update(tea.MouseReleaseMsg{X: toX, Y: toY, Button: tea.MouseLeft})
	return held.(*Model)
}

// pressPointer presses and releases at one cell, without moving.
func pressPointer(t *testing.T, model *Model, x, y int) *Model {
	t.Helper()
	held, _ := model.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	model = held.(*Model)
	held, _ = model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return held.(*Model)
}

// The tree and the panes stand side by side, and the border between them sets the width of
// the tree, as the line between the editor and the result sets their heights.
func TestADragOfTheTreeBorderSetsTheWidthOfTheTree(t *testing.T) {
	model := buildOfflineModel(t, 140, 40)
	_ = model.View()
	border := model.layout.treeTo

	model = dragPointer(t, model, border, model.layout.editorTop+2, 59, model.layout.editorTop+2)
	if held := model.Active().SidebarWidth; held != 60 {
		t.Errorf("the tree asks for %d columns, wanted 60", held)
	}
	_ = model.View()
	if model.layout.treeTo != 59 {
		t.Errorf("the border draws at %d, wanted 59", model.layout.treeTo)
	}

	// A drag past either end stops at the narrowest tree and the narrowest pane.
	model = dragPointer(t, model, model.layout.treeTo, 5, 1, 5)
	_ = model.View()
	if model.layout.treeTo+1 != present.MinSidebarWidth {
		t.Errorf("a drag to the left edge draws %d columns, wanted %d",
			model.layout.treeTo+1, present.MinSidebarWidth)
	}

	model = dragPointer(t, model, model.layout.treeTo, 5, 138, 5)
	_ = model.View()
	if model.layout.treeTo+1 > 140-narrowestPaneWidth {
		t.Errorf("a drag to the right edge draws %d columns, wanted %d at most",
			model.layout.treeTo+1, 140-narrowestPaneWidth)
	}
}

// The line between the editor and the result is drawn as two rows: the foot of the editor
// and the head of the result. A drag on either one moves it by the pointer distance.
func TestADragOfEitherSideOfTheSplitMovesTheLine(t *testing.T) {
	for _, held := range []struct {
		name string
		row  func(model *Model) int
	}{
		{"the foot of the editor", func(model *Model) int {
			return model.layout.editorTop + model.layout.editorRows - 1
		}},
		{"the head of the result", func(model *Model) int { return model.layout.resultTop }},
	} {
		t.Run(held.name, func(t *testing.T) {
			model := buildOfflineModel(t, 140, 40)
			_ = model.View()
			from := held.row(model)
			line := model.layout.editorTop + model.layout.editorRows - 1

			model = dragPointer(t, model, 60, from, 60, from+6)
			_ = model.View()
			if model.layout.editorTop+model.layout.editorRows-1 != line+6 {
				t.Errorf("the line draws at %d, wanted %d",
					model.layout.editorTop+model.layout.editorRows-1, line+6)
			}
		})
	}
}

// A press that never moves is no resize, so each border does what the pane under it does.
func TestAPressOnABorderThatNeverMovedReachesItsPane(t *testing.T) {
	model := buildOfflineModel(t, 140, 40)
	_ = model.View()

	// The foot of the editor hides the result, which is what it did before it could drag.
	model = pressPointer(t, model, 60, model.layout.editorTop+model.layout.editorRows-1)
	if model.Active().ResultVisible {
		t.Error("a press on the foot of the editor left the result on screen")
	}
	model = pressPointer(t, model, 60, model.layout.editorTop+model.layout.editorRows-1)
	if !model.Active().ResultVisible {
		t.Error("a second press did not bring the result back")
	}

	_ = model.View()
	// The head of the result reaches the result and keeps it on screen.
	model = pressPointer(t, model, 60, model.layout.resultTop)
	if !model.Active().ResultVisible {
		t.Error("a press on the head of the result hid it")
	}
	if focus := model.Active().Active().Focus; focus != app.PaneResult {
		t.Errorf("the keyboard is on %q, wanted the result", focus)
	}

	_ = model.View()
	// The border of the tree reaches the tree and changes no width.
	before := model.Active().SidebarWidth
	model = pressPointer(t, model, model.layout.treeTo, model.layout.editorTop+2)
	if held := model.Active().SidebarWidth; held != before {
		t.Errorf("a press on the tree border set the width to %d, wanted %d", held, before)
	}
	if focus := model.Active().Active().Focus; focus != app.PaneSidebar {
		t.Errorf("the keyboard is on %q, wanted the tree", focus)
	}
}

// The tree divider is two columns: the tree border and the pane border. A drag on either
// column, on any row, moves the divider by the pointer distance.
func TestADragOfEitherSideOfTheTreeBorderOnAnyRowMovesIt(t *testing.T) {
	base := buildLoadedModel(t, 2, 40, 60, 6)
	base.render()
	border := base.layout.treeTo
	bottom := firstPaneRow + base.layout.editorRows + base.layout.resultRows
	if base.layout.connections.count == 0 || base.layout.treeRows.count == 0 {
		t.Fatal("the tree draws no connection row or no tree row")
	}
	for _, column := range []int{border, border + 1} {
		for row := firstPaneRow; row < bottom; row++ {
			if _, _, onLine := base.findSplitLine(column, row); onLine {
				continue
			}
			model := buildLoadedModel(t, 2, 40, 60, 6)
			model.render()
			model = dragPointer(t, model, column, row, column+5, row)
			model.render()
			if model.layout.treeTo != border+5 {
				t.Errorf("a drag from column %d on row %d draws the border at %d, wanted %d",
					column, row, model.layout.treeTo, border+5)
			}
		}
	}
}

// A press without motion on the pane border beside the tree focuses that pane.
func TestAPressOnThePaneSideOfTheTreeBorderReachesThePane(t *testing.T) {
	model := buildLoadedModel(t, 2, 40, 60, 6)
	model.render()
	before := model.Active().SidebarWidth
	column := model.layout.treeTo + 1

	model = pressPointer(t, model, column, model.layout.editorTop+2)
	if focus := model.Active().Active().Focus; focus != app.PaneEditor {
		t.Errorf("the keyboard is on %q, wanted the editor", focus)
	}
	model = pressPointer(t, model, column, model.layout.resultTop+2)
	if focus := model.Active().Active().Focus; focus != app.PaneResult {
		t.Errorf("the keyboard is on %q, wanted the result", focus)
	}
	if held := model.Active().SidebarWidth; held != before {
		t.Errorf("a press set the width to %d, wanted %d", held, before)
	}
}

// The tree border draws no hover mark on a tree row.
func TestTheTreeBorderMarksNoTreeRow(t *testing.T) {
	model := buildLoadedModel(t, 2, 40, 60, 6)
	model.render()
	row := model.layout.treeRows.top + 1
	if target := model.resolveHover(model.layout.treeTo-1, row); !target.isSomething() {
		t.Fatal("the tree row takes no mark inside the tree")
	}
	if target := model.resolveHover(model.layout.treeTo, row); target.isSomething() {
		t.Errorf("the tree border marks %+v", target)
	}
}
