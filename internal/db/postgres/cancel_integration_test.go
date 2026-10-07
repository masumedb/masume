//go:build integration

package postgres_test

import (
	"context"
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
