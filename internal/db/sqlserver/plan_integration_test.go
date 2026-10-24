//go:build integration

package sqlserver

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

func TestAFailedPlanSettingOpensTheConnectionAgain(t *testing.T) {
	written := os.Getenv("MASUME_TEST_SQLSERVER")
	if written == "" {
		t.Skip("MASUME_TEST_SQLSERVER is not set, so there is no SQL Server to read")
	}
	address, err := url.Parse(written)
	if err != nil {
		t.Fatalf("MASUME_TEST_SQLSERVER does not read as a URL: %v", err)
	}
	port, _ := strconv.Atoi(address.Port())
	password, _ := address.User.Password()
	profile := cfg.Profile{
		Name: "integration", Engine: core.EngineSqlserver,
		Host: address.Hostname(), Port: port, User: address.User.Username(),
		Database: strings.TrimPrefix(address.Path, "/"), AccessMode: cfg.AccessWrite,
	}

	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	opened, err := NewAdapter(BuildSupport(core.EngineSqlserver)).Connect(ctx, profile, password)
	if err != nil {
		t.Fatal(err)
	}
	session := opened.(*sqlserverSession)
	defer func() { _ = session.Close() }()

	var processID int
	if err := session.Connection.QueryRowContext(ctx, "select @@spid").Scan(&processID); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Connection.ExecContext(ctx, "set showplan_text on"); err != nil {
		t.Fatal(err)
	}
	killer, err := openSqlserverPool(profile, password)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = killer.Close() }()
	if _, err := killer.ExecContext(ctx, "kill "+strconv.Itoa(processID)); err != nil {
		t.Fatal(err)
	}
	session.stopPlanSetting(ctx, "showplan_text")

	read, err := session.RunQuery(ctx, "select 42 as answer", 10, nil)
	if err != nil {
		t.Fatalf("the statement after a failed plan setting failed: %v", err)
	}
	if len(read.Rows) != 1 || read.Rows[0][0] != int64(42) {
		t.Errorf("the statement after a failed plan setting read %v", read.Rows)
	}
}
