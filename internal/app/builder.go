package app

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The query builder holds the tables, the joins and the picked columns of one flat select.
// It writes SQL and never reads it back.

// BuilderColumn is one column of a table of the builder.
type BuilderColumn struct {
	Name     string
	DataType string
	// Picked is true for a column of the select list.
	Picked    bool
	Aggregate statement.Aggregate
	// As is the name the result column takes.
	As string
	// Sort is the direction of this column in the order by, or empty.
	Sort core.SortDirection
}

// BuilderTable is one table of the builder, under the alias its SQL uses.
type BuilderTable struct {
	Ref         db.TableRef
	Alias       string
	Columns     []BuilderColumn
	ForeignKeys []query.ForeignKey
	// Reading is true while the server answers with the columns of the table.
	Reading bool
	Problem string
}

// BuilderJoin joins one table of the builder to a table already in it.
type BuilderJoin struct {
	Kind statement.JoinKind
	// Table is the table this join adds, and Base is the table it joins to. Both are
	// indexes into the tables of the builder.
	Table int
	Base  int
	// Columns and BaseColumns are the two sides of the condition, one pair per column of
	// the foreign key.
	Columns     []string
	BaseColumns []string
	// On is the condition the user wrote in place of the column pairs.
	On string
}

// BuilderSection is the part of the builder that has the cursor.
type BuilderSection string

// The sections the cursor moves through.
const (
	BuilderTables  BuilderSection = "tables"
	BuilderFilters BuilderSection = "filters"
)

// Builder is the state of one query builder tab.
type Builder struct {
	Tables  []BuilderTable
	Joins   []BuilderJoin
	Filters []string
	// Section, Table, Column and Row are the cursor.
	Section BuilderSection
	Table   int
	Column  int
	Row     int
	// Limit is the row limit written into the SQL. Zero writes none.
	Limit int
	// Offset is how far the pane has scrolled down its rows, and ColumnOffset how far the
	// diagram has scrolled along them.
	Offset       int
	ColumnOffset int
	// Rolled is true after the wheel moved the pane. The pane follows the cursor again at
	// the next move of it.
	Rolled bool
}

// NewBuilder returns an empty builder.
func NewBuilder() *Builder {
	return &Builder{Section: BuilderTables}
}

// IsEmpty is true for a builder that holds no table.
func (builder *Builder) IsEmpty() bool { return len(builder.Tables) == 0 }

// MoveCursor puts the cursor on that table and column, and the pane follows it again.
func (builder *Builder) MoveCursor(table, column int) {
	builder.Table, builder.Column, builder.Rolled = table, column, false
}

// Roll moves the pane the wheel turned, down its rows or along them.
func (builder *Builder) Roll(rows, columns int) {
	builder.Offset = max(builder.Offset+rows, 0)
	builder.ColumnOffset = max(builder.ColumnOffset+columns, 0)
	builder.Rolled = true
}

// ActiveTable returns the table the cursor stands in.
func (builder *Builder) ActiveTable() (*BuilderTable, bool) {
	if builder.Table < 0 || builder.Table >= len(builder.Tables) {
		return nil, false
	}
	return &builder.Tables[builder.Table], true
}

// ActiveColumn returns the column the cursor stands on.
func (builder *Builder) ActiveColumn() (*BuilderColumn, bool) {
	table, found := builder.ActiveTable()
	if !found || builder.Column < 0 || builder.Column >= len(table.Columns) {
		return nil, false
	}
	return &table.Columns[builder.Column], true
}

// AddTable appends a table with an alias of its own and puts the cursor in it.
func (builder *Builder) AddTable(ref db.TableRef) int {
	builder.Tables = append(builder.Tables, BuilderTable{
		Ref: ref, Alias: builder.buildAlias(ref.Name), Reading: true,
	})
	builder.Table, builder.Column = len(builder.Tables)-1, 0
	builder.Section = BuilderTables
	return builder.Table
}

// AddJoin joins the table at that index to a table already in the builder.
func (builder *Builder) AddJoin(join BuilderJoin) {
	builder.Joins = append(builder.Joins, join)
}

// FindJoinAt returns the join of that row of the joins section.
func (builder *Builder) FindJoinAt(row int) (BuilderJoin, bool) {
	if row < 0 || row >= len(builder.Joins) {
		return BuilderJoin{}, false
	}
	return builder.Joins[row], true
}

// FindJoin returns the join that adds that table.
func (builder *Builder) FindJoin(table int) (*BuilderJoin, bool) {
	for at := range builder.Joins {
		if builder.Joins[at].Table == table {
			return &builder.Joins[at], true
		}
	}
	return nil, false
}

// buildAlias returns a short alias the builder does not hold yet: the first letter of the
// name, then the first letters of its parts, then a letter with a number.
func (builder *Builder) buildAlias(name string) string {
	taken := map[string]bool{}
	for _, table := range builder.Tables {
		taken[table.Alias] = true
	}

	letters := strings.ToLower(strings.TrimLeft(name, "_"))
	if letters == "" {
		letters = "t"
	}
	candidates := []string{letters[:1]}
	initials := ""
	for _, part := range strings.Split(letters, "_") {
		if part != "" {
			initials += part[:1]
		}
	}
	if initials != "" {
		candidates = append(candidates, initials)
	}
	for _, candidate := range candidates {
		if !taken[candidate] {
			return candidate
		}
	}
	for number := 2; ; number++ {
		candidate := candidates[0] + strconv.Itoa(number)
		if !taken[candidate] {
			return candidate
		}
	}
}

// WriteColumns puts the columns of the server into the table of that index. A column the
// table already holds keeps what was picked for it, so a restored builder keeps its select
// list when the columns are read again.
func (builder *Builder) WriteColumns(table int, detail db.TableDetail) {
	if table < 0 || table >= len(builder.Tables) {
		return
	}
	held := &builder.Tables[table]
	picked := map[string]BuilderColumn{}
	for _, column := range held.Columns {
		if column.Picked {
			picked[strings.ToLower(column.Name)] = column
		}
	}

	held.Reading, held.Problem = false, ""
	held.ForeignKeys = detail.ForeignKeys
	held.Columns = make([]BuilderColumn, 0, len(detail.Columns))
	for _, column := range detail.Columns {
		written := BuilderColumn{Name: column.Name, DataType: column.DataType}
		if kept, found := picked[strings.ToLower(column.Name)]; found {
			written.Picked, written.Aggregate = true, kept.Aggregate
			written.As, written.Sort = kept.As, kept.Sort
		}
		held.Columns = append(held.Columns, written)
	}
}

// WriteProblem puts the reason the columns of a table did not arrive into that table.
func (builder *Builder) WriteProblem(table int, problem string) {
	if table < 0 || table >= len(builder.Tables) {
		return
	}
	builder.Tables[table].Reading = false
	builder.Tables[table].Problem = problem
}

// DropTable removes a table, every table joined through it, and the joins of both.
func (builder *Builder) DropTable(table int) {
	if table < 0 || table >= len(builder.Tables) {
		return
	}
	dropped := builder.collectDependents(table)

	kept := make([]BuilderTable, 0, len(builder.Tables))
	moved := make([]int, len(builder.Tables))
	for at, held := range builder.Tables {
		if dropped[at] {
			moved[at] = -1
			continue
		}
		moved[at] = len(kept)
		kept = append(kept, held)
	}

	joins := make([]BuilderJoin, 0, len(builder.Joins))
	for _, join := range builder.Joins {
		if dropped[join.Table] || dropped[join.Base] {
			continue
		}
		join.Table, join.Base = moved[join.Table], moved[join.Base]
		joins = append(joins, join)
	}

	builder.Tables, builder.Joins = kept, joins
	builder.Table = clampIndex(builder.Table, len(builder.Tables))
	builder.Column = 0
}

// collectDependents returns the table and every table joined through it.
func (builder *Builder) collectDependents(table int) map[int]bool {
	dropped := map[int]bool{table: true}
	for again := true; again; {
		again = false
		for _, join := range builder.Joins {
			if dropped[join.Base] && !dropped[join.Table] {
				dropped[join.Table], again = true, true
			}
		}
	}
	return dropped
}

// DropFilter removes the filter of that row.
func (builder *Builder) DropFilter(row int) {
	if row < 0 || row >= len(builder.Filters) {
		return
	}
	builder.Filters = append(builder.Filters[:row], builder.Filters[row+1:]...)
	builder.Row = clampIndex(builder.Row, len(builder.Filters))
	if len(builder.Filters) == 0 {
		builder.Section = BuilderTables
	}
}

// clampIndex keeps an index inside a list, and answers zero for an empty one.
func clampIndex(index, count int) int {
	if index >= count {
		index = count - 1
	}
	if index < 0 {
		return 0
	}
	return index
}

// CountPicked returns how many columns the select list holds.
func (builder *Builder) CountPicked() int {
	picked := 0
	for _, table := range builder.Tables {
		for _, column := range table.Columns {
			if column.Picked {
				picked++
			}
		}
	}
	return picked
}

// DescribeTitle returns the name of the tab: the tables it joins.
func (builder *Builder) DescribeTitle() string {
	if builder.IsEmpty() {
		return "builder"
	}
	names := make([]string, 0, len(builder.Tables))
	for _, table := range builder.Tables {
		names = append(names, table.Ref.Name)
	}
	return strings.Join(names, " + ")
}

// BuildPlan returns the plan of the builder, with every name written for that dialect.
func (builder *Builder) BuildPlan(dialect *query.Dialect) statement.QueryPlan {
	plan := statement.QueryPlan{Limit: builder.Limit}
	for _, table := range builder.Tables {
		plan.Tables = append(plan.Tables, statement.PlanTable{
			Name:  query.QualifiedName{Schema: table.Ref.Schema, Name: table.Ref.Name},
			Alias: table.Alias,
		})
	}
	for _, join := range builder.Joins {
		plan.Joins = append(plan.Joins, statement.PlanJoin{
			Kind: join.Kind, Table: join.Table,
			On: builder.describeJoinCondition(join, dialect),
		})
	}
	for at, table := range builder.Tables {
		for _, column := range table.Columns {
			if !column.Picked {
				continue
			}
			plan.Fields = append(plan.Fields, statement.PlanField{
				Alias: builder.Tables[at].Alias, Column: column.Name,
				Aggregate: column.Aggregate, As: column.As, Sort: column.Sort,
			})
		}
	}
	plan.Where = strings.Join(builder.Filters, " and ")
	return plan
}

// describeJoinCondition returns the condition of a join: the text the user wrote, or the
// two columns of the foreign key.
func (builder *Builder) describeJoinCondition(
	join BuilderJoin, dialect *query.Dialect,
) string {
	if written := strings.TrimSpace(join.On); written != "" {
		return written
	}
	if join.Table < 0 || join.Table >= len(builder.Tables) ||
		join.Base < 0 || join.Base >= len(builder.Tables) {
		return ""
	}
	pairs := make([]string, 0, len(join.Columns))
	for at := 0; at < min(len(join.Columns), len(join.BaseColumns)); at++ {
		pairs = append(pairs,
			BuildColumnReference(builder.Tables[join.Table].Alias, join.Columns[at], dialect)+
				" = "+BuildColumnReference(
				builder.Tables[join.Base].Alias, join.BaseColumns[at], dialect))
	}
	return strings.Join(pairs, " and ")
}

// BuildColumnReference writes one column with the alias of its table.
func BuildColumnReference(alias, column string, dialect *query.Dialect) string {
	if dialect == nil {
		return alias + "." + column
	}
	return dialect.QuoteIdentifierIfNeeded(alias) + "." +
		dialect.QuoteIdentifierIfNeeded(column)
}

// BuildSQL returns the statement of the builder.
func (builder *Builder) BuildSQL(dialect *query.Dialect) string {
	return statement.BuildQueryPlanSQL(builder.BuildPlan(dialect), dialect)
}

// FindForeignKeyJoin returns the join a foreign key between the new table and a table
// already in the builder proposes. It reads the keys of both sides.
func (builder *Builder) FindForeignKeyJoin(table int) (BuilderJoin, bool) {
	if table < 0 || table >= len(builder.Tables) {
		return BuilderJoin{}, false
	}
	added := builder.Tables[table]

	for base, held := range builder.Tables {
		if base == table {
			continue
		}
		// A key of the new table that refers to a table already in the builder.
		if key, found := findKeyTo(added.ForeignKeys, held.Ref); found {
			return BuilderJoin{
				Kind: statement.JoinInner, Table: table, Base: base,
				Columns: key.Columns, BaseColumns: key.TargetColumns,
			}, true
		}
		// A key of a table already in the builder that refers to the new table.
		if key, found := findKeyTo(held.ForeignKeys, added.Ref); found {
			return BuilderJoin{
				Kind: statement.JoinInner, Table: table, Base: base,
				Columns: key.TargetColumns, BaseColumns: key.Columns,
			}, true
		}
	}
	return BuilderJoin{}, false
}

// findKeyTo returns the first key that refers to that table.
func findKeyTo(keys []query.ForeignKey, target db.TableRef) (query.ForeignKey, bool) {
	for _, key := range keys {
		if strings.EqualFold(key.TargetTable, target.Name) &&
			(key.TargetSchema == "" || strings.EqualFold(key.TargetSchema, target.Schema)) {
			return key, true
		}
	}
	return query.ForeignKey{}, false
}
