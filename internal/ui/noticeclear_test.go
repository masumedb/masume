package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTheNextKeyClearsAReport(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	connection := model.Active()
	connection.Show("id has no foreign key")
	model.readKey(tea.Key{Code: tea.KeyDown})
	if connection.Notice != nil {
		t.Errorf("the report %q is still shown", connection.Notice.Text)
	}
}

func TestTheNextKeyKeepsAnError(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	connection := model.Active()
	connection.ShowError("the server refused the write")
	model.readKey(tea.Key{Code: tea.KeyDown})
	if connection.Notice == nil {
		t.Error("the error was cleared by the next key")
	}
}
