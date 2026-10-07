package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// catalogSession has one table, orders, with one column type of its own.
type catalogSession struct {
	*offlineSession
	totalType string
}

func (session *catalogSession) ListTables(context.Context) ([]db.TableRef, error) {
	return []db.TableRef{{Schema: "public", Name: "orders", Kind: db.RelationTable}}, nil
}

func (session *catalogSession) DescribeTable(
	_ context.Context, table db.TableRef,
) (db.TableDetail, error) {
	return db.TableDetail{Table: table, Columns: []db.ColumnDetail{
		{Name: "id", DataType: "integer", IsPrimaryKey: true},
		{Name: "total", DataType: session.totalType},
	}}, nil
}

func (session *catalogSession) ListIndexes(context.Context, db.TableRef) ([]db.IndexDetail, error) {
	return nil, nil
}

func (session *catalogSession) ListConstraints(
	context.Context, db.TableRef,
) ([]db.ConstraintDetail, error) {
	return nil, nil
}

func openCatalogConnection(model *Model, name, totalType string) *app.Connection {
	connection := app.NewConnection(&catalogSession{
		offlineSession: &offlineSession{
			profile:      cfg.Profile{Name: name, Engine: "postgres"},
			capabilities: core.Capabilities{WritesDDL: true},
		},
		totalType: totalType,
	}, nil, true)
	connection.Catalog.Schemas = []string{"public"}
	model.connections.open(connection)
	return connection
}

func buildCompareModel(t *testing.T) (*Model, *app.Connection) {
	t.Helper()
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	held, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = held.(*Model)
	staging := openCatalogConnection(model, "staging", "integer")
	openCatalogConnection(model, "prod", "bigint")
	model.connections.focus(0)
	model.screen = ScreenWorking
	return model, staging
}

func TestCompareShowsTheChangedColumnOfAnotherConnection(t *testing.T) {
	model, staging := buildCompareModel(t)
	model.openCompareChoice(staging, "public")
	if len(staging.Overlay.Choices) != 1 || staging.Overlay.Choices[0].Label != "prod" {
		t.Fatalf("the card offers %+v", staging.Overlay.Choices)
	}

	for _, message := range drainCommand(pressKey(t, model, tea.KeyPressMsg{Code: 'a', Text: "a"})) {
		model.Update(message)
	}
	tab := staging.Active()
	if tab.Kind != app.TabCompare || tab.ViewData.Kind != app.DataDiff {
		t.Fatalf("the tab is %s with %s", tab.Kind, tab.ViewData.Kind)
	}
	frame := stripEscapes(model.render())
	for _, wanted := range []string{
		"public → prod.public", "~ column total", "- integer", "+ bigint",
	} {
		if !strings.Contains(frame, wanted) {
			t.Errorf("the frame has no %q:\n%s", wanted, frame)
		}
	}
}

func TestCompareWaitsForTheTargetConnection(t *testing.T) {
	model, staging := buildCompareModel(t)
	tab := staging.OpenCompare(app.SchemaCompare{
		SourceSchema: "public", TargetProfile: "archive", TargetSchema: "public",
	})
	model.readCompare(staging, tab)
	if tab.ViewData.Kind != app.DataIdle || tab.ViewData.Reason != "connect to archive to compare" {
		t.Errorf("the tab shows %+v", tab.ViewData)
	}
}

func TestCompareOpensTheAlterScriptInAQueryTab(t *testing.T) {
	model, staging := buildCompareModel(t)
	model.openCompareChoice(staging, "public")
	for _, message := range drainCommand(pressKey(t, model, tea.KeyPressMsg{Code: 'a', Text: "a"})) {
		model.Update(message)
	}
	for _, message := range drainCommand(pressKey(t, model, tea.KeyPressMsg{Code: 'e', Text: "e"})) {
		model.Update(message)
	}
	tab := staging.Active()
	if tab.Kind != app.TabQuery ||
		!strings.Contains(tab.Editor.Text, "ALTER TABLE orders ALTER COLUMN total TYPE bigint;") {
		t.Errorf("the %s tab holds %q", tab.Kind, tab.Editor.Text)
	}
}
