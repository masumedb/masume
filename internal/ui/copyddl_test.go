package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

func TestTheObjectMenuCopiesTheDefinition(t *testing.T) {
	capabilities := core.Capabilities{WritesDDL: true}
	for _, held := range [][]app.MenuAction{
		app.BuildObjectActions(present.TreeNode{
			Kind: present.NodeTable, Table: db.TableRef{Name: "orders", Kind: db.RelationTable},
		}, capabilities),
		app.BuildObjectActions(present.TreeNode{
			Kind: present.NodeTable, Table: db.TableRef{Name: "totals", Kind: db.RelationView},
		}, capabilities),
		app.BuildObjectActions(present.TreeNode{Kind: present.NodeObject}, capabilities),
	} {
		found := false
		for _, action := range held {
			found = found || action.ID == app.ObjectCopyDDL
		}
		if !found {
			t.Errorf("the menu %v has no Copy DDL", held)
		}
	}
}
