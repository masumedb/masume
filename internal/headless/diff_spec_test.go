package headless_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db/engines"
	"github.com/masumedb/masume/internal/headless"
)

func runDiff(t *testing.T, source, target cfg.Profile) (int, string, string) {
	t.Helper()
	out, reported := &bytes.Buffer{}, &bytes.Buffer{}
	code := headless.RunDiff(context.Background(), engines.CreateAdapters(), headless.DiffOptions{
		Options: headless.Options{Profile: source, Out: out, Err: reported},
		Target:  headless.Options{Profile: target, Out: out, Err: reported},
	})
	return code, out.String(), reported.String()
}

func TestDiffReportsNothingForEqualSchemas(t *testing.T) {
	code, out, reported := runDiff(t,
		buildDatabase(t, cfg.AccessWrite), buildDatabase(t, cfg.AccessWrite))
	if code != headless.CodeSame || out != "no differences\n" {
		t.Errorf("the run answered %d with %q and %q", code, out, reported)
	}
}

func TestDiffReportsTheChangedColumnsAndTables(t *testing.T) {
	source := buildDatabase(t, cfg.AccessWrite)
	target := buildDatabase(t, cfg.AccessWrite)
	if code := runStatement(t, target, `
		alter table orders add column placed_at text;
		create table customers (id integer primary key, name text not null);
		create index orders_status on orders (status);`); code != headless.CodeOK {
		t.Fatalf("the target was not changed, and the run answered %d", code)
	}

	code, out, reported := runDiff(t, source, target)
	if code != headless.CodeDifferent {
		t.Fatalf("the run answered %d: %s", code, reported)
	}
	wanted := "customers\n  + table\n\norders\n" +
		"  + column placed_at  text\n" +
		"  + index orders_status  CREATE INDEX orders_status on orders (status)\n"
	if out != wanted {
		t.Errorf("the report reads %q, wanted %q", out, wanted)
	}
}
