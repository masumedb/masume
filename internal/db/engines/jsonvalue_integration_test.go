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

			pending := core.NewPendingChanges()
			pending.Inserts = append(pending.Inserts, map[string]any{
				"id": float64(1), "prefs": map[string]any{"a": []any{float64(1), float64(2)}},
				"tags": []any{float64(3), float64(4)},
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
