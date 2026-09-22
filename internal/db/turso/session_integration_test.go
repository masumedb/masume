//go:build integration

// An integration test: it reads a real libSQL server. The server is started outside this
// code, by `mise run servers-up` or by whatever runs the build, and named to the test
// through MASUME_TEST_TURSO. Nothing here knows how it was started.
//
// The build tag keeps these off `go test ./...` entirely: without `-tags=integration` the
// file is not even compiled.
package turso_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

const dropShop = `drop table if exists orders;`

var shopSchema = []string{
	dropShop,
	`create table orders (
	   id       integer primary key autoincrement,
	   customer text not null,
	   total    real default 0,
	   paid_at  text
	 );`,
	`create index orders_customer_idx on orders (customer);`,
	`create view paid_orders as select * from orders where total > 0;`,
	`insert into orders (customer, total) values ('ada', 12.50), ('grace', 0), ('alan', 99.00);`,
}

// openShop answers a session with the schema laid out. It is dropped first, so a run that
// was cut short leaves nothing for the next one, and dropped again after.
func openShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Turso)
	dbtest.RunStatements(t, session, append([]string{`drop view if exists paid_orders;`},
		shopSchema...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), dropShop, dbtest.ReadEverything, nil)
	})
	return session
}

func TestServerRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total from orders order by customer", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 3 {
		t.Fatalf("the read gave %d rows, wanted 3", len(answered.Rows))
	}
	if answered.Columns[0].Name != "customer" {
		t.Errorf("the first column is %q, wanted customer", answered.Columns[0].Name)
	}
	// The server sends no column type, so the type of the first value stands for the column.
	if answered.Columns[1].DataType != "real" {
		t.Errorf("the total reads as %q, wanted real", answered.Columns[1].DataType)
	}
}

// The client runs `begin` and `commit` as statements of its own. The websocket protocol
// holds one stream for the session, so a transaction spans the statements between them.
func TestServerHoldsATransactionAcrossStatements(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	if _, err := session.RunQuery(ctx,
		"insert into orders (customer, total) values ('turing', 1)",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the insert answered %v", err)
	}
	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}
	if held := session.ReadTransactionState(); held != db.TransactionNone {
		t.Errorf("the state reads %q after a rollback, wanted none", held)
	}

	answered, err := session.RunQuery(ctx,
		"select count(*) from orders", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if written := fmt.Sprintf("%v", answered.Rows[0][0]); written != "3" {
		t.Errorf("the table holds %s rows after a rollback, wanted 3", written)
	}
}

func TestServerListsTheTablesOfTheCatalog(t *testing.T) {
	session := openShop(t)

	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	kinds := map[string]db.RelationKind{}
	for _, table := range tables {
		kinds[table.Name] = table.Kind
	}
	if kinds["orders"] != db.RelationTable {
		t.Errorf("orders reads as %q, wanted a table", kinds["orders"])
	}
	if kinds["paid_orders"] != db.RelationView {
		t.Errorf("paid_orders reads as %q, wanted a view", kinds["paid_orders"])
	}
}

func TestServerDescribesATable(t *testing.T) {
	session := openShop(t)

	detail, err := session.DescribeTable(context.Background(), db.TableRef{Schema: "main", Name: "orders"})
	if err != nil {
		t.Fatalf("the describe answered %v", err)
	}
	byName := map[string]db.ColumnDetail{}
	for _, column := range detail.Columns {
		byName[column.Name] = column
	}
	if !byName["id"].IsPrimaryKey {
		t.Error("id does not read as the primary key")
	}
	if byName["customer"].Nullable {
		t.Error("customer is declared not null and reads as nullable")
	}
	if !byName["total"].HasDefault {
		t.Error("total has a default and does not report one")
	}
}

func TestServerListsTheIndexesOfATable(t *testing.T) {
	session := openShop(t)

	indexes, err := session.ListIndexes(context.Background(), db.TableRef{Schema: "main", Name: "orders"})
	if err != nil {
		t.Fatalf("the index list answered %v", err)
	}
	found := false
	for _, index := range indexes {
		if index.Name == "orders_customer_idx" {
			found = true
		}
	}
	if !found {
		t.Error("orders_customer_idx is not in the index list")
	}
}

func TestServerAnswersTheDefinitionOfATable(t *testing.T) {
	session := openShop(t)

	lines, err := session.BuildTableDDL(context.Background(), db.TableRef{Schema: "main", Name: "orders"})
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("the definition came back empty")
	}
}

func TestServerExplainsAStatement(t *testing.T) {
	session := openShop(t)

	plan, err := session.ExplainQuery(context.Background(),
		"select * from orders where customer = 'ada'", false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if plan.Root.Label == "" && plan.Raw == "" {
		t.Error("the server answered a plan with nothing in it")
	}
}

// A statement the server refuses keeps the error of the driver in the chain, so the message
// the server wrote reaches the user and a caller can still read the type.
func TestServerAnswersAStatementItRefuses(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"insert into orders (customer) values (null)", dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a null in a not-null column answered no error")
	}
	if !errors.Is(err, db.ErrDatabase) {
		t.Error("the error does not read as one from the database")
	}
	if described := db.DescribeError(err); described == "" {
		t.Error("the error is described as an empty text")
	}
}
