package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
)

// buildBatchModel answers a model whose tab ran two statements, so both strips of the result
// are drawn.
func buildBatchModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"select one", "select two"}, 10)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select one"}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id", DataType: "int"}},
		Rows:    [][]any{{1}, {2}},
	})
	tab.Focus = app.PaneResult
	return model, connection, tab
}

// Every key a strip of a pane draws is a button, so the cells it is recorded at have to be
// the cells its own text was drawn in.
func TestTheKeysOfTheStripsStandWhereTheyAreDrawn(t *testing.T) {
	model, _, _ := buildBatchModel(t)
	frame := strings.Split(model.render(), "\n")

	for _, held := range model.layout.buttons {
		text := cutRowText(frame[held.row], held.from, held.to)
		if strings.TrimSpace(text) == "" {
			t.Errorf("the key of %q on row %d covers %q", held.action, held.row, text)
			continue
		}
		if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
			t.Errorf("the key of %q on row %d covers %q, which is not the key alone",
				held.action, held.row, text)
		}
	}
}

// The mark after the last tab opens one, which is what the mark of a browser does.
func TestAPressOnTheNewTabMarkOpensATab(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	model.render()
	before := len(connection.Tabs)

	held, found := findCardButton(model, ActionNewQueryTab)
	if !found {
		t.Fatal("the tab row recorded no key that opens a tab")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if len(connection.Tabs) != before+1 {
		t.Errorf("a press on the new tab mark left %d tabs", len(connection.Tabs))
	}
}

// The strip over the views steps to the view before or after, so the chips are reached
// without the keyboard even where the strip drew no name.
func TestAPressOnTheViewStepKeyStepsTheView(t *testing.T) {
	model, _, tab := buildBatchModel(t)
	model.render()

	// The step back and the step on share one key, so the key is recorded under the first
	// of the two and the half that is pressed decides which one runs.
	held, found := findCardButton(model, ActionPreviousView)
	if !found {
		t.Fatal("the view strip recorded no key that steps the view")
	}
	if held.second != ActionNextView {
		t.Fatalf("the other half of the key runs %q", held.second)
	}
	before := tab.View
	model.readMouse(tea.MouseClickMsg{X: held.keyTo, Y: held.row, Button: tea.MouseLeft})
	if tab.View == before {
		t.Errorf("a press on the step key left the view on %q", tab.View)
	}
}

// The strip over a plan names what it reads another way, and every one of those keys takes
// a press.
func TestTheKeysOfThePlanStripAreButtons(t *testing.T) {
	model, connection, tab := buildBatchModel(t)
	// A plan view is offered only where the server plans a statement.
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, PlansStatement: true, PlansEveryStatement: true,
	}
	tab.View = app.ViewPlan
	tab.ViewData = app.PaneContent{
		Kind: app.DataPlan,
		Plan: query.QueryPlan{
			Measurable: true, Raw: "Seq Scan on orders",
			Root: query.PlanNode{Label: "Seq Scan on orders"},
		},
	}
	frame := strings.Split(model.render(), "\n")

	for _, action := range []ActionID{
		ActionToggleRawPlan, ActionCopyPlan, ActionExplainAnalyze,
	} {
		held, found := findCardButton(model, action)
		if !found {
			t.Errorf("the plan strip recorded no key for %q", action)
			continue
		}
		if text := cutRowText(frame[held.row], held.from, held.to); strings.TrimSpace(
			text) == "" {
			t.Errorf("the key of %q covers %q", action, text)
		}
	}
}

func TestThePlanTreeDrawsABranchForEveryChildButTheLast(t *testing.T) {
	model, connection, tab := buildBatchModel(t)
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, PlansStatement: true, PlansEveryStatement: true,
	}
	tab.View = app.ViewPlan
	tab.ViewData = app.PaneContent{
		Kind: app.DataPlan,
		Plan: query.QueryPlan{Measurable: true, Root: query.PlanNode{
			Label: "Hash Join", Children: []query.PlanNode{
				{Label: "Hash", Detail: "buckets: 1024",
					Children: []query.PlanNode{{Label: "Seq Scan on customers c"}}},
				{Label: "Seq Scan on orders o"},
			},
		}},
	}
	lines := strings.Split(stripEscapes(model.render()), "\n")
	findColumn := func(text string) int {
		for _, line := range lines {
			if at := strings.Index(line, text); at >= 0 {
				return utf8.RuneCountInString(line[:at])
			}
		}
		t.Fatalf("the plan does not draw %q", text)
		return -1
	}

	branch := findColumn("├ Hash ")
	for _, text := range []string{
		"│ buckets: 1024", "│ └ Seq Scan on customers c", "└ Seq Scan on orders o",
	} {
		if column := findColumn(text); column != branch {
			t.Errorf("%q is drawn at column %d, the branch above it at %d",
				text, column, branch)
		}
	}
}

// The counts on the bottom border of the tree stand for the filter, so a press opens it.
func TestAPressOnTheTreeBorderOpensTheFilter(t *testing.T) {
	model := buildLoadedModel(t, 2, 4, 6, 3)
	connection := model.Active()
	model.render()

	held, found := findCardButton(model, ActionFilterTree)
	if !found {
		t.Fatal("the tree border recorded no key that opens the filter")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if !connection.Tree.Filtering {
		t.Error("a press on the counts left the filter closed")
	}
}

// A second search starts empty. The text of the first one stays applied to the tree, so
// without this the second search reads as the two searches joined.
func TestOpeningTheTreeFilterClearsTheTextOfTheLastSearch(t *testing.T) {
	model := buildLoadedModel(t, 2, 4, 6, 3)
	connection := model.Active()
	model.render()

	held, found := findCardButton(model, ActionFilterTree)
	if !found {
		t.Fatal("the tree border recorded no key that opens the filter")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	model.readTreeFilterKey(connection, tea.Key{Code: 't', Text: "table_0001"})
	model.readTreeFilterKey(connection, tea.Key{Code: tea.KeyEnter})
	if connection.Tree.Filter != "table_0001" {
		t.Fatalf("the filter holds %q after Enter", connection.Tree.Filter)
	}

	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if connection.Tree.Filter != "" {
		t.Errorf("the second search starts on %q, wanted an empty field",
			connection.Tree.Filter)
	}
}

// The connection form names its keys, and a press on one runs it as the key does.
func TestAPressOnAKeyOfTheConnectionFormRunsIt(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.screen = ScreenEditingConnection
	model.form = NewFormState(cfg.Profile{Name: "alpha", Engine: "postgres"}, true, nil)
	model.render()

	held, found := findCardButton(model, ActionClose)
	if !found {
		t.Fatal("the form recorded no key that cancels it")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if model.screen != ScreenPickingProfile {
		t.Errorf("a press on the cancel key left the screen on %q", model.screen)
	}
}

// A field that steps through a list of values draws a mark on each side, and a press on one
// steps it.
func TestAPressOnAChoiceMarkStepsTheField(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.screen = ScreenEditingConnection
	model.form = NewFormState(cfg.Profile{Name: "alpha", Engine: "postgres"}, true, nil)
	// The engine is the field that steps through a list.
	model.form.StepField(1)
	model.render()

	if len(model.layout.formChoices) != 1 {
		t.Fatalf("%d fields drew a mark", len(model.layout.formChoices))
	}
	mark := model.layout.formChoices[0]
	before := model.form.Shown()[mark.field].Value
	model.readMouse(tea.MouseClickMsg{X: mark.on, Y: mark.row, Button: tea.MouseLeft})
	if after := model.form.Shown()[mark.field].Value; after == before {
		t.Errorf("a press on the mark left the field on %q", after)
	}
}

// The profile picker names its keys, and a press on one runs it as the key does.
func TestAPressOnAKeyOfThePickerRunsIt(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	model.screen = ScreenPickingProfile
	model.connections = openConnections{}
	model.profiles = []cfg.Profile{{Name: "alpha", Engine: "postgres"}}
	model.render()

	held, found := findCardButton(model, ActionNewConnection)
	if !found {
		t.Fatal("the picker recorded no key that opens a new connection")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if model.screen != ScreenEditingConnection {
		t.Errorf("a press on the new key left the screen on %q", model.screen)
	}
}
