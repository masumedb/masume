package present_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

// A document can hold kilobytes in one field, and its column is drawn at the maximum width in
// every case. A measurement that stops at the maximum gives the same width for every shorter
// text, so the grid layout does not change.
func TestMeasureTextUpToAnswersTheExactWidthBelowTheLimit(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
	}{
		{"nothing", ""},
		{"plain letters", "ada"},
		{"a wide character", "漢"},
		{"wide and narrow together", "漢a漢a"},
		{"a combining mark", "é"},
		{"a control character", "back\rhere"},
		{"an escape", "red\x1b[31mtext"},
		{"bytes that are no text", string([]byte{0xff, 0xfe, 0x41})},
		{"the replacement character itself", "a�b"},
	} {
		t.Run(held.name, func(t *testing.T) {
			wanted := present.MeasureText(held.text)
			if answered := present.MeasureTextUpTo(held.text, wanted+1); answered != wanted {
				t.Errorf("%q measures %d cells up to the limit, wanted %d",
					held.text, answered, wanted)
			}
		})
	}
}

// A text above the limit is not measured further and is never reported as narrower than the
// limit, so a column at its maximum width stays there.
func TestMeasureTextUpToStopsAtTheLimit(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
	}{
		{"a long line", strings.Repeat("a", 4000)},
		{"a long line of wide characters", strings.Repeat("漢", 4000)},
		{"a document written as one line", strings.Repeat(`{"note":"a note"},`, 500)},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := present.MeasureTextUpTo(held.text, 28); answered < 28 {
				t.Errorf("%d cells were counted, wanted the limit of 28 or more", answered)
			}
		})
	}
}

// A limit of zero measures nothing, because a column of zero width draws no cell.
func TestMeasureTextUpToAnswersNothingForNoLimit(t *testing.T) {
	if answered := present.MeasureTextUpTo("ada", 0); answered != 0 {
		t.Errorf("a limit of nothing measured %d cells", answered)
	}
}

// The rows of a result arrive one page at a time. A page added to the measured widths must
// give the same widths as one measurement of every row, or a column would change its width
// with the order of the pages.
func TestWidenColumnsFoldsAPageIntoTheWidthsItHas(t *testing.T) {
	headers := []string{"_id", "customer", "note"}
	first := [][]string{
		{"64b7f0", "ada", "short"},
		{"64b7f1", "grace", "a longer note than the first"},
	}
	second := [][]string{
		{"64b7f2", "alan turing and others", "x"},
		{"64b7f3", "bob", strings.Repeat("a very long document ", 200)},
	}

	folded := present.WidenColumns(present.CalculateColumnWidths(headers, first), second)
	atOnce := present.CalculateColumnWidths(headers, append(append([][]string{}, first...), second...))

	if len(folded) != len(atOnce) {
		t.Fatalf("folding gave %d widths, measuring at once gave %d", len(folded), len(atOnce))
	}
	for index, width := range atOnce {
		if folded[index] != width {
			t.Errorf("column %d is %d cells wide after folding, wanted %d",
				index, folded[index], width)
		}
	}
}

// A cell without a header column is skipped, in the same way as in one measurement of every
// row: the widths are the widths of the columns of the result.
func TestWidenColumnsLeavesOutACellNoColumnHolds(t *testing.T) {
	widths := present.WidenColumns(
		present.CalculateColumnWidths([]string{"_id"}, nil),
		[][]string{{"64b7f0", strings.Repeat("wide", 100)}})
	if len(widths) != 1 {
		t.Fatalf("%d widths were answered for one column", len(widths))
	}
}

// The editor counts bytes and the screen counts cells, so the two are read against each other
// where the caret is drawn and where a press of the pointer lands.
func TestFindCellOfByteCountsTheCellsBeforeTheOffset(t *testing.T) {
	for _, held := range []struct {
		name   string
		text   string
		offset int
		want   int
	}{
		{"the start", "select", 0, 0},
		{"plain letters", "select", 3, 3},
		{"past the end", "select", 40, 6},
		{"after a two-byte character", "'ä' = x", 4, 3},
		{"after a wide character", "'漢' = x", 4, 3},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := present.FindCellOfByte(held.text, held.offset); answered != held.want {
				t.Errorf("answered %d, wanted %d", answered, held.want)
			}
		})
	}
}

func TestFindByteOfCellAnswersTheByteTheCellStartsAt(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
		cell int
		want int
	}{
		{"the start", "select", 0, 0},
		{"plain letters", "select", 3, 3},
		{"past the end", "select", 40, 6},
		{"after a two-byte character", "'ä' = x", 3, 4},
		{"after a wide character", "'漢' = x", 4, 5},
		// A press on the right half of a wide glyph puts the caret after it.
		{"the second cell of a wide character", "'漢' = x", 2, 4},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := present.FindByteOfCell(held.text, held.cell); answered != held.want {
				t.Errorf("answered %d, wanted %d", answered, held.want)
			}
		})
	}
}
