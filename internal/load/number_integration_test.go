//go:build integration

package load_test

import (
	"context"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/query"
)

func TestImportKeepsEveryDigitOfADecimal(t *testing.T) {
	for _, held := range []struct {
		target dbtest.Target
		schema string
		create string
	}{
		{dbtest.Postgres, "public", "create table load_digits (n numeric)"},
		{dbtest.Cockroach, "public", "create table load_digits (n decimal)"},
		{dbtest.MySQL, "shop", "create table load_digits (n decimal(38,10))"},
		{dbtest.Sqlserver, "dbo", "create table load_digits (n decimal(38,10))"},
		{dbtest.Clickhouse, "shop", "create table load_digits (n Decimal(38, 10)) engine = MergeTree order by tuple()"},
		{dbtest.Cassandra, "masume_test", "create table load_digits (n decimal primary key)"},
	} {
		t.Run(string(held.target.Engine), func(t *testing.T) {
			session := dbtest.Open(t, held.target)
			ctx := context.Background()
			_, _ = session.RunQuery(ctx, "drop table if exists load_digits", dbtest.ReadEverything, nil)
			dbtest.RunStatements(t, session, held.create)
			t.Cleanup(func() {
				_, _ = session.RunQuery(ctx, "drop table load_digits", dbtest.ReadEverything, nil)
			})

			plan := load.Plan{
				Table:    query.QualifiedName{Schema: held.schema, Name: "load_digits"},
				Sample:   load.Sample{Columns: []load.SourceColumn{{Name: "n"}}},
				Mappings: []load.Mapping{{Source: "n", Target: "n", Kind: core.KindNumber}},
			}
			rows, err := load.BuildRows(plan, []load.Row{{Line: 1, Values: []any{"12345678901234567.25"}}})
			if err != nil {
				t.Fatalf("the rows answered %v", err)
			}
			statement, err := load.BuildInsert(plan, rows, session.Dialect())
			if err != nil {
				t.Fatalf("the insert answered %v", err)
			}
			if _, err := session.RunQuery(ctx, statement.SQL, 1, statement.Params); err != nil {
				t.Fatalf("the insert ran with %v", err)
			}

			answered, err := session.RunQuery(ctx, "select n from load_digits", dbtest.ReadEverything, nil)
			if err != nil {
				t.Fatalf("the read answered %v", err)
			}
			written := core.FormatCell(answered.Rows[0][0], answered.Columns[0].DataType)
			if strings.TrimRight(written, "0") != "12345678901234567.25" {
				t.Errorf("the decimal reads %s", written)
			}
		})
	}
}
