//go:build integration

package sqlserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

func TestServerTakesTheOptionsOfTheProfile(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Sqlserver)
	profile.Options = map[string]string{"app name": "masume options test", "dial timeout": "5"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	read, err := session.RunQuery(ctx, "select app_name()", dbtest.ReadEverything, nil)
	if err != nil || len(read.Rows) != 1 || read.Rows[0][0] != "masume options test" {
		t.Errorf("the application name reads %v: %v", read.Rows, err)
	}
}

func TestServerRefusesAnOptionValueTheDriverCannotRead(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Sqlserver)
	profile.Options = map[string]string{"dial timeout": "soon"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	if session, err := engines.CreateAdapters().Open(ctx, profile, password); err == nil {
		_ = session.Close()
		t.Fatal("the connection opened with an option the driver cannot read")
	}
}
