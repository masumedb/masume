package schemadiff_test

import (
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/schemadiff"
)

func buildOrders(columns ...db.ColumnDetail) schemadiff.TableSnapshot {
	return schemadiff.TableSnapshot{
		Table:   db.TableRef{Name: "orders", Kind: db.RelationTable},
		Columns: columns,
	}
}

func TestCompareFindsNothingInEqualSchemas(t *testing.T) {
	table := buildOrders(db.ColumnDetail{Name: "id", DataType: "integer"})
	found := schemadiff.Compare(
		schemadiff.Snapshot{Tables: []schemadiff.TableSnapshot{table}},
		schemadiff.Snapshot{Tables: []schemadiff.TableSnapshot{table}})
	if len(found) != 0 {
		t.Errorf("equal schemas differ: %+v", found)
	}
	if text := schemadiff.WriteReport(found); text != "no differences\n" {
		t.Errorf("the report reads %q", text)
	}
}

func TestCompareFindsEachKindOfChange(t *testing.T) {
	source := schemadiff.Snapshot{Tables: []schemadiff.TableSnapshot{
		{
			Table: db.TableRef{Name: "orders", Kind: db.RelationTable},
			Columns: []db.ColumnDetail{
				{Name: "id", DataType: "integer", IsPrimaryKey: true},
				{Name: "total", DataType: "integer"},
				{Name: "note", DataType: "text", Nullable: true},
			},
			Indexes: []db.IndexDetail{{Name: "orders_total", Definition: "(total)"}},
		},
		{Table: db.TableRef{Name: "archive", Kind: db.RelationTable}},
	}}
	target := schemadiff.Snapshot{Tables: []schemadiff.TableSnapshot{
		{
			Table: db.TableRef{Name: "orders", Kind: db.RelationTable},
			Columns: []db.ColumnDetail{
				{Name: "id", DataType: "integer", IsPrimaryKey: true},
				{Name: "total", DataType: "bigint"},
				{Name: "placed_at", DataType: "timestamp", Nullable: true},
			},
			Constraints: []db.ConstraintDetail{
				{Name: "orders_total_check", Kind: db.ConstraintCheck, Definition: "(total > 0)"},
			},
		},
		{Table: db.TableRef{Name: "customers", Kind: db.RelationView}},
	}}

	found := schemadiff.Compare(source, target)
	wanted := []string{
		"archive: - table",
		"customers: + view",
		"orders: - column note  text",
		"orders: + column placed_at  timestamp",
		"orders: ~ column total  integer not null -> bigint not null",
		"orders: - index orders_total  (total)",
		"orders: + constraint orders_total_check  check (total > 0)",
	}
	if len(found) != len(wanted) {
		t.Fatalf("found %d differences, wanted %d: %+v", len(found), len(wanted), found)
	}
	for at, difference := range found {
		if line := difference.Table + ": " + schemadiff.DescribeDifference(difference); line != wanted[at] {
			t.Errorf("difference %d reads %q, wanted %q", at, line, wanted[at])
		}
	}
}

func TestReportGroupsTheLinesByTable(t *testing.T) {
	found := []schemadiff.Difference{
		{Table: "a", Part: schemadiff.PartColumn, Name: "x", Change: schemadiff.ChangeAdded, Target: "int"},
		{Table: "b", Part: schemadiff.PartTable, Name: "b", Change: schemadiff.ChangeRemoved, Source: "table"},
	}
	wanted := "a\n  + column x  int\n\nb\n  - table\n"
	if text := schemadiff.WriteReport(found); text != wanted {
		t.Errorf("the report reads %q, wanted %q", text, wanted)
	}
}
