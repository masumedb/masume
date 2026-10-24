package ui

import (
	"context"
	"slices"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// planningSession records what the plan view asked the server for.
type planningSession struct {
	offlineSession
	planned  []string
	analyzed bool
}

func (session *planningSession) ExplainQuery(
	_ context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	session.planned = append(session.planned, sql)
	session.analyzed = analyze
	return db.QueryPlan{}, nil
}

// buildPlanningModel answers a model on one connection whose server plans every statement.
func buildPlanningModel(t *testing.T) (*Model, *planningSession, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	session := &planningSession{
		profile: cfg.Profile{Name: "offline", Engine: "postgres"},
		capabilities: core.Capabilities{
			PlansStatement: true, PlansEveryStatement: true,
		},
	}
	connection := app.NewConnection(session, nil, true)
	model.connections.open(connection)
	tab := connection.Active()
	if tab == nil {
		t.Fatal("the connection opened with no tab")
	}
	return model, session, connection, tab
}

// A measured plan runs the statement it measures, so the key that asks for one only ever
// estimates a write, and the user is told the write did not run.
func TestExplainAnalyzeOnlyEstimatesAWrite(t *testing.T) {
	model, session, connection, tab := buildPlanningModel(t)
	tab.Editor = app.NewEditorBuffer("delete from orders", 0)

	_, command := model.explain(connection, tab, true)
	if command == nil {
		t.Fatal("the plan was not asked for")
	}
	command()

	if len(session.planned) != 1 {
		t.Fatalf("the server was asked %v, wanted the one statement", session.planned)
	}
	if session.analyzed {
		t.Error("a write was measured, so it ran")
	}
	if connection.Notice == nil {
		t.Error("the write was estimated without a word")
	}
}

// A read is measured as asked, because measuring it changes nothing.
func TestExplainAnalyzeMeasuresARead(t *testing.T) {
	model, session, connection, tab := buildPlanningModel(t)
	tab.Editor = app.NewEditorBuffer("select * from orders", 0)

	_, command := model.explain(connection, tab, true)
	if command == nil {
		t.Fatal("the plan was not asked for")
	}
	command()

	if !session.analyzed {
		t.Error("a read was only estimated")
	}
}

// A plan of a new statement replaces the views of the run before it.
func TestExplainReplacesTheViewsOfThePreviousRun(t *testing.T) {
	model, session, connection, tab := buildPlanningModel(t)
	tab.Results.Start([]string{"update orders set total = 1"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{Affected: 3, HasAffected: true})
	tab.Editor = app.NewEditorBuffer("select * from orders", 0)

	_, command := model.explain(connection, tab, false)
	if command == nil {
		t.Fatal("the plan was not asked for")
	}
	model.Update(command())

	views := tab.Views(session)
	if slices.Contains(views, app.ViewStatistics) {
		t.Errorf("the views are %v, wanted the views of the planned statement", views)
	}
	if tab.ActiveView(session) != app.ViewPlan {
		t.Errorf("the view is %v, wanted the plan", tab.ActiveView(session))
	}
}

// A plan of the statement that ran keeps its rows.
func TestExplainKeepsTheResultOfTheSameStatement(t *testing.T) {
	model, _, connection, tab := buildPlanningModel(t)
	tab.Results.Start([]string{"select * from orders"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id"}}, Rows: [][]any{{1}},
	})
	tab.Editor = app.NewEditorBuffer("select * from orders", 0)

	_, command := model.explain(connection, tab, false)
	model.Update(command())

	if tab.Results.Active() == nil {
		t.Error("the rows of the statement were dropped")
	}
}
