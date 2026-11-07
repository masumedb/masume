//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// idleCheck is the keepalive of a session whose idle check pings its user connection after
// an idle second.
const idleCheck = time.Second

// checkIdleConnection runs the idle check of the health tick on the session.
func checkIdleConnection(t *testing.T, session db.Session) {
	t.Helper()
	checker, checks := db.FindIdleChecker(session)
	if !checks {
		t.Fatal("the session has no idle check")
	}
	_ = checker.CheckIdleConnection(context.Background())
}

// openIdleChecked returns a session whose idle check pings its user connection after an
// idle second.
func openIdleChecked(t *testing.T) db.Session {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.Keepalive = idleCheck
	session, err := engines.CreateAdapters().Open(context.Background(), profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestFirstStatementAfterACheckedIdleDropRuns(t *testing.T) {
	session := openIdleChecked(t)
	other := dbtest.Open(t, dbtest.Postgres)
	pid := readText(t, session, "select pg_backend_pid()")
	dbtest.RunStatements(t, other, "select pg_terminate_backend("+pid+")")
	time.Sleep(idleCheck + 100*time.Millisecond)
	checkIdleConnection(t, session)

	if held := readText(t, session, "select pg_backend_pid()"); held == pid {
		t.Errorf("the session still reads backend %s", pid)
	}
}

func TestCheckedIdleDropInATransactionReportsTheLoss(t *testing.T) {
	session := openIdleChecked(t)
	other := dbtest.Open(t, dbtest.Postgres)
	ctx := context.Background()
	pid := readText(t, session, "select pg_backend_pid()")
	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	dbtest.RunStatements(t, other, "select pg_terminate_backend("+pid+")")
	time.Sleep(idleCheck + 100*time.Millisecond)
	checkIdleConnection(t, session)

	_, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil)
	if err == nil || !strings.Contains(err.Error(), "transaction was lost") {
		t.Errorf("the statement after the drop answered %v", err)
	}
	if state := session.ReadTransactionState(); state != db.TransactionNone {
		t.Errorf("the transaction state reads %v", state)
	}
	if held := readText(t, session, "select pg_backend_pid()"); held == pid {
		t.Errorf("the session still reads backend %s", pid)
	}
}
