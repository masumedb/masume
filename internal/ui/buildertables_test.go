package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/db"
)

func TestTheBuilderTablePickerMarksAView(t *testing.T) {
	model := buildOfflineModel(t, 160, 40)
	connection := model.Active()
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "order_totals", Kind: db.RelationView},
	}
	model.askBuilderTable(connection, nil)

	rows := connection.Overlay.Palette
	if len(rows) != 2 {
		t.Fatalf("the picker lists %d tables", len(rows))
	}
	if rows[0].Icon != cfg.IconTable || rows[0].Detail != "" {
		t.Errorf("the table row is %+v", rows[0])
	}
	if rows[1].Icon != cfg.IconView || rows[1].Detail != "view" {
		t.Errorf("the view row is %+v", rows[1])
	}
}
