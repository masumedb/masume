//go:build integration

package datasearch_test

import (
	"context"
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
