package present_test

import (
	"testing"

	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query"
)

func TestBuildChartRowsGroupsTheDigitsOfAValue(t *testing.T) {
	rows, problem := present.BuildChartRows(
		[]query.ResultColumn{{Name: "country"}, {Name: "revenue"}},
		[][]any{{"NL", "144602.15"}, {"DE", int64(2997)}}, "country", "revenue", false)
	if problem != "" {
		t.Fatalf("the chart is refused: %s", problem)
	}
	for at, want := range []string{"144,602.15", "2,997"} {
		if rows[at].Written != want {
			t.Errorf("row %d reads %q, wanted %q", at, rows[at].Written, want)
		}
	}
	if written := present.FormatChartValue(-1234567.5); written != "-1,234,567.5" {
		t.Errorf("the low value reads %q", written)
	}
}
