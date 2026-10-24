package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

func TestNewRowWithAJSONObjectIsWrittenEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	ctx := context.Background()
	if _, err := held.connection.Session.RunQuery(ctx, "alter table orders add column prefs json", 1, nil); err != nil {
		t.Fatal(err)
	}
	table, known := held.connection.Catalog.FindTable("main", "orders")
	if !known {
		t.Fatal("the catalog has no orders table")
	}
	tab := held.connection.OpenTable(table, "select * from orders")
	tab.Focus = app.PaneResult
	_, command := held.model.runTabRead(held.connection, tab)
	held.pump(t, command)

	held.press(t, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if held.connection.Overlay.Kind != app.OverlayCellEdit {
		t.Fatalf("the new row form did not open: %q", held.connection.Overlay.Kind)
	}
	held.connection.Overlay.Draft = app.NewEditorBuffer(
		`{"total_cents": 5, "status": "Probe", "prefs": {"a": [1, 2]}}`, 0)
	held.press(t, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	held.press(t, tea.KeyPressMsg{Code: 'p', Text: "p"})
	held.press(t, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if held.connection.Overlay.IsOpen() {
		held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})
	}
	if held.connection.Notice != nil && held.connection.Notice.Tone == app.NoticeError {
		t.Fatalf("the apply failed: %s", held.connection.Notice.Text)
	}

	read, err := held.connection.Session.RunQuery(ctx, "select prefs from orders where status = 'Probe'", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Rows) != 1 || core.FormatCell(read.Rows[0][0], "json") != `{"a":[1,2]}` {
		t.Fatalf("the table has %v", read.Rows)
	}
}
