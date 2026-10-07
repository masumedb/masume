//go:build integration

package mongo_test

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
	for _, held := range []struct {
		name        string
		target      dbtest.Target
		transaction bool
	}{
		{"standalone", dbtest.Mongo, false},
		{"authentication", dbtest.MongoAuth, false},
		{"transaction", dbtest.MongoReplicaSet, true},
	} {
		t.Run(held.name, func(t *testing.T) {
			session := dbtest.Open(t, held.target)
			ctx := context.Background()
			dbtest.RunStatements(t, session,
				"db.masume_cancel.drop()", `db.masume_cancel.insertOne({name: "slow"})`)
			t.Cleanup(func() {
				_, _ = session.RunQuery(context.Background(),
					"db.masume_cancel.drop()", dbtest.ReadEverything, nil)
			})
			if held.transaction {
				if err := session.BeginTransaction(ctx); err != nil {
					t.Fatalf("the transaction did not open: %v", err)
				}
				t.Cleanup(func() { _ = session.RollbackTransaction(context.Background()) })
			}
			stopRunningStatement(t, session)
		})
	}
}

func stopRunningStatement(t *testing.T, session db.Session) {
	t.Helper()
	ctx := context.Background()

	var group sync.WaitGroup
	var runErr error
	group.Add(1)
	started := time.Now()
	go func() {
		defer group.Done()
		_, runErr = session.RunQuery(ctx,
			`db.masume_cancel.find({$where: "sleep(10000) || true"})`, dbtest.ReadEverything, nil)
	}()

	stopped := false
	for at := 0; at < 50 && !stopped; at++ {
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
	if runErr == nil || !strings.Contains(db.DescribeError(runErr), "interrupted") {
		t.Errorf("the stopped statement answered %v", runErr)
	}
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Errorf("the statement ran for %v after the cancel", elapsed)
	}
}

func TestServerStopsNothingWhereNothingRuns(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)
	stopped, err := session.CancelRunningQuery(context.Background())
	if err != nil {
		t.Fatalf("the cancel answered %v", err)
	}
	if stopped {
		t.Error("the cancel reported a stop with nothing running")
	}
	if _, runErr := session.RunQuery(context.Background(),
		"db.getCollectionNames()", dbtest.ReadEverything, nil); runErr != nil {
		t.Errorf("the session failed after the cancel: %v", runErr)
	}
}
