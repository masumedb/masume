package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestALongLineBreaksAfterItsLastBlank(t *testing.T) {
	rows := buildWrapRows([]string{"select id, name from customers", ""}, 12)
	parts := []string{}
	for _, row := range rows {
		parts = append(parts, []string{"select id, name from customers", ""}[row.line][row.from:row.to])
	}
	want := []string{"select id, ", "name from ", "customers", ""}
	if strings.Join(parts, "|") != strings.Join(want, "|") {
		t.Errorf("the rows read %q, wanted %q", parts, want)
	}
	if !rows[2].last || rows[0].last || rows[1].startCell != 11 {
		t.Errorf("the rows are %+v", rows)
	}
}

func TestTheEditorWrapsALongLineWhileWrapIsOn(t *testing.T) {
	text := "select " + strings.Repeat("column_name, ", 20) + "id from orders"
	model, _, tab := buildEditingModel(t, text, len(text))
	model.settings.WrapLines = true
	frame := strings.Split(stripEscapes(model.render()), "\n")

	rows := model.layout.editorWrapRows
	if len(rows) < 2 {
		t.Fatalf("the long line took %d rows", len(rows))
	}
	top := model.layout.editorTextTop
	if !strings.Contains(frame[top], " 1 ") || strings.Contains(frame[top+1], " 2 ") {
		t.Errorf("the gutter reads %q and %q", frame[top], frame[top+1])
	}
	if !strings.Contains(frame[top+len(rows)-1], "id from orders") {
		t.Errorf("the last row reads %q", frame[top+len(rows)-1])
	}

	// A press on the second row places the caret in the text of that row.
	model.readMouse(tea.MouseClickMsg{
		X: model.layout.editorTextLeft, Y: top + 1, Button: tea.MouseLeft,
	})
	if caret := tab.Editor.Caret; caret != rows[1].from {
		t.Errorf("a press on the start of the second row put the caret at %d, wanted %d",
			caret, rows[1].from)
	}
}

func TestTheWrapKeyTurnsWrapOnAndOff(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select 1", 0)
	model.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	if !model.settings.WrapLines {
		t.Fatal("alt+l left wrap off")
	}
	model.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModAlt})
	if model.settings.WrapLines {
		t.Error("the second alt+l left wrap on")
	}
}

func TestUpAndDownMoveByWrappedRowsWhileWrapIsOn(t *testing.T) {
	text := "select " + strings.Repeat("column_name, ", 20) + "id from orders"
	model, _, tab := buildEditingModel(t, text, 3)
	model.settings.WrapLines = true
	model.render()
	rows := model.layout.editorWrapRows

	model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if caret := tab.Editor.Caret; caret != rows[1].from+3 {
		t.Errorf("down from the first row put the caret at %d, wanted %d", caret, rows[1].from+3)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if caret := tab.Editor.Caret; caret != 3 {
		t.Errorf("up from the second row put the caret at %d, wanted 3", caret)
	}
}
