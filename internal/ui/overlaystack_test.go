package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/notebook"
)

// pressEscape returns the press that closes a card.
func pressEscape() tea.Key { return tea.Key{Code: tea.KeyEscape} }

// A card raised from another card closes alone, and the key that closed it closes the one
// under it. Closing both at once loses the place the reader was in.
func TestACardRaisedFromAnotherClosesAlone(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0),
	}

	model.readKey(tea.Key{Code: 'o', Mod: uv.ModCtrl})
	if connection.Overlay.Kind != app.OverlayAiChats {
		t.Fatalf("the list of conversations did not open: %q", connection.Overlay.Kind)
	}
	if !connection.CoversAnotherOverlay() {
		t.Error("the list covers nothing")
	}

	model.readKey(pressEscape())
	if connection.Overlay.Kind != app.OverlayAiChat {
		t.Errorf("Esc left %q, wanted the chat", connection.Overlay.Kind)
	}
	model.readKey(pressEscape())
	if connection.Overlay.IsOpen() {
		t.Errorf("the second Esc left %q", connection.Overlay.Kind)
	}
}

// A card opened over nothing closes to the workspace.
func TestACardOverNothingClosesToTheWorkspace(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.Open(app.Overlay{Kind: app.OverlayHelp})

	if connection.CoversAnotherOverlay() {
		t.Error("a card over nothing covers something")
	}
	model.readKey(pressEscape())
	if connection.Overlay.IsOpen() {
		t.Errorf("Esc left %q", connection.Overlay.Kind)
	}
}

// Opening a card of the same kind twice stacks nothing, so one Esc still closes it.
func TestTheSameCardOpenedTwiceStacksNothing(t *testing.T) {
	connection := buildOfflineModel(t, 120, 40).Active()
	connection.Open(app.Overlay{Kind: app.OverlayHelp})
	connection.Open(app.Overlay{Kind: app.OverlayHelp})

	if connection.CoversAnotherOverlay() {
		t.Error("the card covers one of its own kind")
	}
	connection.CloseOverlay()
	if connection.Overlay.IsOpen() {
		t.Errorf("the card is still open: %q", connection.Overlay.Kind)
	}
}

// Closing every card at once leaves nothing behind, so a flow that ends returns to the
// workspace rather than to a card it covered.
func TestClosingEveryCardLeavesNothing(t *testing.T) {
	connection := buildOfflineModel(t, 120, 40).Active()
	connection.Open(app.Overlay{Kind: app.OverlayNotebooks})
	connection.Open(app.Overlay{Kind: app.OverlayConfirm})

	connection.CloseEveryOverlay()
	if connection.Overlay.IsOpen() || connection.CoversAnotherOverlay() {
		t.Errorf("a card is still open: %q", connection.Overlay.Kind)
	}
}

// The question about a notebook is asked over the list, and refusing it returns there.
func TestRefusingTheNotebookQuestionReturnsToTheList(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayNotebooks, Draft: app.NewEditorBuffer("", 0),
		Notebooks: []notebook.Entry{{Name: "revenue", Path: "/tmp/revenue.masume.md"}},
	}

	model.readKey(tea.Key{Code: 'd', Text: "d"})
	if connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the question did not open: %q", connection.Overlay.Kind)
	}
	model.readKey(pressEscape())
	if connection.Overlay.Kind != app.OverlayNotebooks {
		t.Errorf("Esc left %q, wanted the list", connection.Overlay.Kind)
	}
}

// Answering no to the question about a notebook returns to the list as well, so the two ways
// of refusing it land in the same place.
func TestAnsweringNoToTheNotebookQuestionReturnsToTheList(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayNotebooks, Draft: app.NewEditorBuffer("", 0),
		Notebooks: []notebook.Entry{{Name: "revenue", Path: "/tmp/revenue.masume.md"}},
	}

	model.readKey(tea.Key{Code: 'd', Text: "d"})
	if connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the question did not open: %q", connection.Overlay.Kind)
	}
	model.readKey(tea.Key{Code: 'n', Text: "n"})
	if connection.Overlay.Kind != app.OverlayNotebooks {
		t.Errorf("no left %q, wanted the list", connection.Overlay.Kind)
	}
}

// The question about a session is asked over the card of the server activity, and both ways
// of refusing it return there.
func TestTheStopQuestionReturnsToTheActivityCard(t *testing.T) {
	for _, held := range []struct {
		name string
		key  tea.Key
	}{
		{"escape", pressEscape()},
		{"no", tea.Key{Code: 'n', Text: "n"}},
	} {
		t.Run(held.name, func(t *testing.T) {
			model := buildOfflineModel(t, 120, 40)
			connection := model.Active()
			connection.Overlay = app.Overlay{
				Kind: app.OverlayActivity,
				Sessions: []db.Activity{{
					PID: 4417, User: "writer", State: "active", Query: "select 1",
				}},
			}

			model.readKey(tea.Key{Code: 'x', Text: "x"})
			if connection.Overlay.Kind != app.OverlayConfirm {
				t.Fatalf("the question did not open: %q", connection.Overlay.Kind)
			}
			model.readKey(held.key)
			if connection.Overlay.Kind != app.OverlayActivity {
				t.Errorf("%s left %q, wanted the activity card", held.name,
					connection.Overlay.Kind)
			}
		})
	}
}

// A card that begins another piece of work takes its place, so the one it replaced does not
// come back when the new one closes.
func TestACardThatBeginsAnotherTakesItsPlace(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.OpenOver(app.Overlay{Kind: app.OverlayNotebooks})
	connection.Open(app.Overlay{Kind: app.OverlayHelp})

	if connection.CoversAnotherOverlay() {
		t.Error("the card that took the place covers the one it replaced")
	}
	model.readKey(pressEscape())
	if connection.Overlay.IsOpen() {
		t.Errorf("Esc left %q", connection.Overlay.Kind)
	}
}

// The field that renames a notebook is opened over the list, and answering it returns there,
// so the reader sees the renamed file among the others.
func TestRenamingANotebookReturnsToTheList(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "revenue.masume.md")
	if err := os.WriteFile(path, []byte("# revenue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.Open(app.Overlay{
		Kind: app.OverlayNotebooks, Draft: app.NewEditorBuffer("", 0),
		Notebooks: []notebook.Entry{{Name: "revenue", Path: path}},
	})
	model.render()

	model.readKey(tea.Key{Code: 'e', Text: "e"})
	if connection.Overlay.Kind != app.OverlayPrompt {
		t.Fatalf("the field did not open: %q", connection.Overlay.Kind)
	}
	connection.Overlay.Draft = app.NewEditorBuffer("earnings", len("earnings"))
	model.readKey(tea.Key{Code: tea.KeyEnter})

	if connection.Overlay.Kind != app.OverlayNotebooks {
		t.Errorf("the rename left %q, wanted the list", connection.Overlay.Kind)
	}
}
