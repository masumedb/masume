//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// largeRead returns far more rows than a row limit takes. A cross join of two small series
// streams its rows; one large series is built in full before the first row.
const largeRead = "select a.g * 10000 + b.g as g, repeat('x', 200) as filler " +
	"from generate_series(1, 10000) as a(g) cross join generate_series(1, 10000) as b(g)"

func TestARowLimitStopsALargeRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	// The plan of largeRead costs more than jit_above_cost, so the server compiles it with
	// JIT before the first row.
	dbtest.RunStatements(t, session, "set jit = off")
	runs := map[string]func(context.Context) (db.QueryResult, error){
		"a plain read": func(ctx context.Context) (db.QueryResult, error) {
			return session.RunQuery(ctx, largeRead, 100, nil)
		},
		"a guarded read": func(ctx context.Context) (db.QueryResult, error) {
			return db.RunGuardedRead(ctx, session, largeRead,
				func(running context.Context) (db.QueryResult, error) {
					return session.RunQuery(running, largeRead, 100, nil)
				})
		},
	}
	for name, run := range runs {
		t.Run(name, func(t *testing.T) {
			ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
			defer stop()

			started := time.Now()
			read, err := run(ctx)
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("the read answered %v after %s", err, elapsed)
			}
			if elapsed > 5*time.Second {
				t.Errorf("the read took %s", elapsed)
			}
			if len(read.Rows) != 100 || !read.Truncated {
				t.Errorf("the read has %d rows, truncated %v", len(read.Rows), read.Truncated)
			}
			if state := session.ReadTransactionState(); state != db.TransactionNone {
				t.Errorf("the transaction state after the read is %v", state)
			}
			if after := readText(t, session, "select 42"); after != "42" {
				t.Errorf("the next statement read %q", after)
			}
		})
	}
}

func TestARowLimitKeepsAWritingTransactionOpen(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	ctx := context.Background()
	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.RollbackTransaction(ctx) })
	dbtest.RunStatements(t, session, "create temporary table masume_kept (id int)",
		"insert into masume_kept values (1)")

	read, err := session.RunQuery(ctx,
		"select a.g from generate_series(1, 100) as a(g) cross join generate_series(1, 1000) as b(g)",
		10, nil)
	if err != nil || len(read.Rows) != 10 {
		t.Fatalf("the read has %d rows: %v", len(read.Rows), err)
	}
	if state := session.ReadTransactionState(); state != db.TransactionOpen {
		t.Errorf("the transaction state after the read is %v", state)
	}
	if kept := readText(t, session, "select count(*) from masume_kept"); kept != "1" {
		t.Errorf("the transaction holds %s rows", kept)
	}
}

// openReadOnlySession opens a session on a profile that refuses writes.
func openReadOnlySession(t *testing.T) db.Session {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.AccessMode = cfg.AccessReadOnly
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestARowLimitKeepsAReadOnlyTransactionOpen(t *testing.T) {
	sessions := map[string]struct {
		session db.Session
		begin   func(context.Context, db.Session) error
	}{
		"a read-only profile": {
			session: openReadOnlySession(t),
			begin: func(ctx context.Context, session db.Session) error {
				return session.BeginTransaction(ctx)
			},
		},
		"a read-only transaction": {
			session: dbtest.Open(t, dbtest.Postgres),
			begin: func(ctx context.Context, session db.Session) error {
				_, err := session.RunQuery(ctx, "begin read only", 1, nil)
				return err
			},
		},
	}
	for name, held := range sessions {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if err := held.begin(ctx, held.session); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = held.session.RollbackTransaction(ctx) })

			read, err := held.session.RunQuery(ctx,
				"select a.g from generate_series(1, 1000) as a(g) "+
					"cross join generate_series(1, 1000) as b(g)",
				10, nil)
			if err != nil || len(read.Rows) != 10 {
				t.Fatalf("the read has %d rows: %v", len(read.Rows), err)
			}
			if state := held.session.ReadTransactionState(); state != db.TransactionOpen {
				t.Errorf("the transaction state after the read is %v", state)
			}
			if after := readText(t, held.session, "select 42"); after != "42" {
				t.Errorf("the next statement read %q", after)
			}
		})
	}
}
