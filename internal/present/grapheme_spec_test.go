package present_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/masumedb/masume/internal/present"
)

// graphemeTexts are texts where the number of runes is not the number of cells, with the cells
// the renderer draws for each.
var graphemeTexts = []struct {
	name  string
	text  string
	cells int
}{
	{"a skin tone", "👍🏽", 2},
	{"a family joined by zero width joiners", "👨\u200d👩\u200d👧", 2},
	{"a heart with the emoji selector", "❤\ufe0f", 2},
	{"a keycap", "1\ufe0f\u20e3", 2},
	{"a flag", "🇩🇪", 2},
	{"a profession with a skin tone", "👩🏽\u200d💻", 2},
	{"CJK", "漢字", 4},
	{"a combining mark", "e\u0301", 1},
	{"two combining marks", "e\u0327\u0301", 1},
	{"text around them", "a👍🏽b漢e\u0301c", 8},
}

func TestMeasureTextAnswersTheCellsTheRendererDraws(t *testing.T) {
	for _, held := range graphemeTexts {
		t.Run(held.name, func(t *testing.T) {
			if drawn := lipgloss.Width(held.text); drawn != held.cells {
				t.Fatalf("the renderer draws %q in %d cells, wanted %d", held.text, drawn, held.cells)
			}
			if measured := present.MeasureText(held.text); measured != held.cells {
				t.Errorf("MeasureText(%q) is %d, wanted %d", held.text, measured, held.cells)
			}
			if measured := present.MeasureTextUpTo(held.text, 99); measured != held.cells {
				t.Errorf("MeasureTextUpTo(%q) is %d, wanted %d", held.text, measured, held.cells)
			}
		})
	}
}

// findClusterStarts returns the start of each grapheme cluster of the text, and the length of
// the text, as byte offsets.
func findClusterStarts(text string) []int {
	starts := []int{0}
	for at := 0; at < len(text); {
		cluster, _ := ansi.FirstGraphemeCluster(text[at:], ansi.GraphemeWidth)
		at += len(cluster)
		starts = append(starts, at)
	}
	return starts
}

func TestTruncateTextKeepsEveryGraphemeClusterWhole(t *testing.T) {
	for _, held := range graphemeTexts {
		text := "ab" + held.text + "cd"
		starts := findClusterStarts(text)
		for width := 1; width <= present.MeasureText(text); width++ {
			t.Run(fmt.Sprintf("%s in %d cells", held.name, width), func(t *testing.T) {
				cut := present.TruncateText(text, width)
				if drawn := lipgloss.Width(cut); drawn != width {
					t.Errorf("%q cut to %d draws %d cells: %q", text, width, drawn, cut)
				}
				kept := strings.TrimRight(strings.TrimSuffix(cut, "…"), " ")
				if !strings.HasPrefix(text, kept) || !slices.Contains(starts, len(kept)) {
					t.Errorf("%q cut to %d splits a cluster: %q", text, width, cut)
				}

				cut = present.TruncateTextStart(text, width)
				if drawn := lipgloss.Width(cut); drawn != width {
					t.Errorf("%q cut from the start to %d draws %d cells: %q",
						text, width, drawn, cut)
				}
				kept = strings.TrimLeft(strings.TrimPrefix(cut, "…"), " ")
				if !strings.HasSuffix(text, kept) || !slices.Contains(starts, len(text)-len(kept)) {
					t.Errorf("%q cut from the start to %d splits a cluster: %q", text, width, cut)
				}
			})
		}
	}
}

func TestFindByteOfCellReturnsTheStartOfAGraphemeCluster(t *testing.T) {
	for _, held := range graphemeTexts {
		text := "ab" + held.text + "cd"
		starts := findClusterStarts(text)
		for cell := range present.MeasureText(text) + 1 {
			if at := present.FindByteOfCell(text, cell); !slices.Contains(starts, at) {
				t.Errorf("cell %d of %q is byte %d, inside a cluster", cell, text, at)
			}
		}
	}
}
