package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

func TestTheHelpDrawsTheKeysInTheMutedInk(t *testing.T) {
	for _, term := range []string{"", "query tab"} {
		model := buildOfflineModel(t, 120, 34)
		connection := model.Active()
		connection.Overlay = app.Overlay{
			Kind: app.OverlayHelp, Draft: app.NewEditorBuffer(term, len(term)),
		}
		chord := model.registry.FormatFirstActionChordName(cfg.ScopeGlobal, ActionNewQueryTab)
		muted := describeInk(model.styles.Theme.Muted)
		accent := describeInk(model.styles.Theme.Accent)

		found := false
		for _, line := range strings.Split(model.render(), "\n") {
			if !strings.Contains(stripEscapes(line), "new query tab") {
				continue
			}
			found = true
			keys := strings.Builder{}
			for _, cell := range mapCells(line) {
				if strings.TrimSpace(cell.text) == "" || cell.text == "│" {
					continue
				}
				if strings.Contains(cell.sgr, accent) {
					t.Errorf("term %q: the help row draws %q in the accent ink", term, cell.text)
				}
				if strings.Contains(cell.sgr, muted) {
					keys.WriteString(cell.text)
				}
			}
			if !strings.HasPrefix(keys.String(), strings.ReplaceAll(chord, " ", "")) {
				t.Errorf("term %q: the muted cells read %q, wanted the key %q first",
					term, keys.String(), chord)
			}
		}
		if !found {
			t.Errorf("term %q: the help has no row for a new query tab", term)
		}
	}
}

func TestTheHelpOpensAtTheSectionOfTheFocusedPane(t *testing.T) {
	for _, test := range []struct {
		focus app.Pane
		title string
	}{
		{app.PaneSidebar, "tree"},
		{app.PaneEditor, "writing a statement"},
		{app.PaneResult, "grid"},
	} {
		model := buildOfflineModel(t, 120, 34)
		connection := model.Active()
		tab := connection.Active()
		tab.Focus = test.focus
		model.runGlobalAction(connection, tab, Match{Scope: cfg.ScopeGlobal, Action: ActionShowHelp})

		lines := strings.Split(stripEscapes(model.render()), "\n")
		first := ""
		for at, line := range lines {
			if strings.Contains(line, helpPlaceholder) && at+1 < len(lines) {
				first = lines[at+1]
			}
		}
		if !strings.Contains(first, "│ "+test.title+" ") {
			t.Errorf("focus %s: the first help row reads %q, wanted the section %q",
				test.focus, first, test.title)
		}
	}
}

func TestTheHelpSizesTheKeyColumnToTheWidestKey(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()
	connection.Overlay = app.Overlay{Kind: app.OverlayHelp, Draft: app.NewEditorBuffer("", 0)}
	found := false
	for _, line := range strings.Split(stripEscapes(model.render()), "\n") {
		if !strings.Contains(line, "new query tab") {
			continue
		}
		found = true
		gap := strings.Index(line, "new query tab") - strings.Index(line, "Alt+N")
		if gap != len("Alt+Shift+W")+helpColumnGap {
			t.Errorf("the label stands %d cells after the key in %q, wanted %d",
				gap, line, len("Alt+Shift+W")+helpColumnGap)
		}
	}
	if !found {
		t.Fatal("the help has no row for a new query tab")
	}

	connection.Overlay = app.Overlay{Kind: app.OverlayHelp, Draft: app.NewEditorBuffer("run", 3)}
	written := stripEscapes(model.render())
	for _, text := range []string{"run the selection or the statement", "dialogs and forms"} {
		if !strings.Contains(written, text) {
			t.Errorf("the search results cut %q:\n%s", text, written)
		}
	}
}
