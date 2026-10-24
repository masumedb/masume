//go:build integration

package sqlserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db/dbtest"
)

func TestARowLimitStopsALargeRead(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()

	started := time.Now()
	read, err := session.RunQuery(ctx,
		"select a.object_id, replicate('x', 200) as filler "+
			"from sys.all_objects as a cross join sys.all_objects as b cross join sys.all_objects as c",
		100, nil)
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
	if err != nil || len(after.Rows) != 1 || after.Rows[0][0] != int64(42) {
		t.Errorf("the next statement read %v: %v", after.Rows, err)
	}
}
