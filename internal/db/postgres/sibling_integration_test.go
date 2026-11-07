//go:build integration

package postgres_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// checkSibling opens a sibling of a session on the target, and checks that it runs on a
// backend of its own and reports that backend.
func checkSibling(t *testing.T, target dbtest.Target) {
	t.Helper()
	session := dbtest.Open(t, target)
	sibling := dbtest.OpenSibling(t, target, session)

	own := readText(t, sibling, "select pg_backend_pid()")
	if parent := readText(t, session, "select pg_backend_pid()"); own == parent {
		t.Errorf("the sibling runs on backend %s of the session", own)
	}
	if held, known := db.FindBackendID(sibling); !known || strconv.FormatInt(held, 10) != own {
		t.Errorf("the sibling reports backend %d, %v; the server says %s", held, known, own)
	}
	if sibling.Describe().ServerVersion != session.Describe().ServerVersion {
		t.Errorf("the sibling reads server version %q", sibling.Describe().ServerVersion)
	}
}

func TestSiblingRunsOnABackendOfItsOwn(t *testing.T) {
	checkSibling(t, dbtest.Postgres)
}

func TestCockroachSiblingReportsItsOwnBackend(t *testing.T) {
	checkSibling(t, dbtest.Cockroach)
}

func TestSiblingReadsAUUIDAsText(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	sibling := dbtest.OpenSibling(t, dbtest.Postgres, session)
	const written = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	if held := readText(t, sibling, "select '"+written+"'::uuid"); held != written {
		t.Errorf("the uuid reads %q", held)
	}
}

func TestReadOnlySiblingRefusesAWriteOnTheServer(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.AccessMode = cfg.AccessReadOnly
	adapters := engines.CreateAdapters()
	ctx := context.Background()
	session, err := adapters.Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	sibling := dbtest.OpenSibling(t, dbtest.Postgres, session)

	if held := readText(t, sibling, "show default_transaction_read_only"); held != "on" {
		t.Errorf("the sibling reads default_transaction_read_only = %q", held)
	}
}
