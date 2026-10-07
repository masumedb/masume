//go:build integration

package clickhouse_test

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

func TestServerTakesTheOptionsOfTheProfile(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Clickhouse)
	profile.Options = map[string]string{"max_execution_time": "7", "dial_timeout": "5s"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	read, err := session.RunQuery(context.Background(),
		"select value from system.settings where name = 'max_execution_time'", dbtest.ReadEverything, nil)
	if err != nil || len(read.Rows) != 1 || core.FormatCell(read.Rows[0][0], "") != "7" {
		t.Errorf("the setting reads %v: %v", read.Rows, err)
	}
}

func TestServerRefusesAnOptionItDoesNotKnow(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Clickhouse)
	profile.Options = map[string]string{"masume_unknown_setting": "1"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err == nil {
		_ = session.Close()
		t.Fatal("the connection opened with an unknown setting")
	}
	if !strings.Contains(db.DescribeError(err), "unknown setting masume_unknown_setting") {
		t.Errorf("the error is %v", db.DescribeError(err))
	}
}
