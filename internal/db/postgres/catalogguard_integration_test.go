//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// A catalog read that fails inside the user transaction leaves that transaction usable.
func TestAFailedCatalogReadKeepsTheUserTransaction(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	session, err := engines.CreateAdapters().Open(context.Background(), profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	ctx := context.Background()
	dbtest.RunStatements(t, session,
		"drop table if exists masume_guard",
		"create table masume_guard (id int primary key)")
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop table if exists masume_guard", dbtest.ReadEverything, nil)
	})

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := session.RunQuery(ctx, "insert into masume_guard values (1)",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := session.DescribeTable(ctx,
		db.TableRef{Schema: "public", Name: "masume_missing", Kind: db.RelationTable}); err == nil {
		t.Fatal("the read of a missing table answered no error")
	}
	if held := session.ReadTransactionState(); held != db.TransactionOpen {
		t.Errorf("the state reads %q after the failed read, wanted it open", held)
	}
	if _, err := session.RunQuery(ctx, "insert into masume_guard values (2)",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the transaction refused a statement after the failed read: %v", err)
	}
	if err := session.CommitTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	answered, err := session.RunQuery(ctx, "select count(*) from masume_guard",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 2 {
		t.Errorf("the commit kept %d rows, wanted 2", held)
	}
}

// The dashboard reads the server outside the user transaction, so the activity list holds the
// user connection, idle in its transaction.
func TestTheDashboardReadsOutsideTheUserTransaction(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	session, err := engines.CreateAdapters().Open(context.Background(), profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	ctx := context.Background()
	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.RollbackTransaction(context.Background()) })
	if _, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil); err != nil {
		t.Fatal(err)
	}
	user, known := db.FindBackendID(session)
	if !known {
		t.Fatal("the session reports no backend id")
	}

	sessions, err := session.ListActivity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, held := range sessions {
		if held.PID == user {
			return
		}
	}
	t.Errorf("the activity list misses the user connection %d", user)
}
