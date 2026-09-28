package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
)

func TestADetailViewTakesNoGridKey(t *testing.T) {
	model, connection, tab := openOrdersRowCard(t, false)
	connection.CloseEveryOverlay()
	tab.View = app.ViewFields

	model.readKey(tea.Key{Code: 'm', Text: "m"})
	if connection.Overlay.IsOpen() {
		t.Errorf("m on the fields view opened %q", connection.Overlay.Kind)
	}
}
