package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
)

func TestTheWhereFieldCompletesTheColumnsOfTheResult(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	tab.Focus = app.PaneResult
	connection.Open(app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptWhere, Title: "where",
		Draft: app.NewEditorBuffer("", 0),
	})
	model.render()

	for _, character := range "cust" {
		model.Update(tea.KeyPressMsg{Code: character, Text: string(character)})
	}
	chosen, listing := tab.Completion.Chosen()
	if !listing || chosen.Text != "customer" {
		t.Fatalf("cust in the where field offered %v", tab.Completion.Candidates)
	}
	frame := model.render()
	if !strings.Contains(stripEscapes(frame), "customer") {
		t.Error("the list of the where field was not drawn")
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if text := connection.Overlay.Draft.Text; text != "customer" {
		t.Errorf("tab wrote %q into the where field", text)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if tab.Completion.IsListing() {
		t.Error("the list stayed open after the where field closed")
	}
}
