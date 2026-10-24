package ui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/hist"
)

func TestHistoryKeepsTheParameterMarksEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	store, err := hist.Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatalf("the history file did not open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	held.model.log = store

	written := "select id from orders where total_cents > :cents and status = :status"
	held.connection.Active().Editor.SetText(written)
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if held.connection.Overlay.Kind != app.OverlayParameters {
		t.Fatalf("the run asked for no value: %q", held.connection.Overlay.Kind)
	}
	held.connection.Overlay.Draft = app.NewEditorBuffer(`{"cents": 1000, "status": "paid"}`, 0)
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})

	entries, err := store.ListRecent("shop", 10)
	if err != nil {
		t.Fatalf("the history did not read: %v", err)
	}
	if len(entries) != 1 || entries[0].SQL != written || entries[0].ErrorMessage != "" {
		t.Fatalf("the history has %+v, wanted %q", entries, written)
	}
}
