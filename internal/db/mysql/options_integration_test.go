//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

func TestServerTakesTheOptionsOfTheProfile(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	profile.Options = map[string]string{"sql_mode": "'ANSI_QUOTES'", "timeout": "5s"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	read, err := session.RunQuery(ctx, "select @@session.sql_mode", dbtest.ReadEverything, nil)
	if err != nil || len(read.Rows) != 1 || read.Rows[0][0] != "ANSI_QUOTES" {
		t.Errorf("the sql mode reads %v: %v", read.Rows, err)
	}
}

func TestServerRefusesAnOptionItDoesNotKnow(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	profile.Options = map[string]string{"masume_unknown_setting": "1"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err == nil {
		_ = session.Close()
		t.Fatal("the connection opened with an unknown option")
	}
}
