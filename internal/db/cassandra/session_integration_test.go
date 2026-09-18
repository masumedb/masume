//go:build integration

// An integration test: it reads a real Cassandra. The server is started outside this code,
// by `mise run servers-up` or by whatever runs the build, and named to the test through
// MASUME_TEST_CASSANDRA. Nothing here knows how it was started.
//
// The build tag keeps these off `go test ./...` entirely: without `-tags=integration` the
// file is not even compiled.
package cassandra_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
)

// testSchema is the keyspace of these tests, which the connection opens.
const testSchema = "masume_test"

var shopSchema = []string{
	`drop table if exists orders`,
	`create table orders (
	   customer text,
	   id       timeuuid,
	   total    decimal,
	   paid_at  timestamp,
	   primary key (customer, id)
	 ) with clustering order by (id desc)`,
	`create index orders_total_idx on orders (total)`,
	`insert into orders (customer, id, total) values ('ada', now(), 12.50)`,
	`insert into orders (customer, id, total) values ('grace', now(), 0)`,
	`insert into orders (customer, id, total, paid_at) values ('alan', now(), 99.00, '2026-01-02 03:04:05')`,
}

// laidOut lays the table out once. The server waits for every node to agree on a schema
// change, so a table built for each test would cost more than the tests themselves.
var laidOut sync.Once

// openShop answers a session on the keyspace of these tests, with the table laid out.
func openShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Cassandra)
	laidOut.Do(func() { dbtest.RunStatements(t, session, shopSchema...) })
	return session
}

func TestServerRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total, paid_at from orders", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 3 {
		t.Fatalf("the read gave %d rows, wanted 3", len(answered.Rows))
	}
	if answered.Columns[1].DataType != "decimal" {
		t.Errorf("the total reads as %q, wanted decimal", answered.Columns[1].DataType)
	}
}

// A column with no value is a null, and not the zero value of its type. The driver scans
// into a holder of its own, so the adapter reads the null back through a pointer.
func TestServerAnswersANullColumnAsNothing(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select paid_at from orders where customer = 'ada'", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 1 {
		t.Fatalf("the read gave %d rows, wanted 1", len(answered.Rows))
	}
	if held := answered.Rows[0][0]; held != nil {
		t.Errorf("the empty timestamp reads as %#v, wanted nothing", held)
	}
}

func TestServerListsTheTablesOfTheCatalog(t *testing.T) {
	session := openShop(t)

	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	found := false
	for _, table := range tables {
		if table.Schema == testSchema && table.Name == "orders" {
			found = true
			if table.Kind != db.RelationTable {
				t.Errorf("orders reads as %q, wanted a table", table.Kind)
			}
		}
	}
	if !found {
		t.Error("orders is not in the table list")
	}
}

// The keyspace list covers the keyspaces the server keeps for itself as well as this one.
func TestServerListsItsKeyspaces(t *testing.T) {
	session := openShop(t)

	schemas, err := session.ListSchemas(context.Background())
	if err != nil {
		t.Fatalf("the keyspace list answered %v", err)
	}
	held := map[string]bool{}
	for _, schema := range schemas {
		held[schema] = true
	}
	if !held[testSchema] {
		t.Errorf("%s is not in the keyspace list", testSchema)
	}
	if !held["system_schema"] {
		t.Error("system_schema is not in the keyspace list")
	}
}

// The key of the table is its partition column and its clustering column, in that order,
// and the describe draws them before every other column.
func TestServerDescribesATable(t *testing.T) {
	session := openShop(t)

	detail, err := session.DescribeTable(context.Background(),
		db.TableRef{Schema: testSchema, Name: "orders"})
	if err != nil {
		t.Fatalf("the describe answered %v", err)
	}
	if len(detail.Columns) != 4 {
		t.Fatalf("the table holds %d columns, wanted 4", len(detail.Columns))
	}
	if detail.Columns[0].Name != "customer" || !detail.Columns[0].IsPrimaryKey {
		t.Errorf("the first column is %q, wanted the partition key", detail.Columns[0].Name)
	}
	if detail.Columns[1].Name != "id" || !detail.Columns[1].IsPrimaryKey {
		t.Errorf("the second column is %q, wanted the clustering column", detail.Columns[1].Name)
	}
	byName := map[string]db.ColumnDetail{}
	for _, column := range detail.Columns {
		byName[column.Name] = column
	}
	if byName["customer"].Nullable {
		t.Error("the partition key reads as nullable")
	}
	if !byName["total"].Nullable {
		t.Error("a column outside the key takes a null and reads as not nullable")
	}
}

func TestServerListsTheIndexesAndTheKeyOfATable(t *testing.T) {
	session := openShop(t)
	table := db.TableRef{Schema: testSchema, Name: "orders"}

	indexes, err := session.ListIndexes(context.Background(), table)
	if err != nil {
		t.Fatalf("the index list answered %v", err)
	}
	found := false
	for _, index := range indexes {
		if index.Name == "orders_total_idx" {
			found = true
		}
	}
	if !found {
		t.Error("orders_total_idx is not in the index list")
	}

	constraints, constraintErr := session.ListConstraints(context.Background(), table)
	if constraintErr != nil {
		t.Fatalf("the constraint list answered %v", constraintErr)
	}
	if len(constraints) != 1 || constraints[0].Kind != db.ConstraintPrimaryKey {
		t.Fatalf("the table answered %d constraints, wanted its primary key", len(constraints))
	}
}

func TestServerAnswersTheDefinitionOfATable(t *testing.T) {
	session := openShop(t)

	lines, err := session.BuildTableDDL(context.Background(),
		db.TableRef{Schema: testSchema, Name: "orders"})
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	written := ""
	for _, line := range lines {
		written += line + "\n"
	}
	for _, wanted := range []string{
		`create table "masume_test"."orders"`,
		`primary key ("customer", "id")`,
		`clustering order by ("id" DESC)`,
		`create index "orders_total_idx"`,
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the definition holds no %q:\n%s", wanted, written)
		}
	}
}

// A page after the first is taken from the rows above it, because CQL has no OFFSET.
func TestServerReadsAPageAfterTheFirst(t *testing.T) {
	session := openShop(t)
	read := session.Composer().ComposeRelationRead(
		db.TableRef{Schema: testSchema, Name: "orders"}, core.ReadRewrite{})

	first, err := session.ReadPage(context.Background(), read, db.ReadWindow{Limit: 2})
	if err != nil {
		t.Fatalf("the first page answered %v", err)
	}
	if len(first.Rows) != 2 {
		t.Fatalf("the first page gave %d rows, wanted 2", len(first.Rows))
	}
	second, secondErr := session.ReadPage(
		context.Background(), read, db.ReadWindow{Limit: 2, Offset: 2})
	if secondErr != nil {
		t.Fatalf("the second page answered %v", secondErr)
	}
	if len(second.Rows) != 1 {
		t.Fatalf("the second page gave %d rows, wanted 1", len(second.Rows))
	}
}

// The staged changes of the grid run in one logged batch.
func TestServerAppliesStagedChangesTogether(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	// The test writes a partition of its own, so the rows the other tests read stay as
	// they are.
	dbtest.RunStatements(t, session,
		`insert into orders (customer, id, total) values ('turing', now(), 2.00)`,
		`insert into orders (customer, id, total) values ('lovelace', now(), 4.00)`)
	t.Cleanup(func() {
		dbtest.RunStatements(t, session,
			`delete from orders where customer = 'turing'`,
			`delete from orders where customer = 'lovelace'`)
	})

	changes := []db.Change{
		{Payload: db.BoundStatement{
			SQL:    "update orders set total = ? where customer = ? and id = ?",
			Params: []any{"1.00", "turing", readFirstID(t, session, "turing")},
		}},
		{Payload: db.BoundStatement{
			SQL:    "delete from orders where customer = ?",
			Params: []any{"lovelace"},
		}},
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the batch answered %v", err)
	}

	answered, err := session.RunQuery(ctx,
		"select total from orders where customer = 'turing'", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 1 {
		t.Fatalf("the partition holds %d rows, wanted 1", len(answered.Rows))
	}
	if written := db.ReadAnyText(answered.Rows[0][0]); written != "1.00" {
		t.Errorf("the total reads %q after the batch, wanted 1.00", written)
	}

	gone, goneErr := session.RunQuery(ctx,
		"select total from orders where customer = 'lovelace'", dbtest.ReadEverything, nil)
	if goneErr != nil {
		t.Fatalf("the read answered %v", goneErr)
	}
	if len(gone.Rows) != 0 {
		t.Errorf("the deleted partition holds %d rows, wanted none", len(gone.Rows))
	}
}

// readFirstID answers the clustering key of the one row of that customer.
func readFirstID(t *testing.T, session db.Session, customer string) any {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select id from orders where customer = ?", dbtest.ReadEverything, []any{customer})
	if err != nil {
		t.Fatalf("the key read answered %v", err)
	}
	if len(answered.Rows) == 0 {
		t.Fatalf("no row of %s", customer)
	}
	return answered.Rows[0][0]
}

// The server has no EXPLAIN, and a caller must read that back rather than a plan.
func TestServerExplainsNothing(t *testing.T) {
	session := openShop(t)

	_, err := session.ExplainQuery(context.Background(), "select * from orders", false)
	if err == nil {
		t.Fatal("the plan answered no error")
	}
	if !errors.Is(err, db.ErrDatabase) {
		t.Errorf("the plan answered %v, wanted an error of the database tier", err)
	}
}

// A statement the server refuses keeps the error of the driver in the chain, so the message
// the server wrote reaches the user and a caller can still read the type.
func TestServerAnswersAStatementItRefuses(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"select * from nothing_here", dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a read of a table that is not there answered no error")
	}
	if !errors.Is(err, db.ErrDatabase) {
		t.Error("the error does not read as one from the database")
	}
}
