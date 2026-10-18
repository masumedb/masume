//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestGuardedReadWritesNothingThroughARoutine(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	ctx := context.Background()
	dbtest.RunStatements(t, session,
		"drop table if exists masume_purged",
		"create table masume_purged (id int)",
		"create or replace function masume_purge() returns int language sql as "+
			"$$ insert into masume_purged values (1) returning 1 $$")
	t.Cleanup(func() {
		_, _ = session.RunQuery(ctx,
			"drop function if exists masume_purge(); drop table if exists masume_purged",
			dbtest.ReadEverything, nil)
	})

	for _, sql := range []string{
		"select masume_purge()",
		"commit; select masume_purge()",
		"start transaction; select masume_purge()",
	} {
		if _, err := db.RunGuardedRead(ctx, session, sql,
			func(running context.Context) (db.QueryResult, error) {
				return session.RunQuery(running, sql, dbtest.ReadEverything, nil)
			}); err == nil {
			t.Errorf("%s ran as a read", sql)
		}
	}
	if written := readText(t, session, "select count(*) from masume_purged"); written != "0" {
		t.Errorf("the routine wrote %s rows", written)
	}
}
