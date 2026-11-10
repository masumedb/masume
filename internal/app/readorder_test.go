package app

import (
	"reflect"
	"testing"

	"github.com/masumedb/masume/internal/core"
)

// The read of a table orders by the grid sort and then by the primary key, so rows that tie
// on the grid sort keep one order from page to page.
func TestTheReadOrderAddsTheKeyAfterTheGridSort(t *testing.T) {
	id := core.SortState{Column: "id", Direction: core.SortAscending}
	status := core.SortState{Column: "status", Direction: core.SortDescending}
	idDescending := core.SortState{Column: "id", Direction: core.SortDescending}
	for _, held := range []struct {
		name string
		sort []core.SortState
		want []core.SortState
	}{
		{"no grid sort", nil, []core.SortState{id}},
		{"a sort on another column", []core.SortState{status}, []core.SortState{status, id}},
		{"a sort on the key itself", []core.SortState{idDescending},
			[]core.SortState{idDescending}},
	} {
		t.Run(held.name, func(t *testing.T) {
			tab := &Tab{Sort: held.sort, KeySort: []core.SortState{id}}
			if got := tab.buildReadOrder(); !reflect.DeepEqual(got, held.want) {
				t.Errorf("the read orders by %v, wanted %v", got, held.want)
			}
			if got := tab.ResolveReadSort(); len(held.sort) > 0 && !reflect.DeepEqual(got, held.sort) {
				t.Errorf("the header marks %v, wanted only the grid sort %v", got, held.sort)
			}
		})
	}
}
