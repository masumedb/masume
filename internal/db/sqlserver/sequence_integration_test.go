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
	"github.com/masumedb/masume/internal/db"
)

// SQL Server 2016 has no last_used_value. The statement for it reads, and a sequence that
// was never used continues one value after its start.
func TestSequenceValuesReadWithoutLastUsedValue(t *testing.T) {
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

	for _, statement := range []string{
		"if schema_id('masume_seq16') is not null begin drop sequence if exists masume_seq16.unused; drop sequence if exists masume_seq16.used; drop schema masume_seq16 end",
		"create schema masume_seq16",
		"create sequence masume_seq16.unused as int start with 10",
		"create sequence masume_seq16.used as int start with 10",
		"select next value for masume_seq16.used",
	} {
		if _, err := session.Connection.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		_, _ = session.Connection.ExecContext(context.Background(),
			"drop sequence masume_seq16.unused; drop sequence masume_seq16.used; drop schema masume_seq16")
	})

	for _, one := range []struct {
		hasLastUsedValue bool
		unused, used     string
	}{
		{true, "10", "11"},
		{false, "11", "11"},
	} {
		rows, _, err := session.ReadNamedRows(ctx, buildListSequenceValuesSQL(one.hasLastUsedValue))
		if err != nil {
			t.Fatalf("last_used_value %v: %v", one.hasLastUsedValue, err)
		}
		restarts := map[string]string{}
		for _, row := range rows {
			if db.ReadAnyText(row["schema"]) == "masume_seq16" {
				restarts[db.ReadAnyText(row["name"])] = db.ReadAnyText(row["restart_value"])
			}
		}
		if restarts["unused"] != one.unused || restarts["used"] != one.used {
			t.Errorf("last_used_value %v: restarts %v, wanted unused %s and used %s",
				one.hasLastUsedValue, restarts, one.unused, one.used)
		}
	}
}
