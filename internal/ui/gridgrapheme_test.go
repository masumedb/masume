package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/masumedb/masume/internal/db"
)

// Each value is one grapheme cluster or more, and the column after it starts at the same cell
// in every row.
func TestTheGridLinesUpTheColumnAfterAGraphemeCluster(t *testing.T) {
	const frameWidth = 160
	model := buildOfflineModel(t, frameWidth, 48)
	tab := model.Active().Active()

	values := []string{
		"plain", "👍🏽", "👨‍👩‍👧", "❤️", "1️⃣", "🇩🇪",
		"漢字", "é", "ȩ́", "a👍🏽b漢éc",
	}
	rows := make([][]any, 0, len(values))
	for _, value := range values {
		rows = append(rows, []any{value, "after"})
	}
	tab.Results.Start([]string{"select * from notes"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from notes"}, db.QueryResult{
		Columns: []db.ResultColumn{
			{Name: "note", DataType: "text"},
			{Name: "next", DataType: "text"},
		},
		Rows: rows,
	})

	starts := map[int]bool{}
	drawn := 0
	for at, row := range strings.Split(model.View().Content, "\n") {
		if width := lipgloss.Width(row); width != frameWidth {
			t.Errorf("row %d draws %d cells, wanted %d: %q", at, width, frameWidth, stripEscapes(row))
		}
		text := stripEscapes(row)
		cut := strings.Index(text, "after")
		if cut < 0 {
			continue
		}
		drawn++
		starts[lipgloss.Width(text[:cut])] = true
	}
	if drawn != len(values) {
		t.Fatalf("the grid draws %d rows, wanted %d", drawn, len(values))
	}
	if len(starts) != 1 {
		t.Errorf("the second column starts at cells %v, wanted one cell for every row", starts)
	}
}
