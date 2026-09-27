package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
)

func TestTheDefinitionOpensInAQueryTab(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	connection := model.Active()
	tab := connection.Active()
	tab.ViewData = app.PaneContent{
		Kind: app.DataDDL, Lines: []string{"CREATE TABLE public.orders (", "    id integer", ");"},
	}

	model.runDefinitionAction(connection, tab, Match{Action: ActionEditDefinition})
	opened := connection.Active()
	if opened.Editor.Text != "CREATE TABLE public.orders (\n    id integer\n);" {
		t.Errorf("the editor holds %q", opened.Editor.Text)
	}
	if opened.Focus != app.PaneEditor {
		t.Errorf("the focus is on %q", opened.Focus)
	}
}
