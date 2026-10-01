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
	for _, wanted := range []string{"next table", "↵ open"} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the key row has no %q", wanted)
		}
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
	if !strings.HasPrefix(row, resolveOpening(theme.Faint, theme.Panel)+border) {
		t.Errorf("a box that lost the focus keeps the accent border: %q", row)
	}
}

// Shift+Tab moves the focus to the previous table, from the first table to the last.
func TestDiagramShiftTabFocusesThePreviousTable(t *testing.T) {
	model, connection := buildDiagramModel(t)
	model.render()

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	focused := connection.Overlay.Diagram.Boxes[connection.Overlay.Field]
	if focused.Name != "customers" {
		t.Errorf("Shift+Tab moved the focus to %q, wanted customers", focused.Name)
	}
}

// g draws the diagram of the focused table over the one on show, and Esc goes back to it.
func TestDiagramFollowsATableAndGoesBack(t *testing.T) {
	model, connection := buildDiagramModel(t)
	model.render()
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyTab})

	if command := pressKey(t, model, tea.KeyPressMsg{Code: 'g', Text: "g"}); command == nil {
		t.Fatal("g read nothing")
	}
	if connection.Overlay.Following != "shop.customers" {
		t.Fatalf("the diagram follows %q, wanted shop.customers", connection.Overlay.Following)
	}
	model.readDiagramAnswer(diagramMsg{
		ConnectionID: model.ActiveID(), Title: "shop.customers",
		Root: present.DiagramTable{
			Schema: "shop", Name: "customers",
			Columns: []present.DiagramColumn{{Name: "id", Type: "int", Primary: true}},
		},
	})
	if connection.Overlay.Title != " diagram · shop.customers " {
		t.Fatalf("the card is %q after the reads", connection.Overlay.Title)
	}
	if frame := stripEscapes(model.render()); !strings.Contains(frame, "Esc back") {
		t.Errorf("the key row has no Esc back:\n%s", frame)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if connection.Overlay.Title != " diagram · shop.orders " {
		t.Fatalf("Esc went back to %q, wanted shop.orders", connection.Overlay.Title)
	}
	if focused := connection.Overlay.Diagram.Boxes[connection.Overlay.Field]; focused.Name != "customers" {
		t.Errorf("the focus came back on %q, wanted customers", focused.Name)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if connection.Overlay.IsOpen() {
		t.Errorf("the second Esc left the card %q open", connection.Overlay.Kind)
	}
}

// y copies the diagram as text, with no blanks at the end of a line.
func TestDiagramCopiesItsLines(t *testing.T) {
	model, connection := buildDiagramModel(t)
	holdClipboard(t, "", nil)
	model.render()

	pressKey(t, model, tea.KeyPressMsg{Code: 'y', Text: "y"})
	lines := strings.Split(strings.TrimSuffix(model.clipboard, "\n"), "\n")
	if len(lines) != len(connection.Overlay.Diagram.Lines) {
		t.Fatalf("the clipboard holds %d lines, wanted %d", len(lines),
			len(connection.Overlay.Diagram.Lines))
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("the line %q ends in a blank", line)
		}
	}
	if connection.Overlay.Notice != "copied" {
		t.Errorf("the card reads %q, wanted copied", connection.Overlay.Notice)
	}
}

// A click focuses the box under the pointer, and a second click opens its table.
func TestDiagramClickFocusesAndDoubleClickOpens(t *testing.T) {
	model, connection := buildDiagramModel(t)
	model.render()

	box := connection.Overlay.Diagram.Boxes[1]
	click := tea.MouseClickMsg{
		X: model.layout.cardBodyLeft + box.X + 2, Y: model.layout.cardBodyTop + box.Y + 1,
		Button: tea.MouseLeft,
	}
	model.readMouse(click)
	if connection.Overlay.Field != 1 {
		t.Fatalf("the click focused box %d, wanted 1", connection.Overlay.Field)
	}
	model.readMouse(click)
	if connection.Overlay.IsOpen() {
		t.Fatalf("the double click left the card %q open", connection.Overlay.Kind)
	}
	if table := connection.Active().Table; table.Name != "customers" {
		t.Errorf("the double click opened %q, wanted customers", table.Name)
	}
}

// A pan moves the diagram sideways only, and stops at the last column.
func TestDiagramPanKeepsTheRows(t *testing.T) {
	model := buildOfflineModel(t, 90, 24)
	connection := model.Active()
	columns := []present.DiagramColumn{{Name: "id", Type: "int", Primary: true}}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} {
		columns = append(columns, present.DiagramColumn{
			Name: "column_" + name, Type: "character varying(200)",
		})
	}
	related := []present.DiagramTable{}
	for _, name := range []string{"alpha", "beta"} {
		related = append(related, present.DiagramTable{
			Schema: "shop", Name: name,
			Columns: []present.DiagramColumn{
				{Name: "id", Type: "int", Primary: true},
				{Name: "root_id", Type: "int", Foreign: true},
			},
			ForeignKeys: []query.ForeignKey{{
				Columns: []string{"root_id"}, TargetSchema: "shop", TargetTable: "root",
				TargetColumns: []string{"id"},
			}},
		})
	}
	connection.Open(app.Overlay{Kind: app.OverlayMessage, Title: " diagram "})
	model.readDiagramAnswer(diagramMsg{
		ConnectionID: model.ActiveID(), Title: "shop.root",
		Root:    present.DiagramTable{Schema: "shop", Name: "root", Columns: columns},
		Related: related,
	})
	model.render()
	if len(connection.Overlay.Diagram.Lines) <= model.layout.cardBody {
		t.Fatalf("the diagram of %d rows fits the card", len(connection.Overlay.Diagram.Lines))
	}

	for range 20 {
		pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	}
	overlay := connection.Overlay
	if overlay.List.Cursor != 0 {
		t.Errorf("the pan scrolled the diagram to row %d", overlay.List.Cursor)
	}
	if last := overlay.Diagram.Width - model.layout.cardRoom; overlay.List.Offset != last {
		t.Errorf("the pan stopped at column %d, wanted %d", overlay.List.Offset, last)
	}
	if frame := stripEscapes(model.render()); !strings.Contains(frame, "shop.alpha") {
		t.Errorf("the pan hid the first row:\n%s", frame)
	}

	for range 100 {
		pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if last := len(overlay.Diagram.Lines) - model.layout.cardBody; connection.Overlay.List.Cursor != last {
		t.Errorf("the diagram scrolled to row %d, wanted %d", connection.Overlay.List.Cursor, last)
	}
}

// A table with no foreign key says so on the key row.
func TestDiagramOfATableWithNoKeySaysSo(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()
	connection.Open(app.Overlay{Kind: app.OverlayMessage, Title: " diagram "})
	model.readDiagramAnswer(diagramMsg{
		ConnectionID: model.ActiveID(), Title: "shop.notes",
		Root: present.DiagramTable{
			Schema: "shop", Name: "notes",
			Columns: []present.DiagramColumn{{Name: "id", Type: "int", Primary: true}},
		},
	})
	if frame := stripEscapes(model.render()); !strings.Contains(frame, "no foreign keys") {
		t.Errorf("the card has no readout:\n%s", frame)
	}
}
