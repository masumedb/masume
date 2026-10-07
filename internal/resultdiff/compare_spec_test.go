package resultdiff_test

import (
	"slices"
	"testing"

	"github.com/masumedb/masume/internal/resultdiff"
)

func TestCompareByKeyFindsEachChange(t *testing.T) {
	diff := resultdiff.Compare(
		resultdiff.Side{Columns: []string{"id", "total"}, Key: []string{"id"},
			Rows: [][]any{{1, 10}, {2, 20}, {3, 30}}},
		resultdiff.Side{Columns: []string{"id", "total"}, Key: []string{"id"},
			Rows: [][]any{{1, 10}, {2, 25}, {4, 40}}})
	if !slices.Equal(diff.Key, []string{"id"}) || diff.Same != 1 {
		t.Fatalf("key %v and %d same rows", diff.Key, diff.Same)
	}
	wanted := []resultdiff.Row{
		{Change: resultdiff.ChangeAltered, Before: []string{"2", "20"}, After: []string{"2", "25"},
			Changed: []bool{false, true}},
		{Change: resultdiff.ChangeAdded, After: []string{"4", "40"}},
		{Change: resultdiff.ChangeRemoved, Before: []string{"3", "30"}},
	}
	if len(diff.Rows) != len(wanted) {
		t.Fatalf("rows: %+v", diff.Rows)
	}
	for at, row := range diff.Rows {
		if row.Change != wanted[at].Change || !slices.Equal(row.Before, wanted[at].Before) ||
			!slices.Equal(row.After, wanted[at].After) || !slices.Equal(row.Changed, wanted[at].Changed) {
			t.Errorf("row %d is %+v, wanted %+v", at, row, wanted[at])
		}
	}
}

func TestCompareMatchesWholeRowsWithoutAKey(t *testing.T) {
	diff := resultdiff.Compare(
		resultdiff.Side{Columns: []string{"name"}, Rows: [][]any{{"ada"}, {"ada"}, {"alan"}}},
		resultdiff.Side{Columns: []string{"name"}, Rows: [][]any{{"ada"}, {"grace"}}})
	if len(diff.Key) != 0 || diff.Same != 1 {
		t.Fatalf("key %v and %d same rows", diff.Key, diff.Same)
	}
	if diff.Count(resultdiff.ChangeAdded) != 1 || diff.Count(resultdiff.ChangeRemoved) != 2 {
		t.Errorf("rows: %+v", diff.Rows)
	}
}

func TestCompareMatchesWholeRowsWhereAKeyRepeats(t *testing.T) {
	diff := resultdiff.Compare(
		resultdiff.Side{Columns: []string{"id"}, Key: []string{"id"}, Rows: [][]any{{1}, {1}}},
		resultdiff.Side{Columns: []string{"id"}, Key: []string{"id"}, Rows: [][]any{{1}}})
	if len(diff.Key) != 0 || diff.Count(resultdiff.ChangeRemoved) != 1 {
		t.Errorf("key %v and rows %+v", diff.Key, diff.Rows)
	}
}

func TestCompareReportsTheColumnsOfOneSide(t *testing.T) {
	diff := resultdiff.Compare(
		resultdiff.Side{Columns: []string{"id", "note"}, Rows: [][]any{{1, "a"}}},
		resultdiff.Side{Columns: []string{"id", "total"}, Rows: [][]any{{1, 5}}})
	if !slices.Equal(diff.Columns, []string{"id"}) ||
		!slices.Equal(diff.RemovedColumns, []string{"note"}) ||
		!slices.Equal(diff.AddedColumns, []string{"total"}) || diff.Same != 1 {
		t.Errorf("diff: %+v", diff)
	}
}
