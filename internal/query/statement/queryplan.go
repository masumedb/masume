package statement

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// A query plan is one flat select: tables joined on their keys, picked columns, filters and
// a sort. The query builder holds the plan and this file writes the SQL of it.

// JoinKind is the kind of one join.
type JoinKind string

// The kinds a join takes.
const (
	JoinInner JoinKind = "inner"
	JoinLeft  JoinKind = "left"
	JoinRight JoinKind = "right"
	JoinFull  JoinKind = "full"
	// JoinCross is the join of a table with no condition.
	JoinCross JoinKind = "cross"
)

// JoinKinds lists the kinds, in the order the card steps through them.
var JoinKinds = []JoinKind{JoinInner, JoinLeft, JoinRight, JoinFull}

// Aggregate is the function over a picked column.
type Aggregate string

// The aggregates a column takes. AggregateNone writes the column itself.
const (
	AggregateNone  Aggregate = ""
	AggregateCount Aggregate = "count"
	AggregateSum   Aggregate = "sum"
	AggregateAvg   Aggregate = "avg"
	AggregateMin   Aggregate = "min"
	AggregateMax   Aggregate = "max"
)

// Aggregates lists the aggregates, in the order the card steps through them.
var Aggregates = []Aggregate{
	AggregateNone, AggregateCount, AggregateSum, AggregateAvg, AggregateMin, AggregateMax,
}

// PlanTable is one table of the plan, under the alias the SQL uses for it.
type PlanTable struct {
	Name  query.QualifiedName
	Alias string
}

// PlanField is one picked column of the plan.
type PlanField struct {
	Alias     string
	Column    string
	Aggregate Aggregate
	// As is the name the result column takes, and it is empty for a column that keeps its
	// own name.
	As string
	// Sort is the direction this field is ordered by, and it is empty for a field the
	// order by leaves out.
	Sort core.SortDirection
}

// PlanJoin joins one table to a table already in the plan.
type PlanJoin struct {
	Kind JoinKind
	// Table is the table this join adds, by its index in the plan.
	Table int
	// On is the condition text, already written with the aliases of the plan.
	On string
}

// QueryPlan is the whole select the builder writes.
type QueryPlan struct {
	Tables []PlanTable
	Joins  []PlanJoin
	Fields []PlanField
	// Where is the predicate text, already written with the aliases of the plan.
	Where string
	// Having is the predicate over the aggregates.
	Having string
	Limit  int
}

// IsEmpty is true for a plan that names no table.
func (plan QueryPlan) IsEmpty() bool { return len(plan.Tables) == 0 }

// GroupsRows is true for a plan with an aggregate, which groups by every other field.
func (plan QueryPlan) GroupsRows() bool {
	for _, field := range plan.Fields {
		if field.Aggregate != AggregateNone {
			return true
		}
	}
	return false
}

// BuildQueryPlanSQL writes the select of the plan. A plan with no table returns an empty
// string, and a plan with no field reads every column.
func BuildQueryPlanSQL(plan QueryPlan, dialect *query.Dialect) string {
	if plan.IsEmpty() {
		return ""
	}

	lines := []string{"select " + buildPlanFields(plan, dialect)}
	lines = append(lines, "  from "+buildPlanTable(plan.Tables[0], dialect))
	for _, join := range plan.Joins {
		if join.Table < 0 || join.Table >= len(plan.Tables) {
			continue
		}
		written := strings.TrimSpace(join.On)
		// A join with no condition is a cross join. Every other kind needs one.
		kind := JoinCross
		if written != "" {
			kind = join.Kind
			if kind == "" {
				kind = JoinInner
			}
		}
		clause := "  " + string(kind) + " join " +
			buildPlanTable(plan.Tables[join.Table], dialect)
		if written != "" {
			clause += " on " + written
		}
		lines = append(lines, clause)
	}
	if written := strings.TrimSpace(plan.Where); written != "" {
		lines = append(lines, " where "+written)
	}
	if grouped := buildPlanGrouping(plan, dialect); grouped != "" {
		lines = append(lines, " group by "+grouped)
	}
	if written := strings.TrimSpace(plan.Having); written != "" {
		lines = append(lines, "having "+written)
	}
	if sorted := buildPlanSort(plan, dialect); sorted != "" {
		lines = append(lines, " order by "+sorted)
	}
	if plan.Limit > 0 {
		lines = append(lines, " "+dialect.BuildPageWindow(plan.Limit, 0))
	}
	return strings.Join(lines, "\n")
}

// buildPlanTable writes one table with its alias.
func buildPlanTable(table PlanTable, dialect *query.Dialect) string {
	written := dialect.BuildQualifiedName(table.Name)
	if table.Alias == "" {
		return written
	}
	return written + " " + dialect.QuoteIdentifierIfNeeded(table.Alias)
}

// buildPlanFields writes the select list. A plan with no picked column reads every column.
func buildPlanFields(plan QueryPlan, dialect *query.Dialect) string {
	if len(plan.Fields) == 0 {
		return "*"
	}
	written := make([]string, 0, len(plan.Fields))
	for _, field := range plan.Fields {
		text := buildPlanField(field, dialect)
		if field.As != "" {
			text += " as " + dialect.QuoteIdentifierIfNeeded(field.As)
		}
		written = append(written, text)
	}
	return strings.Join(written, ", ")
}

// buildPlanField writes one field, with the aggregate over it where it has one.
func buildPlanField(field PlanField, dialect *query.Dialect) string {
	written := buildFieldColumn(field, dialect)
	if field.Aggregate == AggregateNone {
		return written
	}
	return string(field.Aggregate) + "(" + written + ")"
}

// buildFieldColumn writes the column of a field, with its table alias.
func buildFieldColumn(field PlanField, dialect *query.Dialect) string {
	if field.Column == "*" {
		if field.Alias == "" {
			return "*"
		}
		return dialect.QuoteIdentifierIfNeeded(field.Alias) + ".*"
	}
	written := dialect.QuoteIdentifierIfNeeded(field.Column)
	if field.Alias == "" {
		return written
	}
	return dialect.QuoteIdentifierIfNeeded(field.Alias) + "." + written
}

// buildPlanGrouping writes the group by of a plan with an aggregate: every field without
// one. A plan without an aggregate groups by nothing.
func buildPlanGrouping(plan QueryPlan, dialect *query.Dialect) string {
	if !plan.GroupsRows() {
		return ""
	}
	written := []string{}
	for _, field := range plan.Fields {
		if field.Aggregate != AggregateNone {
			continue
		}
		written = append(written, buildFieldColumn(field, dialect))
	}
	return strings.Join(written, ", ")
}

// buildPlanSort writes the order by of the fields that carry a direction. A sorted field
// with a name of its own is ordered by that name, which every server reads.
func buildPlanSort(plan QueryPlan, dialect *query.Dialect) string {
	written := []string{}
	for _, field := range plan.Fields {
		if field.Sort == "" {
			continue
		}
		text := buildPlanField(field, dialect)
		if field.As != "" {
			text = dialect.QuoteIdentifierIfNeeded(field.As)
		}
		written = append(written, text+" "+string(field.Sort))
	}
	return strings.Join(written, ", ")
}
