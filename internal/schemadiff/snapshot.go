package schemadiff

import (
	"context"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/db"
)

// Snapshot is the catalog of one schema, sorted by table name.
type Snapshot struct {
	Schema string
	Tables []TableSnapshot
}

// TableSnapshot is one relation with its columns, indexes and constraints.
type TableSnapshot struct {
	Table       db.TableRef
	Columns     []db.ColumnDetail
	Indexes     []db.IndexDetail
	Constraints []db.ConstraintDetail
}

// ReadSnapshot reads every relation of one schema. A view has no indexes or constraints.
func ReadSnapshot(ctx context.Context, catalog db.CatalogReader, schema string) (Snapshot, error) {
	tables, err := catalog.ListTables(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{Schema: schema}
	for _, table := range tables {
		if table.Schema != schema {
			continue
		}
		read, readErr := readTable(ctx, catalog, table)
		if readErr != nil {
			return Snapshot{}, readErr
		}
		snapshot.Tables = append(snapshot.Tables, stripSchema(read, schema))
	}
	slices.SortFunc(snapshot.Tables, func(one, other TableSnapshot) int {
		return strings.Compare(one.Table.Name, other.Table.Name)
	})
	return snapshot, nil
}

func readTable(ctx context.Context, catalog db.CatalogReader, table db.TableRef) (TableSnapshot, error) {
	detail, err := catalog.DescribeTable(ctx, table)
	if err != nil {
		return TableSnapshot{}, err
	}
	read := TableSnapshot{Table: table, Columns: detail.Columns}
	if table.Kind != db.RelationTable {
		return read, nil
	}
	if read.Indexes, err = catalog.ListIndexes(ctx, table); err != nil {
		return TableSnapshot{}, err
	}
	if read.Constraints, err = catalog.ListConstraints(ctx, table); err != nil {
		return TableSnapshot{}, err
	}
	return read, nil
}

// stripSchema removes the schema qualifier from defaults and definitions.
func stripSchema(table TableSnapshot, schema string) TableSnapshot {
	if schema == "" {
		return table
	}
	qualifiers := []string{schema + ".", `"` + schema + `".`, "`" + schema + "`.", "[" + schema + "]."}
	strip := func(text string) string {
		for _, qualifier := range qualifiers {
			text = strings.ReplaceAll(text, qualifier, "")
		}
		return text
	}
	columns := make([]db.ColumnDetail, len(table.Columns))
	for at, column := range table.Columns {
		column.DefaultValue = strip(column.DefaultValue)
		columns[at] = column
	}
	indexes := make([]db.IndexDetail, len(table.Indexes))
	for at, index := range table.Indexes {
		index.Definition = strip(index.Definition)
		indexes[at] = index
	}
	constraints := make([]db.ConstraintDetail, len(table.Constraints))
	for at, constraint := range table.Constraints {
		constraint.Definition = strip(constraint.Definition)
		constraints[at] = constraint
	}
	table.Columns, table.Indexes, table.Constraints = columns, indexes, constraints
	return table
}
