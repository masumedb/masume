package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// searchSession has one table, customers, and answers every read with one row.
type searchSession struct {
	*offlineSession
	ran []string
}

func (session *searchSession) ListTables(context.Context) ([]db.TableRef, error) {
	return []db.TableRef{{Schema: "public", Name: "customers", Kind: db.RelationTable}}, nil
}

func (session *searchSession) DescribeTable(
	_ context.Context, table db.TableRef,
) (db.TableDetail, error) {
	return db.TableDetail{Table: table, Columns: []db.ColumnDetail{{Name: "name", DataType: "text"}}}, nil
}

func (session *searchSession) RunQuery(
	_ context.Context, sql string, _ int, _ []any,
) (db.QueryResult, error) {
	session.ran = append(session.ran, sql)
	return db.QueryResult{Columns: []db.ResultColumn{{Name: "name"}}, Rows: [][]any{{"Ada"}}}, nil
}

func TestDataSearchOpensTheMatchedTableFiltered(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	connection := app.NewConnection(&searchSession{offlineSession: &offlineSession{}}, nil, true)
	model.connections.open(connection)
	model.connections.focus(1)

	model.openDataSearch(connection, "public")
	_, command := model.searchData(connection, "ada")
	for _, message := range drainCommand(command) {
		model.Update(message)
	}
	if connection.Overlay.Kind != app.OverlayActionMenu || len(connection.Overlay.Actions) != 1 ||
		connection.Overlay.Actions[0].Detail != "1 row · name" {
		t.Fatalf("the overlay is %s with %+v", connection.Overlay.Kind, connection.Overlay.Actions)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	tab := connection.Active()
	wanted := `cast("name" as text) ilike '%ada%'`
	if tab.Kind != app.TabTable || tab.Table.Name != "customers" || len(tab.Filter) != 1 ||
		tab.Filter[0].Kind != core.FilterRaw || tab.Filter[0].Text != wanted {
		t.Errorf("the %s tab of %q has the filter %+v", tab.Kind, tab.Table.Name, tab.Filter)
	}
	if !strings.Contains(stripEscapes(model.render()), "1 table of 1 holds ada") {
		t.Error("the frame has no summary of the search")
	}
}
