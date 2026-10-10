//go:build integration

package datasearch_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/masumedb/masume/internal/datasearch"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestServerSearchesEveryTableOfTheSchema(t *testing.T) {
	for _, held := range []struct {
		target dbtest.Target
		create string
	}{
		{dbtest.Postgres, "create table masume_search (id int primary key, name text, born date)"},
		{dbtest.MySQL, "create table masume_search (id int primary key, name varchar(20), born date)"},
		{dbtest.Sqlserver, "create table masume_search (id int primary key, name nvarchar(20), born date)"},
		{dbtest.Clickhouse, "create table masume_search (id Int32, name String, born Date) engine = MergeTree order by id"},
	} {
		t.Run(string(held.target.Engine), func(t *testing.T) {
			session := dbtest.Open(t, held.target)
			ctx := context.Background()
			dbtest.RunStatements(t, session, "drop table if exists masume_search", held.create,
				"insert into masume_search values (1, 'Ada', '1985-12-10'), (2, 'Alan', '1992-06-23')")
			t.Cleanup(func() {
				_, _ = session.RunQuery(ctx, "drop table if exists masume_search", dbtest.ReadEverything, nil)
			})

			for _, text := range []string{"ada", "1992"} {
				report, err := datasearch.Search(ctx, session, session.Describe().DefaultSchema, text, 100)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, match := range report.Matches {
					found = found || (match.Table.Name == "masume_search" && match.Rows == 1)
				}
				if !found || len(report.Failed) != 0 {
					t.Errorf("%q: %+v", text, report)
				}
			}
		})
	}
}

func TestMongoSearchesTheStringFieldsOfEveryCollection(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)
	ctx := context.Background()
	dbtest.RunStatements(t, session, "db.masume_search.drop()",
		`db.masume_search.insertMany([{name: "Ada", tags: ["math"], born: 1815}, `+
			`{name: "Alan", tags: ["code", "a.b"], born: 1912}])`)
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx, "db.masume_search.drop()", dbtest.ReadEverything, nil)
	})

	for text, wanted := range map[string]int{"ADA": 1, "a.b": 1, "a_b": 1, "a%n": 1, "1912": 0} {
		report, err := datasearch.Search(ctx, session, session.Describe().DefaultSchema, text, 100)
		if err != nil {
			t.Fatal(err)
		}
		rows := 0
		for _, match := range report.Matches {
			if match.Table.Name == "masume_search" {
				rows = match.Rows
				read, readErr := session.RunQuery(ctx,
					`db.masume_search.countDocuments(`+match.Filter+`)`, dbtest.ReadEverything, nil)
				if readErr != nil || len(read.Rows) != 1 || fmt.Sprint(read.Rows[0][0]) != fmt.Sprint(wanted) {
					t.Errorf("%q: the filter %s counts %v: %v", text, match.Filter, read.Rows, readErr)
				}
			}
		}
		if rows != wanted || len(report.Failed) != 0 {
			t.Errorf("%q: %+v", text, report)
		}
	}
}
