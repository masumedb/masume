package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// orderItems is a relation of the offline server.
var orderItems = db.TableRef{Schema: "public", Name: "order_items", Kind: db.RelationTable}

// buildOrderItemsDetail returns the columns of order_items, with id as the primary key or
// with no key.
func buildOrderItemsDetail(keyed bool) db.TableDetail {
	return db.TableDetail{Table: orderItems, Columns: []db.ColumnDetail{
		{Name: "id", DataType: "integer", IsPrimaryKey: keyed},
		{Name: "qty", DataType: "integer"},
	}}
}

func TestTableTabReadsInThePrimaryKeyOrder(t *testing.T) {
	for _, test := range []struct {
		name      string
		keyed     bool
		sortsRead bool
		sort      []core.SortState
		want      string
		wantNot   string
	}{
		{name: "a table with a primary key", keyed: true, sortsRead: true,
			want: `order by "id" asc`},
		{name: "a sort of the grid replaces the key order", keyed: true, sortsRead: true,
			sort: []core.SortState{{Column: "qty", Direction: core.SortDescending}},
			want: `order by "qty" desc`, wantNot: `"id"`},
		{name: "a table without a primary key", sortsRead: true, wantNot: "order by"},
		{name: "a server that cannot sort", keyed: true, wantNot: "order by"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := buildOfflineModel(t, 160, 48)
			connection := model.Active()
			connection.Session.(*offlineSession).capabilities.SortsRead = test.sortsRead
			connection.Catalog.Details[present.BuildTableID(orderItems)] = present.TableDetailState{
				Kind: present.DetailReady, Detail: buildOrderItemsDetail(test.keyed),
			}
			tab := connection.OpenTable(orderItems, "")
			tab.Sort = test.sort

			model.runTabRead(connection, tab)

			if model.runs.count() != 1 {
				t.Fatalf("the read of the rows was not sent")
			}
			source := tab.Results.Active().Source
			if test.want != "" && !strings.Contains(source, test.want) {
				t.Errorf("the read is %q, wanted it to hold %q", source, test.want)
			}
			if test.wantNot != "" && strings.Contains(source, test.wantNot) {
				t.Errorf("the read is %q, wanted it without %q", source, test.wantNot)
			}
		})
	}
}

func TestTableTabReadsItsRowsAfterItsColumns(t *testing.T) {
	for _, test := range []struct {
		name    string
		problem string
		want    string
	}{
		{name: "the columns are read", want: `order by "id" asc`},
		{name: "the columns cannot be read", problem: "permission denied"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := buildOfflineModel(t, 160, 48)
			connection := model.Active()
			tab := connection.OpenTable(orderItems, "")

			_, command := model.runTabRead(connection, tab)

			if command == nil || !tab.WaitsForKey || model.runs.count() != 0 {
				t.Fatalf("the rows were read before the columns; waits %v, runs %d",
					tab.WaitsForKey, model.runs.count())
			}

			answered := tableDetailMsg{
				ConnectionID: model.ActiveID(), TableID: present.BuildTableID(orderItems),
				Problem: test.problem,
			}
			if test.problem == "" {
				answered.Detail = buildOrderItemsDetail(true)
			}
			_, command = model.readTableDetailAnswer(answered)

			if command == nil || tab.WaitsForKey || model.runs.count() != 1 {
				t.Fatalf("the rows were not read after the columns; waits %v, runs %d",
					tab.WaitsForKey, model.runs.count())
			}
			source := tab.Results.Active().Source
			if test.want != "" && !strings.Contains(source, test.want) {
				t.Errorf("the read is %q, wanted it to hold %q", source, test.want)
			}
			if test.want == "" && strings.Contains(source, "order by") {
				t.Errorf("the read is %q, wanted it without a sort", source)
			}
		})
	}
}

func TestApplyingChangesKeepsTheCursorOnTheEditedRow(t *testing.T) {
	for _, test := range []struct {
		name string
		rows [][]any
		want int
	}{
		{name: "the row moved", rows: [][]any{
			{int64(1), "ada"}, {int64(3), "hedy"}, {int64(4), "joan"}, {int64(2), "grace"},
		}, want: 3},
		{name: "the row is gone", rows: [][]any{
			{int64(1), "ada"}, {int64(3), "hedy"}, {int64(4), "joan"},
		}, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			model, connection, tab := buildTableTabModel(t)
			connection.Catalog.Details[present.BuildTableID(tab.Table)] = present.TableDetailState{
				Kind: present.DetailReady, Detail: db.TableDetail{Table: tab.Table,
					Columns: []db.ColumnDetail{
						{Name: "id", DataType: "integer", IsPrimaryKey: true},
						{Name: "customer", DataType: "text"},
					}},
			}
			columns := tab.Results.Active().State.Result.Columns
			model.placeResultCursor(connection, tab, columns)
			tab.Target = model.resolveEditTarget(connection, tab)
			tab.GridRow = 1

			model.readChangesAnswer(changesAppliedMsg{
				ConnectionID: model.ActiveID(), TabID: tab.ID, Applied: 1,
			})
			read := tab.Results.Active().Source
			model.readQueryAnswer(queryRanMsg{
				ConnectionID: model.ActiveID(), TabID: tab.ID, RunID: model.runs.nextRunID,
				Last: true, Read: db.ComposedRead{Text: read, Display: read},
				Result: db.QueryResult{Columns: columns, Rows: test.rows},
			})

			if active := tab.Results.Active(); active.State.Kind != app.QuerySucceeded {
				t.Fatalf("the read after the apply did not land: %v", active.State.Kind)
			}
			if tab.GridRow != test.want {
				t.Errorf("the cursor is on row %d, wanted row %d", tab.GridRow, test.want)
			}
		})
	}
}
