//go:build integration

package mysql_test

import (
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestSiblingRunsOnAConnectionOfItsOwn(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	sibling := dbtest.OpenSibling(t, dbtest.MySQL, session)

	own := readConnectionID(t, sibling)
	if own == readConnectionID(t, session) {
		t.Errorf("the sibling runs on connection %d of the session", own)
	}
	if held, known := db.FindBackendID(sibling); !known || held != own {
		t.Errorf("the sibling reports connection %d, %v; the server says %d", held, known, own)
	}
	if sibling.Describe().ServerVersion != session.Describe().ServerVersion {
		t.Errorf("the sibling reads server version %q", sibling.Describe().ServerVersion)
	}
}
