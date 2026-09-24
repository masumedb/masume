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
