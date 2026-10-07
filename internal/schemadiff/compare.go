package schemadiff

import (
	"cmp"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/db"
)

// Change is the kind of one difference.
type Change string

const (
	ChangeAdded   Change = "added"
	ChangeRemoved Change = "removed"
	ChangeAltered Change = "changed"
)

// Part is the catalog item of one difference.
type Part string

const (
	PartTable      Part = "table"
	PartColumn     Part = "column"
	PartIndex      Part = "index"
	PartConstraint Part = "constraint"
)

// Difference is one item that is in the target and not in the source, the other way, or in
// both with a different definition. Source and Target are empty where the item is absent.
type Difference struct {
	Table  string
	Part   Part
	Name   string
	Change Change
	Source string
	Target string
}

// Compare returns the differences from source to target, by table, then part, then name.
func Compare(source, target Snapshot) []Difference {
	found := []Difference{}
	targets := map[string]TableSnapshot{}
	for _, table := range target.Tables {
		targets[table.Table.Name] = table
	}
	sources := map[string]bool{}
	for _, from := range source.Tables {
		sources[from.Table.Name] = true
		to, held := targets[from.Table.Name]
		if !held {
			found = append(found, Difference{
				Table: from.Table.Name, Part: PartTable, Name: from.Table.Name,
				Change: ChangeRemoved, Source: string(from.Table.Kind),
			})
			continue
		}
		found = append(found, compareTables(from, to)...)
	}
	for _, to := range target.Tables {
		if !sources[to.Table.Name] {
			found = append(found, Difference{
				Table: to.Table.Name, Part: PartTable, Name: to.Table.Name,
				Change: ChangeAdded, Target: string(to.Table.Kind),
			})
		}
	}
	sortDifferences(found)
	return found
}

func compareTables(from, to TableSnapshot) []Difference {
	table := from.Table.Name
	found := []Difference{}
	if from.Table.Kind != to.Table.Kind {
		found = append(found, Difference{
			Table: table, Part: PartTable, Name: table, Change: ChangeAltered,
			Source: string(from.Table.Kind), Target: string(to.Table.Kind),
		})
	}
	found = append(found, compareItems(table, PartColumn,
		describeColumns(from.Columns), describeColumns(to.Columns))...)
	found = append(found, compareItems(table, PartIndex,
		describeIndexes(from.Indexes), describeIndexes(to.Indexes))...)
	found = append(found, compareItems(table, PartConstraint,
		describeConstraints(from.Constraints), describeConstraints(to.Constraints))...)
	return found
}

// item is a named catalog item and its definition text.
type item struct {
	name       string
	definition string
}

func compareItems(table string, part Part, from, to []item) []Difference {
	found := []Difference{}
	targets := map[string]string{}
	for _, held := range to {
		targets[held.name] = held.definition
	}
	sources := map[string]bool{}
	for _, held := range from {
		sources[held.name] = true
		definition, present := targets[held.name]
		switch {
		case !present:
			found = append(found, Difference{
				Table: table, Part: part, Name: held.name, Change: ChangeRemoved,
				Source: held.definition,
			})
		case definition != held.definition:
			found = append(found, Difference{
				Table: table, Part: part, Name: held.name, Change: ChangeAltered,
				Source: held.definition, Target: definition,
			})
		}
	}
	for _, held := range to {
		if !sources[held.name] {
			found = append(found, Difference{
				Table: table, Part: part, Name: held.name, Change: ChangeAdded,
				Target: held.definition,
			})
		}
	}
	return found
}

func describeColumns(columns []db.ColumnDetail) []item {
	described := make([]item, 0, len(columns))
	for _, column := range columns {
		described = append(described, item{name: column.Name, definition: DescribeColumn(column)})
	}
	return described
}

// DescribeColumn returns the column definition as one line: type, nullability and default.
func DescribeColumn(column db.ColumnDetail) string {
	parts := []string{column.DataType}
	if !column.Nullable {
		parts = append(parts, "not null")
	}
	if column.HasDefault {
		parts = append(parts, "default "+column.DefaultValue)
	}
	if column.IsPrimaryKey {
		parts = append(parts, "primary key")
	}
	if column.IsGenerated {
		parts = append(parts, "generated")
	}
	if column.IsIdentityAlways {
		parts = append(parts, "identity")
	}
	return strings.Join(parts, " ")
}

func describeIndexes(indexes []db.IndexDetail) []item {
	described := make([]item, 0, len(indexes))
	for _, index := range indexes {
		described = append(described, item{name: index.Name, definition: index.Definition})
	}
	return described
}

func describeConstraints(constraints []db.ConstraintDetail) []item {
	described := make([]item, 0, len(constraints))
	for _, constraint := range constraints {
		described = append(described, item{
			name: constraint.Name, definition: string(constraint.Kind) + " " + constraint.Definition,
		})
	}
	return described
}

var partOrder = map[Part]int{PartTable: 0, PartColumn: 1, PartIndex: 2, PartConstraint: 3}

func sortDifferences(found []Difference) {
	slices.SortStableFunc(found, func(one, other Difference) int {
		if order := strings.Compare(one.Table, other.Table); order != 0 {
			return order
		}
		if order := cmp.Compare(partOrder[one.Part], partOrder[other.Part]); order != 0 {
			return order
		}
		return strings.Compare(one.Name, other.Name)
	})
}
