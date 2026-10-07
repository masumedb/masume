//go:build integration

package schemadiff_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/schemadiff"
)

const dropCompareSchemas = "drop schema if exists masume_v1 cascade; drop schema if exists masume_v2 cascade"

func TestServerComparesTwoSchemasWithoutTheirNames(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropCompareSchemas,
		"create schema masume_v1", "create schema masume_v2",
		"create table masume_v1.orders (id serial primary key, total integer not null check (total > 0))",
		"create index orders_total on masume_v1.orders (total)",
		"create table masume_v2.orders (id serial primary key, total bigint not null check (total > 0))",
		"create index orders_total on masume_v2.orders (total)")
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), dropCompareSchemas, dbtest.ReadEverything, nil)
	})

	ctx := context.Background()
	source, err := schemadiff.ReadSnapshot(ctx, session, "masume_v1")
	if err != nil {
		t.Fatal(err)
	}
	target, err := schemadiff.ReadSnapshot(ctx, session, "masume_v2")
	if err != nil {
		t.Fatal(err)
	}
	found := schemadiff.Compare(source, target)
	if len(found) != 1 {
		t.Fatalf("found %d differences, wanted the column type only: %+v", len(found), found)
	}
	if line := schemadiff.DescribeDifference(found[0]); line != "~ column total  integer not null -> bigint not null" {
		t.Errorf("the difference reads %q", line)
	}
}
