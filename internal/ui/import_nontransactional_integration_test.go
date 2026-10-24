//go:build integration

package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/query"
)

func TestImportWritesRowsWithoutTransaction(t *testing.T) {
	for _, server := range []struct {
		target dbtest.Target
		create string
	}{
		{dbtest.Clickhouse, "create table masume_import_plain (id Int64, note String) engine = MergeTree order by id"},
		{dbtest.Cassandra, "create table masume_import_plain (id bigint primary key, note text)"},
	} {
		t.Run(string(server.target.Engine), func(t *testing.T) {
			session := dbtest.Open(t, server.target)
			ctx := context.Background()
			dbtest.RunStatements(t, session, "drop table if exists masume_import_plain", server.create)
			t.Cleanup(func() {
				_, _ = session.RunQuery(ctx, "drop table if exists masume_import_plain", dbtest.ReadEverything, nil)
			})

			var file strings.Builder
			file.WriteString("id,note\n")
			for row := range 30 {
				fmt.Fprintf(&file, "%d,row %d\n", row, row)
			}
			plan := buildIntegrationImportPlan(t, session, file.String(), "masume_import_plain",
				[]load.TargetColumn{{Name: "id", DataType: "bigint"}, {Name: "note", DataType: "text"}})
			answered := runImport(app.NewConnection(session, nil, false), 1, plan, session.Dialect(), 0, nil)().(importRanMsg)
			if answered.Problem != "" || answered.Written != 30 {
				t.Fatalf("answer %+v", answered)
			}
			if count := countIntegrationRows(t, session, "masume_import_plain"); count != "30" {
				t.Fatalf("the table has %s rows", count)
			}
		})
	}
}

func TestImportIntoNewTableWithoutTransaction(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "drop table if exists masume_import_new")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "drop table if exists masume_import_new", dbtest.ReadEverything, nil)
	})

	plan := buildIntegrationImportPlan(t, session, "id,note\n1,one\n2,two\n", "masume_import_new", nil)
	answered := runImport(app.NewConnection(session, nil, false), 1, plan, session.Dialect(), 0, nil)().(importRanMsg)
	if answered.Problem != "" || answered.Written != 2 {
		t.Fatalf("answer %+v", answered)
	}
	if count := countIntegrationRows(t, session, "masume_import_new"); count != "2" {
		t.Fatalf("the new table has %s rows", count)
	}
}

func TestFailedImportWithoutTransactionLeavesNoNewTable(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "drop table if exists masume_import_new")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "drop table if exists masume_import_new", dbtest.ReadEverything, nil)
	})

	// The last amount has more digits than a Decimal(38, 10) column takes.
	var file strings.Builder
	file.WriteString("amount\n")
	for row := range load.SampleRows + 10 {
		fmt.Fprintf(&file, "%d.5\n", row)
	}
	file.WriteString(strings.Repeat("9", 33) + ".5\n")
	plan := buildIntegrationImportPlan(t, session, file.String(), "masume_import_new", nil)
	answered := runImport(app.NewConnection(session, nil, false), 1, plan, session.Dialect(), 0, nil)().(importRanMsg)
	if answered.Problem == "" {
		t.Fatal("the import wrote the long amount")
	}
	read, err := session.RunQuery(ctx, "exists table masume_import_new", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(read.Rows[0][0]) != "0" {
		t.Errorf("the failed import left the new table behind: %s", answered.Problem)
	}
}

func buildIntegrationImportPlan(
	t *testing.T, session db.Session, text, table string, target []load.TargetColumn,
) load.Plan {
	t.Helper()
	path := writeCSV(t, table+".csv", text)
	options := load.DefaultReadOptions()
	sample, err := load.ReadSample(path, options)
	if err != nil {
		t.Fatal(err)
	}
	return load.BuildPlan(path, options, sample,
		query.QualifiedName{Schema: session.Describe().DefaultSchema, Name: table}, target)
}

func countIntegrationRows(t *testing.T, session db.Session, table string) string {
	t.Helper()
	read, err := session.RunQuery(context.Background(), "select count(*) from "+table, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(read.Rows[0][0])
}
