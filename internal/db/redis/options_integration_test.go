//go:build integration

package redis_test

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
	profile, password := dbtest.BuildProfile(t, dbtest.Redis)
	profile.Options = map[string]string{"client_name": "masume-options-test", "dial_timeout": "3s"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	read, err := session.RunQuery(ctx, "CLIENT GETNAME", dbtest.ReadEverything, nil)
	if err != nil || len(read.Rows) != 1 || core.FormatCell(read.Rows[0][0], "") != "masume-options-test" {
		t.Errorf("the client name reads %v: %v", read.Rows, err)
	}
}

func TestServerRefusesAnOptionTheDriverDoesNotKnow(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Redis)
	profile.Options = map[string]string{"nonsense": "1"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err == nil {
		_ = session.Close()
		t.Fatal("the connection opened with an unknown option")
	}
	if !strings.Contains(db.DescribeError(err), "nonsense") {
		t.Errorf("the error is %v", db.DescribeError(err))
	}
}
