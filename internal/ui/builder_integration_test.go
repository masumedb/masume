//go:build integration

// An integration test: it builds a query against the real PostgreSQL named by
// MASUME_TEST_POSTGRES, through the keys of the builder tab. The schema it reads is laid
// out by the test itself.
//
// The build tag keeps these off `go test ./...` entirely: without `-tags=integration` the
// file is not even compiled.
package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// The schema of the test: two tables and the foreign key between them.
const (
	dropBuilderSchema = `drop schema if exists masume_builder cascade;`
	builderSchema     = `
create schema masume_builder;
create table masume_builder.customers (
  id   serial primary key,
  name text not null
);
create table masume_builder.orders (
  id          serial primary key,
  customer_id int not null references masume_builder.customers(id),
  total       numeric(10,2)
);
insert into masume_builder.customers (name) values ('ada'), ('grace');
insert into masume_builder.orders (customer_id, total) values (1, 12.50), (1, 99.00), (2, 5.00);
`
)

// openServerBuilder opens a builder tab on the real server, with the schema of the test laid
// out and the catalog read.
func openServerBuilder(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	session, err := engines.CreateAdapters().Open(t.Context(), profile, password)
	if err != nil {
		t.Fatalf("cannot reach the server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	dbtest.RunStatements(t, session, dropBuilderSchema, builderSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropBuilderSchema, dbtest.ReadEverything, nil)
	})

	model := buildOfflineModel(t, 118, 34)
	connection := app.NewConnection(session, nil, true)
	model.connections.open(connection)

	tables, err := session.ListTables(t.Context())
	if err != nil {
		t.Fatalf("the catalog read failed: %v", err)
	}
	connection.Catalog.Tables, connection.Catalog.Loading = tables, false

	tab := connection.OpenBuilder()
	tab.Focus = app.PaneEditor
	return model, connection, tab
}

// addServerTable adds one table through the picker, as the keys do.
func addServerTable(
	t *testing.T, model *Model, connection *app.Connection, tab *app.Tab, name string,
) {
	t.Helper()
	model.runBuilderAction(connection, tab, Match{Action: ActionAddBuilderTable})

	found := false
	for at, row := range model.filterBuilderTables(connection.Overlay) {
		if row.Label == name {
			connection.Overlay.List.Cursor, found = at, true
		}
	}
	if !found {
		t.Fatalf("the picker lists no %s", name)
	}
	_, command := model.chooseOverlayRow(connection, tab, &connection.Overlay, false)
	runBuilderCommand(t, model, command)
}

// runBuilderCommand runs a command of the draw loop and feeds its answer back.
func runBuilderCommand(t *testing.T, model *Model, command tea.Cmd) {
	t.Helper()
	for at := 0; command != nil && at < 8; at++ {
		message := command()
		if message == nil {
			return
		}
		_, command = model.Update(message)
	}
}

// The builder joins two tables on their foreign key and runs the statement it wrote.
func TestBuilderRunsAgainstTheServer(t *testing.T) {
	model, connection, tab := openServerBuilder(t)

	addServerTable(t, model, connection, tab, "masume_builder.customers")
	addServerTable(t, model, connection, tab, "masume_builder.orders")

	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Fatalf("the second table opened %q", connection.Overlay.Kind)
	}
	if !strings.Contains(connection.Overlay.Body, "foreign key") {
		t.Errorf("the join card says %q", connection.Overlay.Body)
	}
	model.applyJoinCard(connection, tab, connection.Overlay.Field, connection.Overlay.Draft.Text)

	// The name of the customer, and the total of the order.
	tab.Builder.MoveCursor(0, 1)
	model.runBuilderAction(connection, tab, Match{Action: ActionPickColumn})
	tab.Builder.MoveCursor(1, 2)
	model.runBuilderAction(connection, tab, Match{Action: ActionPickColumn})

	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if !strings.Contains(written,
		`inner join "masume_builder"."orders" o on o.customer_id = c.id`) {
		t.Fatalf("the builder wrote\n%s", written)
	}

	_, command := model.runStatementAtCursor(connection, tab)
	runBuilderCommand(t, model, command)
	waitForBuilderResult(t, tab)

	result := tab.Results.State().Result
	if len(result.Columns) != 2 || len(result.Rows) != 3 {
		t.Fatalf("the server answered %d columns and %d rows",
			len(result.Columns), len(result.Rows))
	}
	if result.Columns[0].Name != "name" || result.Columns[1].Name != "total" {
		t.Errorf("the columns read %q and %q",
			result.Columns[0].Name, result.Columns[1].Name)
	}
}

// The aggregate of a column groups the read by every other picked column, and the server
// answers the rows of that grouping.
func TestBuilderRunsAnAggregateAgainstTheServer(t *testing.T) {
	model, connection, tab := openServerBuilder(t)

	addServerTable(t, model, connection, tab, "masume_builder.customers")
	addServerTable(t, model, connection, tab, "masume_builder.orders")
	model.applyJoinCard(connection, tab, connection.Overlay.Field, connection.Overlay.Draft.Text)

	tab.Builder.MoveCursor(0, 1)
	model.runBuilderAction(connection, tab, Match{Action: ActionPickColumn})
	tab.Builder.MoveCursor(1, 0)
	model.runBuilderAction(connection, tab, Match{Action: ActionEditBuilderRow})
	stepBuilderField(tab.Builder, builderFieldAggregate, 1)
	connection.Overlay.Draft.SetText("orders")
	model.applyBuilderField(connection, tab, connection.Overlay.Draft.Text)

	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if !strings.Contains(written, "count(o.id) as orders") ||
		!strings.Contains(written, " group by c.name") {
		t.Fatalf("the builder wrote\n%s", written)
	}

	_, command := model.runStatementAtCursor(connection, tab)
	runBuilderCommand(t, model, command)
	waitForBuilderResult(t, tab)

	result := tab.Results.State().Result
	if len(result.Rows) != 2 {
		t.Fatalf("the server answered %d rows, wanted one per customer", len(result.Rows))
	}
}

// waitForBuilderResult waits for the run of the tab to finish.
func waitForBuilderResult(t *testing.T, tab *app.Tab) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		switch tab.Results.State().Kind {
		case app.QuerySucceeded:
			return
		case app.QueryFailed:
			t.Fatalf("the statement failed: %s", tab.Results.State().Message)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the statement did not finish")
}
