package statement_test

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/query/statement"
)

// buildTestDialect returns a dialect that quotes with double quotes, as PostgreSQL does.
func buildTestDialect() *query.Dialect {
	return &query.Dialect{
		Engine:          core.EnginePostgres,
		QuoteIdentifier: func(name string) string { return `"` + name + `"` },
	}
}

// buildShopPlan returns a plan over three joined tables.
func buildShopPlan() statement.QueryPlan {
	return statement.QueryPlan{
		Tables: []statement.PlanTable{
			{Name: query.QualifiedName{Schema: "shop", Name: "customers"}, Alias: "c"},
			{Name: query.QualifiedName{Schema: "shop", Name: "orders"}, Alias: "o"},
			{Name: query.QualifiedName{Schema: "shop", Name: "order_items"}, Alias: "i"},
		},
		Joins: []statement.PlanJoin{
			{Kind: statement.JoinInner, Table: 1, On: "o.customer_id = c.id"},
			{Kind: statement.JoinLeft, Table: 2, On: "i.order_id = o.id"},
		},
		Fields: []statement.PlanField{
			{Alias: "c", Column: "name"},
			{Alias: "o", Column: "total"},
		},
	}
}

func TestBuildQueryPlanSQLWritesTheJoins(t *testing.T) {
	written := statement.BuildQueryPlanSQL(buildShopPlan(), buildTestDialect())

	wanted := `select c.name, o.total
  from "shop"."customers" c
  inner join "shop"."orders" o on o.customer_id = c.id
  left join "shop"."order_items" i on i.order_id = o.id`
	if written != wanted {
		t.Errorf("the plan wrote\n%s\nwanted\n%s", written, wanted)
	}
}

// A plan with no picked column reads every column, so the first table alone is a valid query.
func TestBuildQueryPlanSQLReadsEveryColumnWithoutAField(t *testing.T) {
	plan := statement.QueryPlan{
		Tables: []statement.PlanTable{
			{Name: query.QualifiedName{Schema: "shop", Name: "orders"}, Alias: "o"},
		},
	}

	written := statement.BuildQueryPlanSQL(plan, buildTestDialect())
	if written != "select *\n  from \"shop\".\"orders\" o" {
		t.Errorf("the plan wrote %q", written)
	}
}

// An aggregate groups by every field without one.
func TestBuildQueryPlanSQLGroupsByTheFieldsWithoutAnAggregate(t *testing.T) {
	plan := buildShopPlan()
	plan.Fields = []statement.PlanField{
		{Alias: "c", Column: "name"},
		{Alias: "o", Column: "id", Aggregate: statement.AggregateCount, As: "orders",
			Sort: core.SortDescending},
		{Alias: "i", Column: "qty", Aggregate: statement.AggregateSum, As: "items"},
	}

	written := statement.BuildQueryPlanSQL(plan, buildTestDialect())
	for _, wanted := range []string{
		"select c.name, count(o.id) as orders, sum(i.qty) as items",
		" group by c.name",
		" order by orders desc",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the plan wrote\n%s\nwanted %q in it", written, wanted)
		}
	}
}

// A plan without an aggregate groups by nothing, whatever its fields are.
func TestBuildQueryPlanSQLGroupsNothingWithoutAnAggregate(t *testing.T) {
	written := statement.BuildQueryPlanSQL(buildShopPlan(), buildTestDialect())

	if strings.Contains(written, "group by") {
		t.Errorf("the plan wrote a group by:\n%s", written)
	}
}

func TestBuildQueryPlanSQLWritesTheFilterAndTheLimit(t *testing.T) {
	plan := buildShopPlan()
	plan.Where = "o.total > 100"
	plan.Limit = 200

	written := statement.BuildQueryPlanSQL(plan, buildTestDialect())
	for _, wanted := range []string{" where o.total > 100", " limit 200"} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the plan wrote\n%s\nwanted %q in it", written, wanted)
		}
	}
}

// A having clause follows the group by, because it reads the aggregates.
func TestBuildQueryPlanSQLWritesHavingAfterTheGrouping(t *testing.T) {
	plan := buildShopPlan()
	plan.Fields = []statement.PlanField{
		{Alias: "c", Column: "name"},
		{Alias: "o", Column: "id", Aggregate: statement.AggregateCount, As: "orders"},
	}
	plan.Having = "count(o.id) > 2"

	written := statement.BuildQueryPlanSQL(plan, buildTestDialect())
	group := strings.Index(written, " group by")
	having := strings.Index(written, "having")
	if group == -1 || having == -1 || having < group {
		t.Errorf("the plan wrote\n%s", written)
	}
}

// A table without an alias is written as it stands, so a plan of one table needs no alias.
func TestBuildQueryPlanSQLWritesATableWithoutAnAlias(t *testing.T) {
	plan := statement.QueryPlan{
		Tables: []statement.PlanTable{
			{Name: query.QualifiedName{Schema: "shop", Name: "orders"}},
		},
		Fields: []statement.PlanField{{Column: "id"}},
	}

	written := statement.BuildQueryPlanSQL(plan, buildTestDialect())
	if written != "select id\n  from \"shop\".\"orders\"" {
		t.Errorf("the plan wrote %q", written)
	}
}

// A plan with no table writes nothing, so an empty builder shows an empty preview.
func TestBuildQueryPlanSQLWritesNothingWithoutATable(t *testing.T) {
	if written := statement.BuildQueryPlanSQL(
		statement.QueryPlan{}, buildTestDialect()); written != "" {
		t.Errorf("an empty plan wrote %q", written)
	}
}

// A join that names a table the plan does not hold is left out, so a dropped table cannot
// write a broken statement.
func TestBuildQueryPlanSQLLeavesOutAJoinWithoutATable(t *testing.T) {
	plan := buildShopPlan()
	plan.Joins = append(plan.Joins, statement.PlanJoin{Kind: statement.JoinInner, Table: 7})

	written := statement.BuildQueryPlanSQL(plan, buildTestDialect())
	if strings.Count(written, "join") != 2 {
		t.Errorf("the plan wrote\n%s", written)
	}
}
