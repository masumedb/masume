package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

// showRows puts a finished read of id and total on the tab.
func showRows(tab *app.Tab, rows [][]any) {
	tab.Results.Start([]string{"select id, total from orders"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select id, total from orders"}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id"}, {Name: "total"}}, Rows: rows,
	})
	tab.View = app.ViewData
}

func TestCompareShowsTheRowsThatDifferFromThePinnedResult(t *testing.T) {
	model := buildOfflineModel(t, 110, 30)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	tab.Target.KeyColumns = []string{"id"}

	showRows(tab, [][]any{{1, 10}, {2, 20}, {3, 30}})
	pressKey(t, model, tea.KeyPressMsg{Code: 'P', Text: "P"})
	showRows(tab, [][]any{{1, 10}, {2, 25}, {4, 40}})
	pressKey(t, model, tea.KeyPressMsg{Code: '=', Text: "="})

	if tab.View != app.ViewCompare {
		t.Fatalf("the tab shows the %s view", tab.View)
	}
	frame := stripEscapes(model.render())
	for _, wanted := range []string{
		"pinned: orders, 3 rows · here: orders, 3 rows · matched by id",
		"+1 added · -1 removed · ~1 changed · 1 same", "20 → 25", "+  4", "-  3",
	} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the frame has no %q:\n%s", wanted, frame)
		}
	}
}

func TestCompareAsksForAPinnedResultFirst(t *testing.T) {
	model := buildOfflineModel(t, 110, 30)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	showRows(tab, [][]any{{1, 10}})
	pressKey(t, model, tea.KeyPressMsg{Code: '=', Text: "="})

	if tab.View != app.ViewData || !strings.Contains(stripEscapes(model.render()), "pin a result first") {
		t.Errorf("the tab shows the %s view", tab.View)
	}
}

// showTable puts a finished read of that table, with those columns, on the tab.
func showTable(tab *app.Tab, columns []string, rows [][]any) {
	tab.Results.Start([]string{"select * from t"}, 100)
	result := db.QueryResult{Rows: rows}
	for _, name := range columns {
		result.Columns = append(result.Columns, db.ResultColumn{Name: name})
	}
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from t"}, result)
	tab.View = app.ViewData
}

func TestCompareOfTwoTablesNamesTheColumnsItLeftOut(t *testing.T) {
	model := buildOfflineModel(t, 140, 30)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	tab.Target.KeyColumns = []string{"id"}

	showTable(tab, []string{"id", "name", "email"}, [][]any{{1, "Ada", "a@x"}, {2, "Alan", "b@x"}})
	pressKey(t, model, tea.KeyPressMsg{Code: 'P', Text: "P"})
	showTable(tab, []string{"id", "customer_id", "total"}, [][]any{{1, 1, 5}, {2, 1, 7}})
	pressKey(t, model, tea.KeyPressMsg{Code: '=', Text: "="})

	frame := stripEscapes(model.render())
	for _, wanted := range []string{
		"different columns · only pinned: name, email · only here: customer_id, total · compared: id",
		"no differences in the compared columns",
	} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the frame has no %q:\n%s", wanted, frame)
		}
	}
}

func TestCompareRefusesResultsWithNoColumnInCommon(t *testing.T) {
	model := buildOfflineModel(t, 140, 30)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult

	showTable(tab, []string{"name"}, [][]any{{"Ada"}})
	pressKey(t, model, tea.KeyPressMsg{Code: 'P', Text: "P"})
	showTable(tab, []string{"total"}, [][]any{{5}})
	pressKey(t, model, tea.KeyPressMsg{Code: '=', Text: "="})

	if frame := stripEscapes(model.render()); !strings.Contains(frame, "have no column in common") {
		t.Errorf("the frame has no refusal:\n%s", frame)
	}
}
