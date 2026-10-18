//go:build integration

package mysql_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

// readConnectionID returns the server ID of the user connection of the session.
func readConnectionID(t *testing.T, session db.Session) int64 {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select connection_id()", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	switch held := answered.Rows[0][0].(type) {
	case int64:
		return held
	case uint64:
		return int64(held)
	}
	t.Fatalf("the ID reads %T", answered.Rows[0][0])
	return 0
}

func TestUserConnectionOpensAgainAfterTheServerClosesIt(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	other := dbtest.Open(t, dbtest.MySQL)
	ctx := context.Background()

	killed := readConnectionID(t, session)
	if _, err := other.RunQuery(ctx, "kill connection "+strconv.FormatInt(killed, 10),
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the kill answered %v", err)
	}

	// The statement that finds the connection closed fails, and the next one runs.
	_, _ = session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil)
	if reopened := readConnectionID(t, session); reopened == killed {
		t.Errorf("the session still reads connection %d", killed)
	}
}

func TestTimedOutStatementStopsOnTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	other := dbtest.Open(t, dbtest.MySQL)
	ctx := context.Background()
	stopped := readConnectionID(t, session)

	limited, stop := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stop()
	if _, err := session.RunQuery(limited, "select sleep(30)", dbtest.ReadEverything, nil); err == nil {
		t.Fatal("the sleep ran to the end")
	}

	answered, err := other.RunQuery(ctx,
		"select count(*) from information_schema.processlist where id = "+strconv.FormatInt(stopped, 10)+
			" and info like 'select sleep%'", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the process list answered %v", err)
	}
	if count := db.ReadNonNegativeCount(answered.Rows[0][0]); count != 0 {
		t.Errorf("the server still runs the sleep on connection %d", stopped)
	}
	if _, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil); err != nil {
		t.Errorf("the next statement answered %v", err)
	}
}
