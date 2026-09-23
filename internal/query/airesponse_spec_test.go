package query_test

import (
	"reflect"
	"testing"

	"github.com/masumedb/masume/internal/query"
)

func TestReadMarkdownTable(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		found bool
		want  query.MarkdownTable
		next  int
	}{
		{
			name:  "a table and the prose after it",
			lines: []string{"| a | b |", "|---|--:|", "| 1 | 2 |", "|3|", "after"},
			found: true, next: 4,
			want: query.MarkdownTable{
				Headers: []string{"a", "b"},
				Rows:    [][]string{{"1", "2"}, {"3", ""}},
			},
		},
		{name: "no delimiter row", lines: []string{"| a | b |", "| 1 | 2 |"}},
		{name: "a delimiter of another width", lines: []string{"| a | b |", "|---|"}},
		{name: "prose", lines: []string{"a | b", "---"}},
		{name: "one line", lines: []string{"| a |"}},
	}
	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			table, next, found := query.ReadMarkdownTable(held.lines, 0)
			if found != held.found {
				t.Fatalf("found is %v", found)
			}
			if !found {
				return
			}
			if next != held.next || !reflect.DeepEqual(table, held.want) {
				t.Errorf("read %+v up to %d", table, next)
			}
		})
	}
}
