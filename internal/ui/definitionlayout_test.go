package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/query/language"
)

func TestLayoutDefinitionLaysOutALongOneLineStatement(t *testing.T) {
	for _, held := range []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			"a table and a short index",
			[]string{
				"CREATE TABLE customers (id integer PRIMARY KEY, name text NOT NULL, email text UNIQUE NOT NULL);",
				"",
				"CREATE INDEX customers_name ON customers (name);",
			},
			[]string{
				"CREATE TABLE customers (",
				"  id integer PRIMARY KEY,",
				"  name text NOT NULL,",
				"  email text UNIQUE NOT NULL",
				");",
				"",
				"CREATE INDEX customers_name ON customers (name);",
			},
		},
		{
			"a view on one line",
			[]string{"CREATE ALGORITHM=UNDEFINED VIEW `paid` AS select `o`.`id` AS `id` from `orders` `o` where (`o`.`total` > 0)"},
			[]string{
				"CREATE ALGORITHM=UNDEFINED VIEW `paid` AS",
				"select `o`.`id` AS `id`",
				"from `orders` `o`",
				"where (`o`.`total` > 0)",
			},
		},
		{
			"a statement over several lines",
			[]string{"create table t (", "  a text, b text, c text, d text, e text, f text, g text, h text, i text", ");"},
			[]string{"create table t (", "  a text, b text, c text, d text, e text, f text, g text, h text, i text", ");"},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := layoutDefinition(language.SQL, held.lines)
			if strings.Join(got, "\n") != strings.Join(held.want, "\n") {
				t.Errorf("the definition is\n%s\nwanted\n%s",
					strings.Join(got, "\n"), strings.Join(held.want, "\n"))
			}
		})
	}
}

func TestTheDDLViewMarksALineItCutsAndScrollsSideways(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	tab := model.Active().Active()
	held := []string{"CREATE TABLE t (", "  created_at text NOT NULL DEFAULT CURRENT_TIMESTAMP", ");"}

	rows := model.renderWideLines(tab, held, 30, 5)
	if got := stripEscapes(rows[1]); !strings.HasSuffix(strings.TrimRight(got, " "), "…") {
		t.Errorf("the cut line reads %q, wanted an ellipsis at its right edge", got)
	}
	if got := stripEscapes(rows[0]); strings.Contains(got, "…") {
		t.Errorf("the line that fits reads %q, wanted no ellipsis", got)
	}

	tab.DetailColumnOffset = 1 << 20
	rows = model.renderWideLines(tab, held, 30, 5)
	got := stripEscapes(rows[1])
	if !strings.HasPrefix(got, "…") || !strings.Contains(got, "CURRENT_TIMESTAMP") {
		t.Errorf("the line scrolled to its end reads %q, wanted an ellipsis on the left and its end", got)
	}
	if tab.DetailColumnOffset != 52-28 {
		t.Errorf("the offset is %d, wanted it held to the widest line", tab.DetailColumnOffset)
	}
}
