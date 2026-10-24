//go:build integration

package clickhouse_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestARowLimitStopsALargeRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()

	started := time.Now()
	read, err := session.RunQuery(ctx,
		"select number, repeat('x', 200) as filler from numbers(1000000000)", 100, nil)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("the read answered %v after %s", err, elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the read took %s", elapsed)
	}
	if len(read.Rows) != 100 || !read.Truncated {
		t.Errorf("the read has %d rows, truncated %v", len(read.Rows), read.Truncated)
	}

	after, err := session.RunQuery(ctx, "select 42 as answer", 10, nil)
	if err != nil || len(after.Rows) != 1 {
		t.Errorf("the next statement read %v: %v", after.Rows, err)
	}
}

func TestAnErrorAfterTheRowLimitFailsTheRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()

	_, err := session.RunQuery(ctx,
		"select number from numbers(10) where throwIf(number = 3) = 0 "+
			"settings max_block_size = 1", 2, nil)
	if err == nil {
		t.Fatal("a read that failed after its row limit answered no error")
	}
	if written := db.DescribeError(err); !strings.Contains(written, "throwIf") {
		t.Errorf("the read answered %q", written)
	}
}

func TestACancelledWriteStopsOnTheServer(t *testing.T) {
	session := openShop(t)
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()

	_, err := session.RunQuery(ctx,
		"insert into shop.orders (id, customer, total) "+
			"select number, 'masume_cancelled_write', sleepEachRow(1) from numbers(60) "+
			"settings max_block_size = 1", dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a cancelled write answered no error")
	}
	running, err := session.RunQuery(context.Background(),
		"select count() from system.processes "+
			"where query like '%masume_cancelled_write%' and query not like '%system.processes%'",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the next statement answered %v", err)
	}
	if held := db.ReadNonNegativeCount(running.Rows[0][0]); held != 0 {
		t.Errorf("the server still runs %d cancelled writes", held)
	}
}
