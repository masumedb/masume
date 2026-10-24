package clickhouse

import (
	"testing"
	"time"
)

// A value takes the place of a placeholder as a literal. A mark in a string, a quoted name
// or a comment is no placeholder.
func TestBindParamsWritesEachValueAsALiteral(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	for _, held := range []struct {
		statement string
		params    []any
		want      string
	}{
		{"select ?", []any{"it's \\"}, `select 'it\'s \\'`},
		{"select ?, ?, ?", []any{int64(-5), true, nil}, "select -5, 1, NULL"},
		{"select ?", []any{1.5}, "select cast(1.5, 'Float64')"},
		{"select ? from `a?` where b = '?' -- ?\n", []any{1}, "select 1 from `a?` where b = '?' -- ?\n"},
		{"select ? /* ? */ #?", []any{1}, "select 1 /* ? */ #?"},
		{"select \\?, ?", []any{2}, "select ?, 2"},
		{"select 'a\\'?', ?", []any{2}, "select 'a\\'?', 2"},
		{`select 'C:\\?', ?`, []any{1}, `select 'C:\\?', 1`},
		{"select ?", []any{[]string{"a", "b"}}, "select ['a', 'b']"},
		{"select ?", []any{[]byte{0, 255}}, "select unhex('00ff')"},
		{"select ?", []any{time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)},
			"select toDateTime('2026-09-23 12:00:00', 'UTC')"},
		{"select ?", []any{time.Date(2026, 9, 23, 12, 0, 0, 125000000, tokyo)},
			"select toDateTime64('2026-09-23 12:00:00.125000000', 9, 'Asia/Tokyo')"},
		{"select ?", nil, "select ?"},
	} {
		bound, err := bindParams(held.statement, held.params)
		if err != nil {
			t.Errorf("%q answered %v", held.statement, err)
			continue
		}
		if bound != held.want {
			t.Errorf("%q binds as %q, wanted %q", held.statement, bound, held.want)
		}
	}
}

// A statement with more placeholders than values is refused.
func TestBindParamsRefusesAMissingValue(t *testing.T) {
	if _, err := bindParams("select ?, ?", []any{1}); err == nil {
		t.Error("a statement with a placeholder and no value was bound")
	}
}
