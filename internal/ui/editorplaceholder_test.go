package ui

import (
	"strings"
	"testing"
)

func TestTheEmptyEditorShowsTheShapeOfAStatementAndTheRunKey(t *testing.T) {
	model := buildOfflineModel(t, 140, 40)
	connection := model.Active()
	tab := connection.Active()

	drawn := model.renderEditorPlaceholder(connection, tab, 60, true)
	text := stripEscapes(drawn)
	if !strings.HasPrefix(text, " "+describeEditorHint(connection, tab)) ||
		!strings.Contains(text, "run") {
		t.Errorf("the placeholder reads %q", text)
	}
	if !strings.Contains(drawn, "\x1b[3;") {
		t.Errorf("the placeholder is not in italics: %q", drawn)
	}
}
