package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// stageCellEdits stages an edit on the first cell of that many rows.
func stageCellEdits(tab *app.Tab, rows int) {
	for at := range rows {
		row := at
		tab.StageChange(func(pending *core.PendingChanges) {
			pending.Edits[core.BuildEditKey(row, 0)] = core.CellEdit{
				RowIndex: row, ColumnIndex: 0,
				Value: core.CellValue{Kind: core.CellText, Text: "typed"},
			}
		})
	}
}

// succeedRead puts a result of a few rows into the tab, as a read of the relation would.
func succeedRead(tab *app.Tab) {
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Display: "select * from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"},
				{Name: "customer", DataType: "text"},
			},
			Rows: [][]any{{int64(1), "ada"}, {int64(2), "grace"}, {int64(3), "hedy"}},
		})
}

// buildTableTabModel answers a model whose tab is bound to a relation, which is the tab a
// reader edits cells in.
func buildTableTabModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	table := db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}
	connection.Catalog.Tables = []db.TableRef{table}
	tab := connection.OpenTable(table, "select * from public.orders")
	tab.Results.Start([]string{"select * from public.orders"}, 200)
	succeedRead(tab)
	return model, connection, tab
}

// A staged change names a row by its place in the result. A run puts other rows in those
// places, so the change goes with them and the reader is told. Silence reads as the client
// losing what they typed.
func TestRunningAStatementReportsTheChangesItDropped(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Editor = app.NewEditorBuffer("select * from orders", 0)
	tab.Results.Start([]string{"select * from orders"}, 200)
	succeedRead(tab)
	stageCellEdits(tab, 2)

	model.execute(connection, tab, []string{"select * from orders"})

	if core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the run left %d changes staged", core.CountChanges(tab.Pending))
	}
	if connection.Notice == nil {
		t.Fatal("the run dropped the staged changes and reported nothing")
	}
	if !strings.Contains(connection.Notice.Text, "2 staged changes were dropped") {
		t.Errorf("the run reported %q", connection.Notice.Text)
	}
}

// A read of the relation replaces the rows just as a statement does, so it drops the staged
// work too. A change that outlived the read would name a row the reader never chose: the
// changes are applied through the place of the row in the result, so the rows that came in
// their place would be written instead.
func TestReadingTheRelationAgainReportsTheChangesItDropped(t *testing.T) {
	model, connection, tab := buildTableTabModel(t)
	stageCellEdits(tab, 3)

	model.runTabRead(connection, tab)

	if core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the read left %d changes staged, which name rows it replaced",
			core.CountChanges(tab.Pending))
	}
	if connection.Notice == nil {
		t.Fatal("the read dropped the staged changes and reported nothing")
	}
	if !strings.Contains(connection.Notice.Text, "3 staged changes were dropped") {
		t.Errorf("the read reported %q", connection.Notice.Text)
	}
}

// answerQuestion presses the key that answers the open question.
func answerQuestion(model *Model, yes bool) tea.Cmd {
	key := tea.KeyPressMsg{Code: 'n', Text: "n"}
	if yes {
		key = tea.KeyPressMsg{Code: 'y', Text: "y"}
	}
	_, command := model.Update(key)
	return command
}

// Sorting a relation reads it again in another order, so the same rows stand in other places.
// Work staged before the sort would be written to whichever row landed in its place.
func TestSortingARelationDropsTheStagedChangesAfterAYes(t *testing.T) {
	model, connection, tab := buildTableTabModel(t)
	stageCellEdits(tab, 1)

	shape := model.buildGridShape(connection, tab)
	model.sortByColumn(connection, tab, shape, false)
	if connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the sort did not ask; the card is %q", connection.Overlay.Kind)
	}
	if overlay := connection.Overlay; overlay.Body != "Sorting discards 1 staged change." ||
		overlay.Yes != "discard and sort" || overlay.No != "keep editing" {
		t.Errorf("the question reads %q with %q and %q", overlay.Body, overlay.Yes, overlay.No)
	}
	if frame := stripEscapes(model.render()); !strings.Contains(frame, "discard and sort") ||
		!strings.Contains(frame, "keep editing") {
		t.Error("the card does not draw the answers of the sort")
	}
	answerQuestion(model, true)

	if core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the sort left %d changes staged, which name places the rows have left",
			core.CountChanges(tab.Pending))
	}
	if len(tab.Sort) != 1 {
		t.Errorf("the sort was not applied after the yes: %v", tab.Sort)
	}
	if connection.Notice == nil ||
		!strings.Contains(connection.Notice.Text, "1 staged change was dropped") {
		t.Error("the sort dropped the staged change and did not report it")
	}
}

// Every rerun of a grid key and every run key asks before it discards staged changes. A no
// keeps the changes, the rows and the rewrite as they were.
func TestARerunWithStagedChangesKeepsThemOnANo(t *testing.T) {
	cases := []struct {
		name string
		run  func(model *Model, connection *app.Connection, tab *app.Tab)
	}{
		{"sort", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runGridAction(connection, tab, Match{Action: ActionSortColumn})
		}},
		{"filter by the cell", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runGridAction(connection, tab, Match{Action: ActionFilterByCell})
		}},
		{"exclude the cell", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runGridAction(connection, tab, Match{Action: ActionExcludeCell})
		}},
		{"remove the last filter", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runGridAction(connection, tab, Match{Action: ActionPopFilter})
		}},
		{"clear the rewrites", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runGridAction(connection, tab, Match{Action: ActionClearRewrites})
		}},
		{"run the statement", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runStatementAtCursor(connection, tab)
		}},
		{"run the buffer", func(model *Model, connection *app.Connection, tab *app.Tab) {
			model.runWholeBuffer(connection, tab)
		}},
	}
	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildTableTabModel(t)
			tab.Filter = []core.FilterStep{{Kind: core.FilterRaw, Text: "id > 1"}}
			stageCellEdits(tab, 2)
			filter := len(tab.Filter)

			held.run(model, connection, tab)
			if connection.Overlay.Kind != app.OverlayConfirm {
				t.Fatalf("%s did not ask; the card is %q", held.name, connection.Overlay.Kind)
			}
			if command := answerQuestion(model, false); command != nil {
				t.Error("the no started a run")
			}

			if left := core.CountChanges(tab.Pending); left != 2 {
				t.Errorf("%d changes staged after the no, wanted 2", left)
			}
			if tab.Results.IsRunning() {
				t.Error("the no started a run")
			}
			if len(tab.Sort) != 0 || len(tab.Filter) != filter {
				t.Errorf("the no changed the rewrite: sort %v, filter %v", tab.Sort, tab.Filter)
			}
		})
	}
}

// The where prompt asks before it discards staged changes, and a no keeps the old filter.
func TestTheWherePromptAsksBeforeItDiscardsStagedChanges(t *testing.T) {
	model, connection, tab := buildTableTabModel(t)
	stageCellEdits(tab, 1)

	model.runGridAction(connection, tab, Match{Action: ActionFilterWhere})
	connection.Overlay.Draft = app.NewEditorBuffer("id > 2", len("id > 2"))
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the where prompt did not ask; the card is %q", connection.Overlay.Kind)
	}
	answerQuestion(model, false)

	if len(tab.Filter) != 0 {
		t.Errorf("the no applied the filter: %v", tab.Filter)
	}
	if core.CountChanges(tab.Pending) != 1 {
		t.Error("the no discarded the staged change")
	}
}

// A run with nothing staged says nothing about changes, or every run would report on work
// that was never there.
func TestARunWithNothingStagedReportsNoDroppedChanges(t *testing.T) {
	for _, held := range []struct {
		name string
		run  func(model *Model, connection *app.Connection, tab *app.Tab)
	}{
		{
			name: "a statement of the reader",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.execute(connection, tab, []string{"select * from orders"})
			},
		},
		{
			name: "a read of the relation",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.runTabRead(connection, tab)
			},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildTableTabModel(t)
			connection.Notice = nil
			held.run(model, connection, tab)
			if connection.Notice != nil &&
				strings.Contains(connection.Notice.Text, "dropped") {
				t.Errorf("%s reported %q with nothing staged",
					held.name, connection.Notice.Text)
			}
		})
	}
}

// Every path that replaces a result comes through one place, so none of them can keep staged
// work that names the rows it replaced.
func TestNoResultIsReplacedWithStagedWorkLeftOnIt(t *testing.T) {
	cases := []struct {
		name string
		run  func(model *Model, connection *app.Connection, tab *app.Tab)
	}{
		{
			name: "the reader runs a statement",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.execute(connection, tab, []string{"select * from orders"})
			},
		},
		{
			name: "the relation is read again",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.runTabRead(connection, tab)
			},
		},
		{
			name: "a column is sorted",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				shape := model.buildGridShape(connection, tab)
				model.sortByColumn(connection, tab, shape, false)
				answerQuestion(model, true)
			},
		},
		{
			name: "a rewrite is cleared",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				tab.Filter = []core.FilterStep{{Kind: core.FilterRaw, Text: "id > 1"}}
				model.runGridAction(connection, tab, Match{Action: ActionClearRewrites})
				answerQuestion(model, true)
			},
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildTableTabModel(t)
			stageCellEdits(tab, 2)
			held.run(model, connection, tab)
			if left := core.CountChanges(tab.Pending); left != 0 {
				t.Errorf("%d changes outlived the result they name after %s",
					left, held.name)
			}
		})
	}
}
