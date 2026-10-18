//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

// readText returns the first cell of a read as text.
func readText(t *testing.T, session db.Session, statement string) string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(), statement, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("%s answered %v", statement, err)
	}
	return core.FormatCell(answered.Rows[0][0], "")
}

func TestTimedOutStatementKeepsTheSession(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	other := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, "set application_name = 'kept'")
	pid := readText(t, session, "select pg_backend_pid()")

	limited, stop := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer stop()
	if _, err := session.RunQuery(limited, "select pg_sleep(30)", dbtest.ReadEverything, nil); err == nil {
		t.Fatal("the sleep ran to the end")
	}

	if running := readText(t, other, "select count(*) from pg_stat_activity where pid = "+pid+
		" and state = 'active'"); running != "0" {
		t.Errorf("the server still runs the sleep on backend %s", pid)
	}
	if held := readText(t, session, "select pg_backend_pid()"); held != pid {
		t.Errorf("the session moved from backend %s to %s", pid, held)
	}
	if held := readText(t, session, "show application_name"); held != "kept" {
		t.Errorf("the session setting reads %q", held)
	}
}

func TestLostTransactionIsReportedOnce(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	other := dbtest.Open(t, dbtest.Postgres)
	ctx := context.Background()
	pid := readText(t, session, "select pg_backend_pid()")
	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the begin answered %v", err)
	}
	dbtest.RunStatements(t, other, "select pg_terminate_backend("+pid+")")

	// The first statement finds the connection closed.
	_, _ = session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil)
	_, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil)
	if err == nil || !strings.Contains(err.Error(), "transaction was lost") {
		t.Errorf("the statement after the drop answered %v", err)
	}
	if _, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil); err != nil {
		t.Errorf("the next statement answered %v", err)
	}
	if state := session.ReadTransactionState(); state != db.TransactionNone {
		t.Errorf("the transaction state reads %v", state)
	}
	if held := readText(t, session, "select pg_backend_pid()"); held == pid {
		t.Errorf("the session still reads backend %s", pid)
	}
}

func TestSideConnectionOpensAgainAfterTheServerClosesIt(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	other := dbtest.Open(t, dbtest.Postgres)
	ctx := context.Background()
	if _, err := session.ListSchemas(ctx); err != nil {
		t.Fatalf("the first catalog read answered %v", err)
	}
	user := readText(t, session, "select pg_backend_pid()")
	dbtest.RunStatements(t, other, "select pg_terminate_backend(pid) from pg_stat_activity "+
		"where application_name = 'masume' and pid not in ("+user+", pg_backend_pid())")

	if err := session.Ping(ctx); err != nil {
		t.Errorf("the health check answered %v", err)
	}
	if _, err := session.ListSchemas(ctx); err != nil {
		t.Errorf("the catalog read answered %v", err)
	}
}
