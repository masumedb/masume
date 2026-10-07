//go:build integration

package mysql_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

// openFlavoured opens the test server with the engine of the server that answers, MySQL or MariaDB.
func openFlavoured(t *testing.T) db.Session {
	t.Helper()
	probe := dbtest.Open(t, dbtest.MySQL)
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	if strings.Contains(strings.ToLower(probe.Describe().ServerVersion), "mariadb") {
		profile.Engine = core.EngineMariadb
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatalf("the connection answered %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestServerReportsWhichSessionWaitsForALock(t *testing.T) {
	session := openFlavoured(t)
	holder := openFlavoured(t)
	waiter := openFlavoured(t)
	dbtest.RunStatements(t, session,
		"drop table if exists masume_lock_orders",
		"create table masume_lock_orders (id int primary key, total int not null) engine = innodb",
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
	if found.Mode == "" {
		t.Error("the wait has no lock mode")
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
