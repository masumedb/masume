package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query"
)

// buildDiagramModel returns a model with the diagram of shop.orders open, which refers to
// shop.customers.
func buildDiagramModel(t *testing.T) (*Model, *app.Connection) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "shop", Name: "orders", Kind: db.RelationTable},
		{Schema: "shop", Name: "customers", Kind: db.RelationTable},
	}
	connection.Open(app.Overlay{Kind: app.OverlayMessage, Title: " diagram "})
	model.readDiagramAnswer(diagramMsg{
		ConnectionID: model.ActiveID(), Title: "shop.orders",
		Root: present.DiagramTable{
			Schema: "shop", Name: "orders",
			Columns: []present.DiagramColumn{
				{Name: "id", Type: "int", Primary: true},
				{Name: "customer_id", Type: "int", Foreign: true},
			},
			ForeignKeys: []query.ForeignKey{{
				Columns: []string{"customer_id"}, TargetSchema: "shop",
				TargetTable: "customers", TargetColumns: []string{"id"},
			}},
		},
		Related: []present.DiagramTable{{
			Schema: "shop", Name: "customers",
			Columns: []present.DiagramColumn{{Name: "id", Type: "int", Primary: true}},
		}},
	})
	if connection.Overlay.Kind != app.OverlayDiagram {
		t.Fatalf("the card is %q, wanted the diagram", connection.Overlay.Kind)
	}
	return model, connection
}

// The key row of the diagram has one key that pans, and the keys that focus and open a table.
func TestDiagramKeyRowPansAndOpens(t *testing.T) {
	model, _ := buildDiagramModel(t)

	frame := stripEscapes(model.render())
	for _, wanted := range []string{"↑↓←→ pan", "next table", "↵ open"} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the key row has no %q", wanted)
		}
	}
	if strings.Contains(frame, "scroll") {
		t.Error("the key row still has the word scroll")
	}
}

// Tab moves the focus from the table of the diagram to the next table, and Enter opens the
// focused table in a tab of its own.
func TestDiagramTabFocusesTheNextTableAndEnterOpensIt(t *testing.T) {
	model, connection := buildDiagramModel(t)
	model.render()

	focused := connection.Overlay.Diagram.Boxes[connection.Overlay.Field]
	if focused.Name != "orders" {
		t.Fatalf("the diagram opens on %q, wanted orders", focused.Name)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyTab})
	focused = connection.Overlay.Diagram.Boxes[connection.Overlay.Field]
	if focused.Name != "customers" {
		t.Fatalf("Tab moved the focus to %q, wanted customers", focused.Name)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if connection.Overlay.IsOpen() {
		t.Fatalf("the diagram is still open as %q", connection.Overlay.Kind)
	}
	if table := connection.Active().Table; table.Name != "customers" {
		t.Errorf("Enter opened %q, wanted customers", table.Name)
	}
}

// The border of the focused box is drawn in the accent.
func TestDiagramDrawsTheFocusedBorderInTheAccent(t *testing.T) {
	model, connection := buildDiagramModel(t)
	overlay := connection.Overlay
	box := overlay.Diagram.Boxes[overlay.Field]

	theme := model.styles.Theme
	border := "╭" + strings.Repeat("─", box.Width-2) + "╮"

	row := model.paintDiagramRow(overlay, box.Y, 200)
	if !strings.HasPrefix(row, resolveOpening(theme.Accent, theme.Panel)+border) {
		t.Errorf("the top border of the focused box is not in the accent: %q", row)
	}
	overlay.Field = wrap(overlay.Field+1, len(overlay.Diagram.Boxes))
	row = model.paintDiagramRow(overlay, box.Y, 200)
	if !strings.HasPrefix(row, resolveOpening(theme.Text, theme.Panel)+border) {
		t.Errorf("a box that lost the focus keeps the accent border: %q", row)
	}
}
