package ui

import (
	"slices"
	"testing"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query/statement"
)

func TestTheForeignKeysOfAJoinGiveItsCondition(t *testing.T) {
	customers := completionRelation{
		reference: statement.TableReference{
			SelectSource: statement.SelectSource{Name: "customers"}, Alias: "c", HasAlias: true,
		},
		detail: db.TableDetail{Table: db.TableRef{Schema: "public", Name: "customers"}},
	}
	orders := completionRelation{
		reference: statement.TableReference{
			SelectSource: statement.SelectSource{Name: "orders"}, Alias: "o", HasAlias: true,
		},
		detail: db.TableDetail{
			Table: db.TableRef{Schema: "public", Name: "orders"},
			ForeignKeys: []db.ForeignKey{{
				Columns: []string{"customer_id"}, TargetSchema: "public",
				TargetTable: "customers", TargetColumns: []string{"id"},
			}},
		},
	}
	relations := []completionRelation{customers, orders}

	if found := buildJoinConditions(relations, "orders", "o"); !slices.Equal(found,
		[]string{"o.customer_id = c.id"}) {
		t.Errorf("joining orders offered %v", found)
	}
	if found := buildJoinConditions(relations, "customers", "c"); !slices.Equal(found,
		[]string{"o.customer_id = c.id"}) {
		t.Errorf("joining customers offered %v", found)
	}
}
