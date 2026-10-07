//go:build integration

package postgres_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/db/dbtest"
	"github.com/masumedb/masume/internal/db/engines"
)

func TestServerTakesTheOptionsOfTheProfile(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.Options = map[string]string{
		"application_name": "masume options test",
		"options":          "-c work_mem=7MB",
		"connect_timeout":  "5",
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	for statement, wanted := range map[string]string{
		"show application_name": "masume options test", "show work_mem": "7MB",
	} {
		read, err := session.RunQuery(ctx, statement, dbtest.ReadEverything, nil)
		if err != nil || len(read.Rows) != 1 || read.Rows[0][0] != wanted {
			t.Errorf("%s reads %v: %v", statement, read.Rows, err)
		}
	}
}

func TestServerRefusesAnOptionItDoesNotKnow(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.Options = map[string]string{"masume_unknown_setting": "1"}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err == nil {
		_ = session.Close()
		t.Fatal("the connection opened with an unknown option")
	}
	if !strings.Contains(db.DescribeError(err), "masume_unknown_setting") {
		t.Errorf("the error does not name the option: %v", db.DescribeError(err))
	}
}

func TestServerIsReachedThroughAnotherHost(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	reachable := net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port))
	profile.Port = 1
	profile.OtherHosts = []string{reachable}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatalf("the other host was not tried: %v", err)
	}
	_ = session.Close()
}
