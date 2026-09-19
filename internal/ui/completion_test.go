package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
)

// buildListingModel answers a model with the suggestions standing over this statement.
func buildListingModel(t *testing.T, written string) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer(written, len(written))
	model.refreshCompletion(connection, tab)
	if !tab.Completion.IsListing() {
		t.Skipf("the statement %q offered nothing", written)
	}
	return model, connection, tab
}

// A list that stands over the statement owns the arrows, whether the word under the caret is
// written or empty. The arrows used to move the caret over an empty word, which left the list
// standing where the caret no longer was.
func TestTheArrowsStepTheListWhereverItOpened(t *testing.T) {
	for _, written := range []string{
		"select * from ",
		"select * from ord",
	} {
		t.Run(written, func(t *testing.T) {
			model, _, tab := buildListingModel(t, written)
			caret := tab.Editor.Caret

			model.readWorkspaceKey(tea.Key{Code: tea.KeyDown})
			if tab.Completion.Selected != 1 {
				t.Errorf("Down marked candidate %d", tab.Completion.Selected)
			}
			model.readWorkspaceKey(tea.Key{Code: tea.KeyUp})
			if tab.Completion.Selected != 0 {
				t.Errorf("Up marked candidate %d", tab.Completion.Selected)
			}
			if tab.Editor.Caret != caret {
				t.Errorf("the arrows moved the caret to %d, and it was at %d",
					tab.Editor.Caret, caret)
			}
			if !tab.Completion.IsListing() {
				t.Error("the arrows took the list off the statement")
			}
		})
	}
}

// Tab takes the candidate the arrows marked, so the two work together.
func TestTabTakesTheCandidateTheArrowsMarked(t *testing.T) {
	model, _, tab := buildListingModel(t, "select * from ")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyDown})
	wanted := tab.Completion.Candidates[tab.Completion.Selected].Text

	model.readWorkspaceKey(tea.Key{Code: tea.KeyTab})
	if !strings.Contains(tab.Editor.Text, wanted) {
		t.Errorf("the statement reads %q and does not hold %q", tab.Editor.Text, wanted)
	}
	if tab.Completion.IsListing() {
		t.Error("the list still stands over the statement")
	}
}

// Escape takes the list off the statement, and the arrows move the caret again once it is
// gone.
func TestEscapeGivesTheArrowsBackToTheEditor(t *testing.T) {
	model, _, tab := buildListingModel(t, "select id,\nname from ")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyEscape})
	if tab.Completion.IsListing() {
		t.Fatal("Escape left the list standing")
	}

	caret := tab.Editor.Caret
	model.readWorkspaceKey(tea.Key{Code: tea.KeyUp})
	if tab.Editor.Caret == caret {
		t.Error("the caret stayed where it was once the list was gone")
	}
}

// The list opens as the statement is written, and one key opens it again for the word under
// the caret after it was dismissed.
func TestOneKeyAsksForTheList(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer("select * from ", len("select * from "))
	tab.Completion.Dismiss()

	model.readWorkspaceKey(tea.Key{Code: ' ', Text: " ", Mod: uv.ModCtrl})
	if !tab.Completion.IsListing() {
		t.Error("the key left the list closed")
	}
	if tab.Editor.Text != "select * from " {
		t.Errorf("the statement reads %q", tab.Editor.Text)
	}
}

// The title of the pane names the keys of the list while it stands, and the same ones however
// the list opened.
func TestTheTitleNamesTheKeysOfTheList(t *testing.T) {
	for _, written := range []string{"select * from ", "select * from ord"} {
		model, _, tab := buildListingModel(t, written)
		frame := strings.Split(model.render(), "\n")
		title := stripStyles(frame[firstPaneRow])
		for _, key := range []string{"↑↓", "⇥ accept", "Esc"} {
			if !strings.Contains(title, key) {
				t.Errorf("the title of %q reads %q and does not name %q",
					written, strings.TrimSpace(title), key)
			}
		}
		if strings.Contains(title, "Space") {
			t.Errorf("the title of %q still names a key that asks for the list", written)
		}
		_ = tab
	}
}

// A terminal reports the locks it has on as modifiers of every press. Caps Lock and Num Lock
// change nothing about an arrow, so the list still answers one, and a letter is still typed.
func TestTheLocksOfTheTerminalDoNotTakeTheKeysOfTheList(t *testing.T) {
	for _, lock := range []struct {
		name string
		mod  uv.KeyMod
	}{{"caps lock", uv.ModCapsLock}, {"num lock", uv.ModNumLock}} {
		t.Run(lock.name, func(t *testing.T) {
			model, _, tab := buildListingModel(t, "select * from ")
			caret := tab.Editor.Caret

			model.readWorkspaceKey(tea.Key{Code: tea.KeyDown, Mod: lock.mod})
			if tab.Completion.Selected != 1 {
				t.Errorf("Down with %s marked candidate %d",
					lock.name, tab.Completion.Selected)
			}
			if tab.Editor.Caret != caret {
				t.Errorf("Down with %s moved the caret to %d", lock.name, tab.Editor.Caret)
			}

			model.readWorkspaceKey(tea.Key{Code: 'a', Text: "a", Mod: lock.mod})
			if !strings.HasSuffix(tab.Editor.Text, "a") {
				t.Errorf("a letter with %s wrote %q", lock.name, tab.Editor.Text)
			}
		})
	}
}

// A key that carries Shift grows the selection of the statement, so the list leaves it to the
// editor rather than stepping.
func TestShiftAndAnArrowStillGrowTheSelection(t *testing.T) {
	model, _, tab := buildListingModel(t, "select id,\nname from ")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyUp, Mod: uv.ModShift})
	if tab.Completion.Selected != 0 {
		t.Errorf("Shift and Up stepped the list to %d", tab.Completion.Selected)
	}
	if !tab.Editor.HasSelection() {
		t.Error("Shift and Up selected nothing")
	}
}

// A candidate taken in the middle of a statement leaves no selection behind it. The text after
// the caret was selected, and the next typed character replaced it.
func TestATakenCandidateSelectsNothing(t *testing.T) {
	written := "select 1;\nselect * from ord\nselect 3;\nselect 4;"
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer(written, strings.Index(written, "\nselect 3;"))
	model.refreshCompletion(connection, tab)
	if !tab.Completion.IsListing() {
		t.Skipf("the statement %q offered nothing", written)
	}

	model.acceptCompletion(connection, tab)
	if tab.Editor.HasSelection() {
		t.Errorf("the taken candidate selected %q", tab.Editor.Selection())
	}
	if !strings.HasSuffix(tab.Editor.Text, "\nselect 3;\nselect 4;") {
		t.Errorf("the taken candidate wrote %q", tab.Editor.Text)
	}
}

// The list offers the columns of the statement at the caret. A buffer of several statements
// names other relations, and a column of one of those belongs to no statement being written.
func TestTheListOffersTheColumnsOfTheStatementAtTheCaret(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	written := "select * from public.orders as o;\nselect * from public.customers as c where c."
	tab.Editor = app.NewEditorBuffer(written, len(written))
	model.refreshCompletion(connection, tab)

	sources := model.buildCompletionSources(connection, tab)
	if _, named := sources.ColumnsByQualifier["o"]; named {
		t.Error("the list offers the columns of a statement the caret is not in")
	}
}

// The word the caret stands in the middle of is replaced whole, so no tail of the old word
// is left behind the name that was taken.
func TestATakenCandidateTakesTheRestOfTheWord(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	written := "select * from ordersandmore"
	tab.Editor = app.NewEditorBuffer(written, len("select * from ord"))
	model.refreshCompletion(connection, tab)
	if !tab.Completion.IsListing() {
		t.Skip("the statement offered nothing")
	}

	model.acceptCompletion(connection, tab)
	if strings.Contains(tab.Editor.Text, "andmore") {
		t.Errorf("the statement reads %q", tab.Editor.Text)
	}
}
