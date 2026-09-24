package build_test

import (
	"testing"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/postgres"
	"github.com/masumedb/masume/internal/query/build"
)

func TestInlineFilterQuotesOnlyTheColumnThatNeedsIt(t *testing.T) {
	steps := []core.FilterStep{
		{Column: "qty", Test: core.FilterEquals, Value: 7},
		{Column: "order", Test: core.FilterIsNull},
		{Column: "Total", Test: core.FilterIsNotNull},
	}
	shown := build.InlineFilter(steps, postgres.Dialect)
	if shown.Text != `qty = 7 and "order" is null and "Total" is not null` {
		t.Errorf("the shown filter reads %q", shown.Text)
	}
	sent := build.ComposeFilter(steps, postgres.Dialect, 1)
	if sent.Text != `"qty" = $1 and "order" is null and "Total" is not null` {
		t.Errorf("the sent filter reads %q", sent.Text)
	}
}
