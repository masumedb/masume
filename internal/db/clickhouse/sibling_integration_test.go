//go:build integration

package clickhouse_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

// readMaxThreads returns the max_threads setting of the server session.
func readMaxThreads(t *testing.T, session db.Session) string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select value from system.settings where name = 'max_threads'", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatal(err)
	}
	return core.FormatCell(answered.Rows[0][0], "")
}

func TestSiblingKeepsAServerSessionOfItsOwn(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	sibling := dbtest.OpenSibling(t, dbtest.Clickhouse, session)

	if _, err := sibling.RunQuery(context.Background(),
		"set max_threads = 3", dbtest.ReadEverything, nil); err != nil {
		t.Fatal(err)
	}
	if held := readMaxThreads(t, sibling); held != "3" {
		t.Errorf("the sibling reads max_threads = %q", held)
	}
	if held := readMaxThreads(t, session); held == "3" {
		t.Error("the setting of the sibling reached the session")
	}
}
