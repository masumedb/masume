//go:build integration

package ui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/query"
)

func TestFailedImportLeavesNoNewTableOnMySQL(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "drop table if exists masume_import_ddl")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "drop table if exists masume_import_ddl", dbtest.ReadEverything, nil)
	})

	// The first batch fits, and a text longer than a MySQL text column fails the second one.
	var file strings.Builder
	file.WriteString("note\n")
	for row := range load.BatchRows + 10 {
		file.WriteString("row " + strconv.Itoa(row) + "\n")
	}
	file.WriteString(strings.Repeat("x", 70000) + "\n")
	path := writeCSV(t, "masume_import_ddl.csv", file.String())

	options := load.DefaultReadOptions()
	sample, err := load.ReadSample(path, options)
	if err != nil {
		t.Fatal(err)
	}
	plan := load.BuildPlan(path, options, sample,
		query.QualifiedName{Schema: session.Describe().DefaultSchema, Name: "masume_import_ddl"}, nil)
	connection := app.NewConnection(session, nil, false)

	answered := runImport(connection, 1, plan, session.Dialect(), 0, nil)().(importRanMsg)
	if answered.Problem == "" {
		t.Fatal("the import wrote the long text")
	}
	if !strings.Contains(answered.Problem, "too long") {
		t.Fatalf("the import failed with %s", answered.Problem)
	}
	read, err := session.RunQuery(ctx,
		"select count(*) from information_schema.tables where table_name = 'masume_import_ddl'",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count := strconv.FormatInt(read.Rows[0][0].(int64), 10); count != "0" {
		t.Errorf("the failed import left the new table behind: %s", answered.Problem)
	}
}
