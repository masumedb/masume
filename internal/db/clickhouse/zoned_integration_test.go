//go:build integration

package clickhouse_test

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db/dbtest"
)

// A ClickHouse DateTime column is zoned, and a column without a time zone is not.
func TestResultMarksAZonedColumn(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)

	answered, err := session.RunQuery(context.Background(),
		"select now() as moment, today() as wall", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Columns) != 2 {
		t.Fatalf("the read gave %d columns, wanted 2", len(answered.Columns))
	}
	if !answered.Columns[0].Zoned {
		t.Errorf("the %s column is not zoned", answered.Columns[0].DataType)
	}
	if answered.Columns[1].Zoned {
		t.Errorf("the %s column is zoned", answered.Columns[1].DataType)
	}
}

// A DateTime is returned in the time zone of the column.
func TestResultKeepsTheZoneOfTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)

	answered, err := session.RunQuery(context.Background(),
		"select toDateTime('2026-09-23 12:00:00', 'Asia/Tokyo')", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held, isTime := answered.Rows[0][0].(time.Time)
	if !isTime {
		t.Fatalf("the read gave %T, wanted a time", answered.Rows[0][0])
	}
	if written := held.Format("2006-01-02 15:04:05 -07:00"); written != "2026-09-23 12:00:00 +09:00" {
		t.Errorf("the moment reads %q, wanted %q", written, "2026-09-23 12:00:00 +09:00")
	}
}
