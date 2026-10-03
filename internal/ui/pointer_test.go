package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestThePointerShapeFollowsWhatItStandsOn(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select id from orders", 0)
	model.View()
	layout := model.layout

	held := findButtonOfAction(model, ActionShowPalette)
	for _, step := range []struct {
		name string
		x, y int
		want string
	}{
		{"a key of the title bar", held.from, held.row, pointerHand},
		{"the text of the editor", layout.editorTextLeft + 2, layout.editorTextTop, pointerText},
		{"the line under the editor", layout.paneFrom + 4,
			layout.editorTop + layout.editorRows - 1, pointerRows},
		{"the border of the explorer", layout.treeTo, layout.editorTop + 1, pointerColumns},
	} {
		_, command := model.Update(tea.MouseMotionMsg{X: step.x, Y: step.y})
		if model.pointerShape != step.want {
			t.Errorf("over %s the pointer is %q, wanted %q", step.name, model.pointerShape,
				step.want)
		}
		if command == nil {
			t.Errorf("the move over %s sent no shape", step.name)
		}
	}
	if _, command := model.Update(tea.MouseMotionMsg{X: layout.treeTo, Y: layout.editorTop + 2}); command != nil {
		t.Error("a move that keeps the shape sent it again")
	}
}
