//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestServerStopsARunningStatement(t *testing.T) {
	for _, target := range []dbtest.Target{dbtest.Postgres, dbtest.Cockroach} {
		t.Run(string(target.Engine), func(t *testing.T) {
			stopRunningStatement(t, dbtest.Open(t, target))
		})
	}
}

func stopRunningStatement(t *testing.T, session db.Session) {
	t.Helper()
	ctx := context.Background()

	var group sync.WaitGroup
	var runErr error
	group.Add(1)
	go func() {
		defer group.Done()
		_, runErr = session.RunQuery(ctx, "select pg_sleep(5)", dbtest.ReadEverything, nil)
	}()

	stopped := false
	for at := 0; at < 30 && !stopped; at++ {
		time.Sleep(100 * time.Millisecond)
		held, err := session.CancelRunningQuery(ctx)
		if err != nil {
			t.Fatalf("the cancel answered %v", err)
		}
		stopped = held
	}
	group.Wait()

	if !stopped {
		t.Fatal("the statement was not stopped")
	}
	if runErr == nil {
		t.Error("the stopped statement answered no error")
	}
	if _, err := session.RunQuery(ctx, "select 1", dbtest.ReadEverything, nil); err != nil {
		t.Errorf("the statement after the stopped one answered %v", err)
	}
}

func TestCockroachStopsAnotherSessionBothWays(t *testing.T) {
	for _, held := range []struct {
		name   string
		marker string
		ends   bool
	}{
		{"cancel the statement", "masume-crdb-cancel-marker", false},
		{"end the session", "masume-crdb-end-marker", true},
	} {
		t.Run(held.name, func(t *testing.T) {
			session := dbtest.Open(t, dbtest.Cockroach)
			other := dbtest.Open(t, dbtest.Cockroach)

			ctx := context.Background()
			go func() {
				_, _ = other.RunQuery(ctx,
					"select pg_sleep(5) /* "+held.marker+" */", dbtest.ReadEverything, nil)
			}()

			pid := findSessionByMarker(t, session, held.marker)
			stopped, stopErr := session.CancelBackend(ctx, pid, held.ends)
			if stopErr != nil {
				t.Fatalf("the server refused to stop session %d: %v", pid, stopErr)
			}
			if !stopped {
				t.Errorf("the server did not stop session %d", pid)
			}
		})
	}
}

func TestCockroachReportsWhichSessionWaitsForALock(t *testing.T) {
	session := dbtest.Open(t, dbtest.Cockroach)
	holder := dbtest.Open(t, dbtest.Cockroach)
	waiter := dbtest.Open(t, dbtest.Cockroach)
	dbtest.RunStatements(t, session,
		"drop table if exists masume_lock_orders",
		"create table masume_lock_orders (id int primary key, total int not null)",
		"insert into masume_lock_orders values (1, 1)")
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop table if exists masume_lock_orders", dbtest.ReadEverything, nil)
	})

	ctx := context.Background()
	if err := holder.BeginTransaction(ctx); err != nil {
		t.Fatalf("the holder opened no transaction: %v", err)
	}
	t.Cleanup(func() { _ = holder.RollbackTransaction(context.Background()) })
	if _, err := holder.RunQuery(ctx,
		"update masume_lock_orders set total = 2 where id = 1",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the holder took no lock: %v", err)
	}

	blocked := make(chan error, 1)
	go func() {
		_, err := waiter.RunQuery(context.Background(),
			"update masume_lock_orders set total = 3 where id = 1", dbtest.ReadEverything, nil)
		blocked <- err
	}()

	var found db.LockWait
	for range 100 {
		waits, err := session.ListLockWaits(ctx)
		if err != nil {
			t.Fatalf("the server did not report its lock waits: %v", err)
		}
		for _, wait := range waits {
			if strings.Contains(wait.BlockedQuery, "total = 3") {
				found = wait
			}
		}
		if found.BlockedPID != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if found.BlockedPID == 0 {
		t.Fatal("the server reported no session waiting for the lock that is held")
	}
	if found.BlockingPID == 0 || found.BlockedPID == found.BlockingPID {
		t.Errorf("the wait has the holder %d and the waiter %d", found.BlockingPID, found.BlockedPID)
	}
	if found.Relation != "masume_lock_orders" {
		t.Errorf("the wait is on relation %q", found.Relation)
	}
	if !strings.Contains(found.BlockingQuery, "total = 2") {
		t.Errorf("the holder is reported as running %q", found.BlockingQuery)
	}

	if err := holder.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the holder did not roll back: %v", err)
	}
	select {
	case err := <-blocked:
		if err != nil {
			t.Errorf("the waiter failed once the lock was free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Error("the waiter never finished after the lock was freed")
	}
}
