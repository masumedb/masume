//go:build integration

package sqlserver_test

import (
	"context"
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

// readSessionID returns the session id of the user connection of the session.
func readSessionID(t *testing.T, session db.Session) int64 {
	t.Helper()
	answered, err := session.RunQuery(context.Background(), "select @@spid", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	return db.ReadNonNegativeCount(answered.Rows[0][0])
}

func TestSiblingRunsOnASessionOfItsOwn(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	sibling := dbtest.OpenSibling(t, dbtest.Sqlserver, session)

	own := readSessionID(t, sibling)
	if own == readSessionID(t, session) {
		t.Errorf("the sibling runs on session %d of the session", own)
	}
	if held, known := db.FindBackendID(sibling); !known || held != own {
		t.Errorf("the sibling reports session %d, %v; the server says %d", held, known, own)
	}
	if sibling.Describe().DefaultSchema != session.Describe().DefaultSchema {
		t.Errorf("the sibling reads default schema %q", sibling.Describe().DefaultSchema)
	}
}
