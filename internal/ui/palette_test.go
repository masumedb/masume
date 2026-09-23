package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/notebook"
)

// Every row the palette offers has to run something. A row whose id names no action is
// drawn like any other, and reports that there is no such action only once the user has
// chosen it, so the fault is found by the person using the client and not by the suite.
func TestEveryPaletteEntryRunsSomething(t *testing.T) {
	// The ids the palette handles itself, before it looks for an action of that name.
	handled := map[string]bool{
		"copy-plan": true, "reload-themes": true,
		"ai-explain-query": true, "ai-optimize-query": true,
		"ai-build-notebook":  true,
		configProblemsAction: true,
		settingsAction:       true,
	}
	// The rows that move the result pane to one of its views, which the palette resolves
	// against the views themselves and not against an action.
	for _, view := range paletteViews {
		handled["tab-"+string(view)] = true
	}

	for _, entry := range paletteEntries {
		if handled[entry.id] {
			continue
		}
		if _, known := FindActionID(entry.id); !known {
			t.Errorf("the palette row %q names no action", entry.id)
		}
	}
}

// A row that names an action has to name its own. An id that names one action while the row
// runs another would run the wrong thing, which is worse than running nothing.
func TestEveryPaletteEntryNamesItsOwnAction(t *testing.T) {
	for _, entry := range paletteEntries {
		if entry.action == "" {
			continue
		}
		if entry.id != string(entry.action) {
			t.Errorf("the palette row %q runs %q; the id and the action have to be the same",
				entry.id, entry.action)
		}
	}
}

// The palette offers no view of a result that was never run. A row that is drawn and then
// does nothing reads as a client that is broken.
func TestThePaletteOffersNoViewBeforeAQueryIsRun(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()

	for _, row := range model.buildPaletteActions(connection) {
		if strings.HasPrefix(row.Label, "View: ") {
			t.Errorf("the palette offers %q, and nothing has run", row.Label)
		}
	}

	// A run that returned rows offers the views of those rows, and none of a table the
	// tab was not opened on.
	tab := connection.Active()
	tab.Results.Start([]string{"select 1"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id", DataType: "int4"}},
		Rows:    [][]any{{int64(1)}},
	})

	offered := map[string]bool{}
	for _, row := range model.buildPaletteActions(connection) {
		offered[row.ID] = true
	}
	for _, view := range tab.Views(connection.Session) {
		if !offered["tab-"+string(view)] {
			t.Errorf("the palette offers no row for the %s view", view)
		}
	}
	if offered["tab-"+string(app.ViewIndexes)] {
		t.Error("the palette offers the indexes of a tab that opened no table")
	}
}

// listPaletteRows returns the rows the palette offers now.
func listPaletteRows(model *Model) map[string]bool {
	offered := map[string]bool{}
	for _, row := range model.buildPaletteActions(model.Active()) {
		offered[row.ID] = true
	}
	return offered
}

// A row the state cannot run is left out, and not offered and refused.
func TestThePaletteLeavesOutWhatTheStateCannotRun(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	offered := listPaletteRows(model)

	for _, id := range []string{
		"run-at-cursor", "run-batch", "explain", "explain-analyze", "save-query",
		"format-sql", "reveal-sql", "ai-explain-query", "ai-optimize-query",
		"cancel-query", "copy-plan", "undo-write", "reopen-tab", "next-tab",
		"export-csv", "export-json", "copy-csv", "copy-json", "copy-markdown",
		"copy-inserts", "count-rows", "next-page",
		"undo-change", "redo-change", "review-changes", "discard-changes",
		"run-cell", "add-cell-below", "write-notebook-report", "chat-to-notebook",
		"ai-fix-error",
	} {
		if offered[id] {
			t.Errorf("the palette offers %q on a tab that holds nothing", id)
		}
	}
	// Every row that stands on no state is still offered.
	for _, id := range []string{
		"show-history", "show-saved", "new-query-tab", "open-picker", settingsAction,
	} {
		if !offered[id] {
			t.Errorf("the palette does not offer %q", id)
		}
	}
}

// Each row appears with the state it needs, and goes with it.
func TestThePaletteFollowsTheStateOfTheTab(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()
	tab := connection.Active()

	wants := func(step string, ids ...string) {
		t.Helper()
		offered := listPaletteRows(model)
		for _, id := range ids {
			if !offered[id] {
				t.Errorf("%s: the palette does not offer %q", step, id)
			}
		}
	}
	hides := func(step string, ids ...string) {
		t.Helper()
		offered := listPaletteRows(model)
		for _, id := range ids {
			if offered[id] {
				t.Errorf("%s: the palette still offers %q", step, id)
			}
		}
	}

	tab.Editor.SetText("select id from orders")
	wants("a statement in the editor", "run-at-cursor", "run-batch", "save-query",
		"format-sql", "reveal-sql", "ai-explain-query", "ai-optimize-query")

	tab.Results.Start([]string{"select id from orders"}, 1)
	wants("a statement that ran", "tab-data", "tab-fields")
	tab.Results.Succeed(0, db.ComposedRead{Pageable: true}, db.QueryResult{
		Columns:   []db.ResultColumn{{Name: "id", DataType: "int4"}},
		Rows:      [][]any{{int64(1)}},
		Truncated: true,
	})
	wants("a result", "export-csv", "export-json", "copy-csv", "copy-json",
		"copy-markdown", "copy-inserts", "count-rows", "next-page")

	stageCellEdits(tab, 1)
	wants("a staged change", "review-changes", "discard-changes", "undo-change")
	hides("a staged change", "redo-change")

	tab.UndoChange()
	wants("an undone change", "redo-change")
	hides("an undone change", "review-changes", "discard-changes", "undo-change")

	connection.OpenQueryTab("select 1")
	wants("a second tab", "next-tab")
	hides("a second tab", "export-csv", "count-rows", "tab-data")

	connection.CloseTab(connection.ActiveIndex)
	wants("a closed tab", "reopen-tab")

	connection.OpenNotebookInNewTab(
		notebook.Parse("```sql id=one\nselect 1\n```\n"), "", notebook.OriginPersonal)
	wants("a notebook tab", "run-cell", "run-from-cell", "run-marked-cells",
		"add-cell-below", "set-cell-kind", "edit-cell-source",
		"write-notebook-report", "notebook-run-policy")
	hides("a notebook tab", "format-sql", "ai-explain-query", "ai-optimize-query")
}

func TestThePaletteRowLeadsWithTheNameAndEndsWithTheKey(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPalette, Draft: app.NewEditorBuffer("", 0),
		Palette: model.buildPaletteActions(connection),
	}
	frame := strings.Split(model.render(), "\n")
	if filter := stripEscapes(frame[model.layout.overlayRows.top-1]); !strings.Contains(
		filter, "Search commands…") {
		t.Errorf("the filter line reads %q", strings.TrimSpace(filter))
	}

	first := connection.Overlay.Palette[0]
	block := model.layout.overlayRows
	text := strings.TrimSpace(strings.TrimPrefix(
		strings.TrimSpace(cutRowText(frame[block.top], block.from, block.to)), "❯"))
	text = strings.TrimSpace(strings.TrimSuffix(text, "█"))
	if !strings.HasPrefix(text, first.Label) || !strings.HasSuffix(text, first.Chord) {
		t.Errorf("the first row reads %q, wanted %q first and %q last",
			text, first.Label, first.Chord)
	}
}

func TestThePaletteDrawsTheMatchedTextInBold(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPalette, Draft: app.NewEditorBuffer("every", 5),
		Palette: model.buildPaletteActions(connection),
	}
	frame := strings.Split(model.render(), "\n")

	bold := strings.Builder{}
	for _, cell := range mapCells(frame[model.layout.overlayRows.top]) {
		if strings.Contains(cell.sgr, "\x1b[1;") || strings.Contains(cell.sgr, ";1;") ||
			strings.Contains(cell.sgr, "\x1b[1m") {
			bold.WriteString(cell.text)
		}
	}
	if bold.String() != "every" {
		t.Errorf("the row draws %q in bold, wanted %q", bold.String(), "every")
	}
}

// A palette row with an action and the first help row of one action draw the label of that
// action, and the label is never empty.
func TestThePaletteAndTheHelpDrawTheLabelOfTheAction(t *testing.T) {
	for _, entry := range paletteEntries {
		if entry.action == "" {
			continue
		}
		action, known := FindAction(entry.scope, entry.action)
		if !known || action.Label == "" {
			t.Errorf("the palette row %q runs an action with no label", entry.id)
		}
		if entry.label != "" {
			t.Errorf("the palette row %q has the label %q, and its action has %q",
				entry.id, entry.label, action.Label)
		}
	}

	seen := map[string]bool{}
	for _, section := range HelpSections {
		for _, entry := range section.Entries {
			if len(entry.Actions) != 1 {
				continue
			}
			actionKey := cfg.BuildActionKey(entry.Scope, string(entry.Actions[0]))
			first := !seen[actionKey]
			seen[actionKey] = true
			if !first {
				continue
			}
			action, known := FindAction(entry.Scope, entry.Actions[0])
			if !known || action.Label == "" {
				t.Errorf("the help row of %s has an action with no label", actionKey)
			}
			if entry.Text != "" {
				t.Errorf("the help row of %s has the text %q, and its action has %q",
					actionKey, entry.Text, action.Label)
			}
		}
	}
}

func TestThePaletteDrawsTheLabelInSentenceCase(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()
	connection.Active().Editor.Text = "select 1"

	for _, row := range model.buildPaletteActions(connection) {
		if row.ID == "run-at-cursor" {
			if row.Label != "Run the selection or the statement" {
				t.Errorf("the palette row reads %q", row.Label)
			}
			return
		}
	}
	t.Error("the palette has no run-at-cursor row")
}
