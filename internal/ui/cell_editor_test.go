package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
)

// buildOpenCellEditor answers a model whose cell editor card holds that text, with the caret
// at the offset.
func buildOpenCellEditor(t *testing.T, written string, caret int) (*Model, *app.Overlay) {
	t.Helper()
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.Active().Focus = app.PaneResult
	connection.Open(buildCellEditor(app.Overlay{Kind: app.OverlayCellEdit}, written))
	connection.Overlay.Draft.PlaceCaret(caret, false)
	return model, &connection.Overlay
}

// Home and End in a field of several lines move within the line of the caret. Home stops at
// the first character after the indent, as it does in the editor.
func TestHomeAndEndInTheCellEditorStayOnTheLine(t *testing.T) {
	model, overlay := buildOpenCellEditor(t, "{\n  \"note\": \"first\"\n}", 6)

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyHome})
	if overlay.Draft.Caret != 4 {
		t.Errorf("home put the caret at %d, wanted 4", overlay.Draft.Caret)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	if overlay.Draft.Caret != 19 {
		t.Errorf("end put the caret at %d, wanted 19", overlay.Draft.Caret)
	}
}

// Ctrl+A selects the whole value, as it does in the editor.
func TestCtrlAInTheCellEditorSelectsTheValue(t *testing.T) {
	model, overlay := buildOpenCellEditor(t, "one\ntwo", 2)

	pressKey(t, model, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})

	if overlay.Draft.Selection() != "one\ntwo" {
		t.Errorf("ctrl+a selected %q", overlay.Draft.Selection())
	}
}

// A line wider than the card moves left, so the caret at its end is drawn.
func TestTheCellEditorDrawsTheCaretOfALongLine(t *testing.T) {
	written := strings.Repeat("x", 300) + "tail"
	model, overlay := buildOpenCellEditor(t, written, len(written))

	rows := model.renderDraftRows(overlay.Draft, 40, 3, FieldLook{
		Ground: model.styles.Theme.Panel, Ink: model.styles.Theme.Text, Focused: true,
	})

	if !strings.Contains(stripEscapes(rows[0]), "tail") {
		t.Errorf("the row is %q, wanted the end of the line", stripEscapes(rows[0]))
	}
}

func TestBuildCellEditorOpensAFieldSixRowsTall(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{Kind: app.OverlayCellEdit}, "one line")

	if overlay.Draft == nil || overlay.Draft.Text != "one line" {
		t.Fatalf("the field holds %+v", overlay.Draft)
	}
	if overlay.Draft.Caret != len("one line") {
		t.Errorf("the caret stands at %d, wanted the end of the value", overlay.Draft.Caret)
	}
	if overlay.ContentRows != minCellEditorRows {
		t.Errorf("the card gives %d rows, wanted %d", overlay.ContentRows, minCellEditorRows)
	}
}

func TestBuildCellEditorGrowsForAValueOfManyLines(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{Kind: app.OverlayCellEdit},
		"1\n2\n3\n4\n5\n6\n7\n8")

	if overlay.ContentRows != 8 {
		t.Errorf("the card gives %d rows, wanted 8", overlay.ContentRows)
	}
}

func TestBuildCellEditorMarksTheValueTheCellHolds(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit, Cell: app.CellTarget{Choices: []string{"pending", "packed", "sent"}},
	}, "packed")

	if overlay.Draft != nil {
		t.Error("a cell that is picked holds a field")
	}
	if overlay.List.Cursor != 1 {
		t.Errorf("the cursor stands on %d, wanted 1", overlay.List.Cursor)
	}
	if overlay.ContentRows != 3 {
		t.Errorf("the card gives %d rows, wanted 3", overlay.ContentRows)
	}
}

func TestBuildCellEditorMarksTheFirstValueForACellWithNone(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit, Cell: app.CellTarget{Choices: []string{"true", "false"}},
	}, "")

	if overlay.List.Cursor != 0 {
		t.Errorf("the cursor stands on %d, wanted the first row", overlay.List.Cursor)
	}
}
