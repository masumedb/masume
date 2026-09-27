package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/hist"
)

func TestTheHistoryGivesTheStatementTheFreeWidth(t *testing.T) {
	model := buildOfflineModel(t, 160, 40)
	connection := model.Active()
	long := "select o.id, c.name, o.total, o.status, o.placed_at from orders o " +
		"join customers c on c.id = o.customer_id"
	connection.Overlay = app.Overlay{
		Kind: app.OverlayHistory,
		Entries: []hist.HistoryEntry{
			{SQL: long, RanAt: time.Now(), Elapsed: 3 * time.Millisecond,
				RowCount: 200, HasRowCount: true},
			{SQL: "update orders set total = 1", RanAt: time.Now(),
				Elapsed: 570 * time.Microsecond, RowCount: 999, HasRowCount: true},
		},
	}
	screen := stripEscapes(model.render())
	if !strings.Contains(screen, long) {
		t.Errorf("the statement is cut:\n%s", screen)
	}
	for _, said := range []string{"ran at", "statement", "rows", "time"} {
		if !strings.Contains(screen, said) {
			t.Errorf("the history has no heading %q:\n%s", said, screen)
		}
	}
	lines := strings.Split(screen, "\n")
	first, second := -1, -1
	for _, line := range lines {
		if strings.Contains(line, "o.customer_id") {
			first = utf8.RuneCountInString(line[:strings.Index(line, "3 ms")+len("3 ms")])
		}
		if strings.Contains(line, "update orders set total = 1") {
			second = utf8.RuneCountInString(line[:strings.Index(line, "0.57 ms")+len("0.57 ms")])
		}
	}
	if first < 0 || first != second {
		t.Errorf("the times end in columns %d and %d, wanted one column", first, second)
	}
}
