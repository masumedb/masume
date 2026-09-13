package app

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// buildBuilderDialect returns a dialect that quotes with double quotes.
func buildBuilderDialect() *query.Dialect {
	return &query.Dialect{
		Engine:          core.EnginePostgres,
		QuoteIdentifier: func(name string) string { return `"` + name + `"` },
	}
}

// addTable puts a table with its columns into the builder.
func addTable(builder *Builder, name string, columns []string, keys ...query.ForeignKey) int {
	at := builder.AddTable(db.TableRef{Schema: "shop", Name: name})
	detail := db.TableDetail{ForeignKeys: keys}
	for _, column := range columns {
		detail.Columns = append(detail.Columns, db.ColumnDetail{Name: column})
	}
	builder.WriteColumns(at, detail)
	return at
}

func TestBuilderWritesTheSQLOfItsTables(t *testing.T) {
	builder := NewBuilder()
	customers := addTable(builder, "customers", []string{"id", "name"})
	orders := addTable(builder, "orders", []string{"id", "customer_id", "total"})
	builder.AddJoin(BuilderJoin{
		Kind: statement.JoinInner, Table: orders, Base: customers,
		Column: "customer_id", BaseColumn: "id",
	})
	builder.Tables[customers].Columns[1].Picked = true
	builder.Tables[orders].Columns[2].Picked = true

	written := builder.BuildSQL(buildBuilderDialect())
	wanted := `select c.name, o.total
  from "shop"."customers" c
  inner join "shop"."orders" o on o.customer_id = c.id`
	if written != wanted {
		t.Errorf("the builder wrote\n%s\nwanted\n%s", written, wanted)
	}
}

// Every table gets an alias of its own, so one table joined twice is still readable.
func TestBuilderGivesEveryTableItsOwnAlias(t *testing.T) {
	builder := NewBuilder()
	addTable(builder, "customers", []string{"id"})
	addTable(builder, "orders", []string{"id"})
	addTable(builder, "order_items", []string{"id"})
	addTable(builder, "carts", []string{"id"})

	aliases := []string{}
	for _, table := range builder.Tables {
		aliases = append(aliases, table.Alias)
	}
	held := map[string]bool{}
	for _, alias := range aliases {
		if held[alias] {
			t.Fatalf("the aliases repeat: %v", aliases)
		}
		held[alias] = true
	}
	if aliases[0] != "c" || aliases[1] != "o" {
		t.Errorf("the aliases read %v", aliases)
	}
}

// The foreign key of the new table proposes the join.
func TestBuilderProposesTheJoinOfAForeignKey(t *testing.T) {
	builder := NewBuilder()
	addTable(builder, "customers", []string{"id", "name"})
	orders := addTable(builder, "orders", []string{"id", "customer_id"}, query.ForeignKey{
		Columns: []string{"customer_id"}, TargetSchema: "shop", TargetTable: "customers",
		TargetColumns: []string{"id"},
	})

	join, found := builder.FindForeignKeyJoin(orders)
	if !found {
		t.Fatal("the foreign key proposed no join")
	}
	if join.Table != orders || join.Base != 0 ||
		join.Column != "customer_id" || join.BaseColumn != "id" {
		t.Errorf("the join reads %+v", join)
	}
}

// A key of a table already in the builder proposes the join as well, so the order the
// tables are added in does not matter.
func TestBuilderProposesTheJoinOfAKeyPointingAtTheNewTable(t *testing.T) {
	builder := NewBuilder()
	addTable(builder, "orders", []string{"id", "customer_id"}, query.ForeignKey{
		Columns: []string{"customer_id"}, TargetSchema: "shop", TargetTable: "customers",
		TargetColumns: []string{"id"},
	})
	customers := addTable(builder, "customers", []string{"id", "name"})

	join, found := builder.FindForeignKeyJoin(customers)
	if !found {
		t.Fatal("the foreign key proposed no join")
	}
	if join.Column != "id" || join.BaseColumn != "customer_id" {
		t.Errorf("the join reads %+v", join)
	}
}

// Two tables with no key between them propose nothing, and the card asks for the condition.
func TestBuilderProposesNoJoinWithoutAForeignKey(t *testing.T) {
	builder := NewBuilder()
	addTable(builder, "customers", []string{"id"})
	products := addTable(builder, "products", []string{"id", "name"})

	if _, found := builder.FindForeignKeyJoin(products); found {
		t.Error("a join was proposed without a foreign key")
	}
}

// The condition the user wrote replaces the columns of the key.
func TestBuilderWritesTheConditionTheUserTyped(t *testing.T) {
	builder := NewBuilder()
	customers := addTable(builder, "customers", []string{"id"})
	orders := addTable(builder, "orders", []string{"id", "customer_id"})
	builder.AddJoin(BuilderJoin{
		Kind: statement.JoinLeft, Table: orders, Base: customers,
		On: "o.customer_id = c.id and o.total > 0",
	})

	written := builder.BuildSQL(buildBuilderDialect())
	if !strings.Contains(written, "left join \"shop\".\"orders\" o on o.customer_id = c.id and o.total > 0") {
		t.Errorf("the builder wrote\n%s", written)
	}
}

// Dropping a table drops every table joined through it, so no join is left without a side.
func TestBuilderDropsTheTablesJoinedThroughTheOneItDrops(t *testing.T) {
	builder := NewBuilder()
	customers := addTable(builder, "customers", []string{"id"})
	orders := addTable(builder, "orders", []string{"id", "customer_id"})
	items := addTable(builder, "order_items", []string{"order_id"})
	builder.AddJoin(BuilderJoin{Table: orders, Base: customers, Column: "customer_id", BaseColumn: "id"})
	builder.AddJoin(BuilderJoin{Table: items, Base: orders, Column: "order_id", BaseColumn: "id"})

	builder.DropTable(orders)

	if len(builder.Tables) != 1 || builder.Tables[0].Ref.Name != "customers" {
		t.Fatalf("the builder holds %d tables", len(builder.Tables))
	}
	if len(builder.Joins) != 0 {
		t.Errorf("the builder holds %d joins", len(builder.Joins))
	}
}

// Dropping a leaf keeps the tables before it, and the joins that are left still name the
// tables they join.
func TestBuilderKeepsTheJoinsOfTheTablesItKeeps(t *testing.T) {
	builder := NewBuilder()
	customers := addTable(builder, "customers", []string{"id", "name"})
	orders := addTable(builder, "orders", []string{"id", "customer_id"})
	items := addTable(builder, "order_items", []string{"order_id"})
	builder.AddJoin(BuilderJoin{Table: orders, Base: customers, Column: "customer_id", BaseColumn: "id"})
	builder.AddJoin(BuilderJoin{Table: items, Base: orders, Column: "order_id", BaseColumn: "id"})
	builder.Tables[customers].Columns[1].Picked = true

	builder.DropTable(items)

	if len(builder.Tables) != 2 || len(builder.Joins) != 1 {
		t.Fatalf("the builder holds %d tables and %d joins",
			len(builder.Tables), len(builder.Joins))
	}
	join := builder.Joins[0]
	if join.Table != 1 || join.Base != 0 {
		t.Errorf("the join reads %+v", join)
	}
	if !strings.Contains(builder.BuildSQL(buildBuilderDialect()), "inner join") {
		t.Errorf("the builder wrote\n%s", builder.BuildSQL(buildBuilderDialect()))
	}
}

// The filters are joined into one where clause.
func TestBuilderWritesEveryFilterIntoTheWhere(t *testing.T) {
	builder := NewBuilder()
	orders := addTable(builder, "orders", []string{"id", "total"})
	builder.Tables[orders].Columns[0].Picked = true
	builder.Filters = []string{"o.total > 100", "o.total < 500"}

	written := builder.BuildSQL(buildBuilderDialect())
	if !strings.Contains(written, " where o.total > 100 and o.total < 500") {
		t.Errorf("the builder wrote\n%s", written)
	}
}

// A column with an aggregate groups by every other picked column.
func TestBuilderGroupsByTheColumnsWithoutAnAggregate(t *testing.T) {
	builder := NewBuilder()
	customers := addTable(builder, "customers", []string{"id", "name"})
	orders := addTable(builder, "orders", []string{"id", "customer_id"})
	builder.AddJoin(BuilderJoin{Table: orders, Base: customers, Column: "customer_id", BaseColumn: "id"})
	builder.Tables[customers].Columns[1].Picked = true
	builder.Tables[orders].Columns[0].Picked = true
	builder.Tables[orders].Columns[0].Aggregate = statement.AggregateCount
	builder.Tables[orders].Columns[0].As = "orders"

	written := builder.BuildSQL(buildBuilderDialect())
	for _, wanted := range []string{
		"select c.name, count(o.id) as orders", " group by c.name",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the builder wrote\n%s\nwanted %q in it", written, wanted)
		}
	}
}

// An empty builder writes nothing, and a table without a picked column reads every column.
func TestBuilderWritesEveryColumnUntilOneIsPicked(t *testing.T) {
	builder := NewBuilder()
	if written := builder.BuildSQL(buildBuilderDialect()); written != "" {
		t.Errorf("an empty builder wrote %q", written)
	}

	addTable(builder, "orders", []string{"id", "total"})
	if written := builder.BuildSQL(buildBuilderDialect()); !strings.HasPrefix(written, "select *") {
		t.Errorf("the builder wrote %q", written)
	}
}

// The title names the tables, so the tab row says what the builder holds.
func TestBuilderTitleNamesItsTables(t *testing.T) {
	builder := NewBuilder()
	if builder.DescribeTitle() != "builder" {
		t.Errorf("an empty builder is named %q", builder.DescribeTitle())
	}

	addTable(builder, "customers", []string{"id"})
	addTable(builder, "orders", []string{"id"})
	if builder.DescribeTitle() != "customers + orders" {
		t.Errorf("the builder is named %q", builder.DescribeTitle())
	}
}

// The builder of a tab is stored and read back: its tables, its aliases, its joins, its
// filters, and what was picked from each table.
func TestBuilderTabRoundTripsThroughTheWorkspace(t *testing.T) {
	connection := &Connection{Catalog: NewCatalog(), Unread: map[int]bool{}}
	tab := NewBuilderTab(1)
	connection.Tabs = []*Tab{tab}

	builder := tab.Builder
	customers := addTable(builder, "customers", []string{"id", "name"})
	orders := addTable(builder, "orders", []string{"id", "customer_id", "total"})
	builder.AddJoin(BuilderJoin{
		Kind: statement.JoinLeft, Table: orders, Base: customers,
		Column: "customer_id", BaseColumn: "id",
	})
	builder.Tables[customers].Columns[1].Picked = true
	builder.Tables[orders].Columns[2].Picked = true
	builder.Tables[orders].Columns[2].Aggregate = statement.AggregateSum
	builder.Tables[orders].Columns[2].As = "revenue"
	builder.Tables[orders].Columns[2].Sort = core.SortDescending
	builder.Filters = []string{"o.total > 0"}
	tab.PaneHeight = 21

	snapshot := connection.BuildWorkspaceSnapshot()
	restored := &Connection{Catalog: NewCatalog(), Unread: map[int]bool{}}
	restored.RestoreTabs(snapshot, func(db.TableRef) string { return "" })

	if len(restored.Tabs) != 1 || !restored.Tabs[0].BuildsQuery() {
		t.Fatalf("the workspace restored %d tabs", len(restored.Tabs))
	}
	held := restored.Tabs[0]
	if held.PaneHeight != 21 {
		t.Errorf("the pane height reads %d", held.PaneHeight)
	}
	if !restored.Unread[held.ID] {
		t.Error("the restored builder reads no column of its tables")
	}

	// The columns arrive from the server again, and the picks stand where they stood.
	for at, table := range held.Builder.Tables {
		if !table.Reading {
			t.Errorf("table %d waits for no columns", at)
		}
	}
	held.Builder.WriteColumns(0, db.TableDetail{Columns: []db.ColumnDetail{
		{Name: "id"}, {Name: "name"},
	}})
	held.Builder.WriteColumns(1, db.TableDetail{Columns: []db.ColumnDetail{
		{Name: "id"}, {Name: "customer_id"}, {Name: "total"},
	}})

	written := held.Builder.BuildSQL(buildBuilderDialect())
	wanted := `select c.name, sum(o.total) as revenue
  from "shop"."customers" c
  left join "shop"."orders" o on o.customer_id = c.id
 where o.total > 0
 group by c.name
 order by revenue desc`
	if written != wanted {
		t.Errorf("the restored builder wrote\n%s\nwanted\n%s", written, wanted)
	}
}

// A change to the builder changes the signature of the tabs, so the workspace is stored.
func TestBuilderChangeShowsInTheTabSignature(t *testing.T) {
	connection := &Connection{Catalog: NewCatalog(), Unread: map[int]bool{}}
	tab := NewBuilderTab(1)
	connection.Tabs = []*Tab{tab}
	addTable(tab.Builder, "orders", []string{"id", "total"})

	before := connection.DescribeTabs()
	tab.Builder.Tables[0].Columns[1].Picked = true
	if connection.DescribeTabs() == before {
		t.Error("a picked column left the signature unchanged")
	}

	before = connection.DescribeTabs()
	tab.Builder.Filters = []string{"o.total > 0"}
	if connection.DescribeTabs() == before {
		t.Error("a filter left the signature unchanged")
	}
}
