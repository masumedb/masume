//go:build integration

package engines_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestApplyChangesWritesANestedJSONValue(t *testing.T) {
	for _, server := range []struct {
		target dbtest.Target
		create string
	}{
		{dbtest.Postgres, "create table masume_json_value (id integer primary key, prefs jsonb, tags jsonb)"},
		{dbtest.MySQL, "create table masume_json_value (id integer primary key, prefs json, tags json)"},
		{dbtest.Sqlserver, "create table masume_json_value (id integer primary key, prefs nvarchar(max), tags nvarchar(max))"},
		{dbtest.Clickhouse, "create table masume_json_value (id Int64, prefs String, tags Array(Int64)) engine = MergeTree order by id"},
		{dbtest.Turso, "create table masume_json_value (id integer primary key, prefs json, tags json)"},
	} {
		t.Run(string(server.target.Engine), func(t *testing.T) {
			session := dbtest.Open(t, server.target)
			ctx := context.Background()
			dbtest.RunStatements(t, session, "drop table if exists masume_json_value", server.create)
			t.Cleanup(func() {
				_, _ = session.RunQuery(ctx, "drop table if exists masume_json_value", dbtest.ReadEverything, nil)
			})
			shape, err := session.RunQuery(ctx, "select * from masume_json_value", dbtest.ReadEverything, nil)
			if err != nil {
				t.Fatal(err)
			}

			pending := core.NewPendingChanges()
			pending.Inserts = append(pending.Inserts, map[string]any{
				"id": float64(1), "prefs": map[string]any{"a": []any{float64(1), float64(2)}},
				"tags": []any{float64(3), float64(4)},
			})
			changes, err := session.Composer().BuildChanges(db.ChangeTarget{
				Table:   db.TableRef{Schema: session.Describe().DefaultSchema, Name: "masume_json_value", Kind: db.RelationTable},
				Columns: shape.Columns,
			}, pending)
			if err != nil {
				t.Fatal(err)
			}
			if err := session.ApplyChanges(ctx, changes); err != nil {
				t.Fatalf("the insert failed: %v", err)
			}
			read, err := session.RunQuery(ctx, "select prefs, tags from masume_json_value", dbtest.ReadEverything, nil)
			if err != nil {
				t.Fatal(err)
			}
			prefs := strings.ReplaceAll(core.FormatCell(read.Rows[0][0], read.Columns[0].DataType), " ", "")
			if prefs != `{"a":[1,2]}` {
				t.Errorf("prefs reads %s", prefs)
			}
			if tags := strings.ReplaceAll(core.FormatCell(read.Rows[0][1], read.Columns[1].DataType), " ", ""); tags != "[3,4]" {
				t.Errorf("tags reads %s", tags)
			}
		})
	}
}

func TestApplyChangesBindsAListToACQLCollection(t *testing.T) {
	session := dbtest.Open(t, dbtest.Cassandra)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "drop table if exists masume_json_value",
		"create table masume_json_value (id text primary key, tags list<double>)")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "drop table if exists masume_json_value", dbtest.ReadEverything, nil)
	})

	pending := core.NewPendingChanges()
	pending.Inserts = append(pending.Inserts, map[string]any{
		"id": "one", "tags": []any{float64(3), float64(4)},
	})
	changes, err := session.Composer().BuildChanges(db.ChangeTarget{
		Table: db.TableRef{Schema: session.Describe().DefaultSchema, Name: "masume_json_value", Kind: db.RelationTable},
	}, pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the insert failed: %v", err)
	}
	read, err := session.RunQuery(ctx, "select tags from masume_json_value", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tags := fmt.Sprint(read.Rows[0][0]); tags != "[3 4]" {
		t.Errorf("tags reads %s", tags)
	}
}

func TestApplyChangesWritesAnObjectIntoAClickHouseMap(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "drop table if exists masume_json_value",
		"create table masume_json_value (id Int64, counts Map(String, Int64), names Map(String, String)) engine = MergeTree order by id")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "drop table if exists masume_json_value", dbtest.ReadEverything, nil)
	})
	shape, err := session.RunQuery(ctx, "select * from masume_json_value", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}

	pending := core.NewPendingChanges()
	pending.Inserts = append(pending.Inserts, map[string]any{
		"id":     float64(1),
		"counts": map[string]any{"a": float64(1), "b": float64(2)},
		"names":  map[string]any{"first": "it's"},
	})
	changes, err := session.Composer().BuildChanges(db.ChangeTarget{
		Table:   db.TableRef{Schema: session.Describe().DefaultSchema, Name: "masume_json_value", Kind: db.RelationTable},
		Columns: shape.Columns,
	}, pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the insert failed: %v", err)
	}
	read, err := session.RunQuery(ctx,
		"select counts['b'], names['first'] from masume_json_value", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count := fmt.Sprint(read.Rows[0][0]); count != "2" {
		t.Errorf("counts['b'] reads %s", count)
	}
	if name := fmt.Sprint(read.Rows[0][1]); name != "it's" {
		t.Errorf("names['first'] reads %s", name)
	}
}

func TestApplyChangesWritesAListIntoAPostgresArray(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "drop table if exists masume_json_value",
		"create table masume_json_value (id integer primary key, names text[], counts int[])")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "drop table if exists masume_json_value", dbtest.ReadEverything, nil)
	})
	shape, err := session.RunQuery(ctx, "select * from masume_json_value", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}

	pending := core.NewPendingChanges()
	pending.Inserts = append(pending.Inserts, map[string]any{
		"id":     float64(1),
		"names":  []any{"a", `say "hi"`, nil, "NULL"},
		"counts": []any{float64(3), nil, float64(4)},
	})
	changes, err := session.Composer().BuildChanges(db.ChangeTarget{
		Table:   db.TableRef{Schema: session.Describe().DefaultSchema, Name: "masume_json_value", Kind: db.RelationTable},
		Columns: shape.Columns,
	}, pending)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the insert failed: %v", err)
	}
	read, err := session.RunQuery(ctx,
		"select names::text, counts::text, names[3] is null from masume_json_value", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if names := fmt.Sprint(read.Rows[0][0]); names != `{a,"say \"hi\"",NULL,"NULL"}` {
		t.Errorf("names reads %s", names)
	}
	if counts := fmt.Sprint(read.Rows[0][1]); counts != "{3,NULL,4}" {
		t.Errorf("counts reads %s", counts)
	}
	if read.Rows[0][2] != true {
		t.Errorf("the third name is %v, wanted NULL", read.Rows[0][2])
	}
}
