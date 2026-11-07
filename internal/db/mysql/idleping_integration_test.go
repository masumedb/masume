//go:build integration

package mysql_test

import (
	"context"
	"strconv"
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
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	profile.Keepalive = idleCheck
	session, err := engines.CreateAdapters().Open(context.Background(), profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// killConnection ends the connection of that ID on the server.
func killConnection(t *testing.T, other db.Session, id int64) {
	t.Helper()
	if _, err := other.RunQuery(context.Background(), "kill connection "+
		strconv.FormatInt(id, 10), dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the kill answered %v", err)
	}
}

func TestFirstStatementAfterACheckedIdleDropRuns(t *testing.T) {
	session := openIdleChecked(t)
	other := dbtest.Open(t, dbtest.MySQL)
	killed := readConnectionID(t, session)
	killConnection(t, other, killed)
	time.Sleep(idleCheck + 100*time.Millisecond)
	checkIdleConnection(t, session)

	if reopened := readConnectionID(t, session); reopened == killed {
		t.Errorf("the session still reads connection %d", killed)
	}
}

func TestCheckedIdleDropInATransactionReportsTheLoss(t *testing.T) {
	session := openIdleChecked(t)
	other := dbtest.Open(t, dbtest.MySQL)
	ctx := context.Background()
	killed := readConnectionID(t, session)
	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	killConnection(t, other, killed)
	time.Sleep(idleCheck + 100*time.Millisecond)
	checkIdleConnection(t, session)

	_, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil)
	if err == nil || !strings.Contains(err.Error(), "transaction was lost") {
		t.Errorf("the statement after the drop answered %v", err)
	}
	if reopened := readConnectionID(t, session); reopened == killed {
		t.Errorf("the session still reads connection %d", killed)
	}
}
