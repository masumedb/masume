package schemadiff_test

import (
	"context"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/postgres"
	"github.com/masumedb/masume/internal/schemadiff"
)

func writeScript(t *testing.T, source, target schemadiff.Snapshot, family core.Family) (schemadiff.Script, error) {
	t.Helper()
	return schemadiff.WriteScript(context.Background(), schemadiff.ScriptRequest{
		Source: source, Target: target, Family: family, Dialect: postgres.Dialect,
		SessionSchema: "public",
	})
}

func buildSnapshot(columns ...db.ColumnDetail) schemadiff.Snapshot {
	return schemadiff.Snapshot{Schema: "public", Tables: []schemadiff.TableSnapshot{{
		Table: db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}, Columns: columns,
	}}}
}

func TestScriptAltersEachPartOfAPostgresColumn(t *testing.T) {
	script, err := writeScript(t,
		buildSnapshot(db.ColumnDetail{Name: "total", DataType: "integer", Nullable: true}),
		buildSnapshot(db.ColumnDetail{
			Name: "total", DataType: "bigint", HasDefault: true, DefaultValue: "0",
		}),
		core.FamilyPostgres)
	if err != nil {
		t.Fatal(err)
	}
	wanted := "ALTER TABLE orders ALTER COLUMN total TYPE bigint;\n\n" +
		"ALTER TABLE orders ALTER COLUMN total SET NOT NULL;\n\n" +
		"ALTER TABLE orders ALTER COLUMN total SET DEFAULT 0;\n"
	if written := script.Write(); written != wanted {
		t.Errorf("the script reads %q, wanted %q", written, wanted)
	}
}

func TestScriptNotesAnIdentityItDoesNotWrite(t *testing.T) {
	script, err := writeScript(t,
		buildSnapshot(db.ColumnDetail{Name: "id", DataType: "integer"}),
		buildSnapshot(db.ColumnDetail{Name: "id", DataType: "integer", IsIdentityAlways: true}),
		core.FamilyPostgres)
	if err != nil {
		t.Fatal(err)
	}
	if written := script.Write(); !strings.HasPrefix(written,
		"-- not written: the identity or generation of orders.id\n") {
		t.Errorf("the script reads %q", written)
	}
}

func TestScriptWritesNothingForEqualSchemas(t *testing.T) {
	snapshot := buildSnapshot(db.ColumnDetail{Name: "id", DataType: "integer"})
	script, err := writeScript(t, snapshot, snapshot, core.FamilyPostgres)
	if err != nil {
		t.Fatal(err)
	}
	if written := script.Write(); written != "-- no differences\n" {
		t.Errorf("the script reads %q", written)
	}
}

func TestScriptRefusesAnotherFamily(t *testing.T) {
	snapshot := buildSnapshot()
	if _, err := writeScript(t, snapshot, snapshot, core.FamilySqlite); err == nil ||
		!strings.Contains(err.Error(), "PostgreSQL and MySQL families only") {
		t.Errorf("the script answered %v", err)
	}
}
