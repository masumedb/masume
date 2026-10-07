package resultdiff

import (
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/core"
)

// Side is one result of a compare: its column names, rows, and key columns. No key columns
// matches whole rows.
type Side struct {
	Columns []string
	Rows    [][]any
	Key     []string
}

// Change is the kind of one row difference.
type Change string

const (
	ChangeAdded   Change = "added"
	ChangeRemoved Change = "removed"
	ChangeAltered Change = "changed"
)

// Row is one row that differs. Before and After are the cells in the order of Columns, as
// text; an absent side is nil. Changed marks the cells that differ in a changed row.
type Row struct {
	Change  Change
	Before  []string
	After   []string
	Changed []bool
}

// Diff is the outcome of a compare.
type Diff struct {
	// Columns held by both sides, in the order of the after side.
	Columns []string
	// Key is the columns rows are matched by. Empty matches whole rows.
	Key  []string
	Rows []Row
	Same int
	// Columns only in the before side, and only in the after side.
	RemovedColumns []string
	AddedColumns   []string
}

// Count returns how many rows have that change.
func (diff Diff) Count(change Change) int {
	count := 0
	for _, row := range diff.Rows {
		if row.Change == change {
			count++
		}
	}
	return count
}

// Compare returns the rows of after that differ from before. Rows are matched by the key
// both sides share, and by whole rows where there is none, or where a key value repeats.
func Compare(before, after Side) Diff {
	diff := Diff{}
	for _, name := range after.Columns {
		if slices.Contains(before.Columns, name) {
			diff.Columns = append(diff.Columns, name)
		} else {
			diff.AddedColumns = append(diff.AddedColumns, name)
		}
	}
	for _, name := range before.Columns {
		if !slices.Contains(after.Columns, name) {
			diff.RemovedColumns = append(diff.RemovedColumns, name)
		}
	}

	from := project(before, diff.Columns)
	to := project(after, diff.Columns)
	if key := findSharedKey(before.Key, after.Key, diff.Columns); len(key) > 0 {
		if matched, ok := compareByKey(from, to, keyPositions(key, diff.Columns)); ok {
			diff.Key, diff.Rows, diff.Same = key, matched.Rows, matched.Same
			return diff
		}
	}
	whole := compareWholeRows(from, to, len(diff.Columns))
	diff.Rows, diff.Same = whole.Rows, whole.Same
	return diff
}

// project returns the rows as text, with the cells of these columns in this order.
func project(side Side, columns []string) [][]string {
	positions := make([]int, len(columns))
	for at, name := range columns {
		positions[at] = slices.Index(side.Columns, name)
	}
	rows := make([][]string, len(side.Rows))
	for at, row := range side.Rows {
		cells := make([]string, len(columns))
		for column, position := range positions {
			if position >= 0 && position < len(row) {
				cells[column] = core.FormatCell(row[position], "")
			}
		}
		rows[at] = cells
	}
	return rows
}

// findSharedKey returns the key where both sides have the same one and every key column is
// shared.
func findSharedKey(before, after, columns []string) []string {
	if len(before) == 0 || !slices.Equal(before, after) {
		return nil
	}
	for _, name := range after {
		if !slices.Contains(columns, name) {
			return nil
		}
	}
	return after
}

func keyPositions(key, columns []string) []int {
	positions := make([]int, len(key))
	for at, name := range key {
		positions[at] = slices.Index(columns, name)
	}
	return positions
}

func joinCells(cells []string, positions []int) string {
	parts := make([]string, len(positions))
	for at, position := range positions {
		parts[at] = cells[position]
	}
	return strings.Join(parts, "\x1f")
}

// compareByKey matches rows by key. It returns false where a key value repeats on one side.
func compareByKey(from, to [][]string, key []int) (Diff, bool) {
	before := map[string][]string{}
	for _, row := range from {
		id := joinCells(row, key)
		if _, held := before[id]; held {
			return Diff{}, false
		}
		before[id] = row
	}
	diff := Diff{}
	seen := map[string]bool{}
	for _, row := range to {
		id := joinCells(row, key)
		if seen[id] {
			return Diff{}, false
		}
		seen[id] = true
		old, held := before[id]
		if !held {
			diff.Rows = append(diff.Rows, Row{Change: ChangeAdded, After: row})
			continue
		}
		changed := make([]bool, len(row))
		differs := false
		for at := range row {
			changed[at] = row[at] != old[at]
			differs = differs || changed[at]
		}
		if !differs {
			diff.Same++
			continue
		}
		diff.Rows = append(diff.Rows, Row{Change: ChangeAltered, Before: old, After: row, Changed: changed})
	}
	for _, row := range from {
		if !seen[joinCells(row, key)] {
			diff.Rows = append(diff.Rows, Row{Change: ChangeRemoved, Before: row})
		}
	}
	return diff, true
}

// compareWholeRows matches equal rows, one for one, so a repeated row counts each time.
func compareWholeRows(from, to [][]string, width int) Diff {
	all := make([]int, width)
	for at := range all {
		all[at] = at
	}
	left := map[string]int{}
	for _, row := range from {
		left[joinCells(row, all)]++
	}
	diff := Diff{}
	for _, row := range to {
		id := joinCells(row, all)
		if left[id] > 0 {
			left[id]--
			diff.Same++
			continue
		}
		diff.Rows = append(diff.Rows, Row{Change: ChangeAdded, After: row})
	}
	for _, row := range from {
		id := joinCells(row, all)
		if left[id] > 0 {
			left[id]--
			diff.Rows = append(diff.Rows, Row{Change: ChangeRemoved, Before: row})
		}
	}
	return diff
}
