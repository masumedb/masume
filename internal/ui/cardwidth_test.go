package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

func TestACardIsSizedToItsContent(t *testing.T) {
	if width := fitCardWidth(140, 30, "", " diagram ", ""); width != narrowestOverlayCard {
		t.Errorf("a card of 30 cells is %d wide, wanted the narrowest %d",
			width, narrowestOverlayCard)
	}
	if width := fitCardWidth(140, 70, "", " diagram ", ""); width != 74 {
		t.Errorf("a card of 70 cells is %d wide, wanted 74", width)
	}
	if width := fitCardWidth(100, 300, "", " diagram ", ""); width != 100 {
		t.Errorf("a card of 300 cells is %d wide, wanted the maximum of 100", width)
	}
	keys := "^S stage · ^L NULL · ^E empty · ^D default · Esc cancel"
	if width := fitCardWidth(140, 6, keys, " total ", ""); width != len([]rune(keys))+4 {
		t.Errorf("a card is %d wide, wanted its keys on one row", width)
	}
}

func TestTheCellEditorIsSizedToTheValue(t *testing.T) {
	model := buildOfflineModelFor(t, 160, 48)
	overlay := buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit,
		Cell: app.CellTarget{Column: db.ResultColumn{Name: "total", DataType: "numeric"}},
	}, "379.41")
	model.Active().Overlay = overlay

	card := model.renderCellEditor(overlay, model.resolveOverlayWidth(app.OverlayCellEdit))
	if width := measureStyledWidth(card); width >= model.width*80/100 {
		t.Errorf("the editor of one number is %d cells wide", width)
	}
}

func TestTheWritePlanIsSizedToItsLines(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	model.width = 200
	overlay := app.Overlay{Kind: app.OverlayWritePlan, Title: " write plan ", Plan: buildTestPlan()}
	connection.Overlay = overlay

	card := model.renderWritePlan(overlay, model.resolveOverlayWidth(app.OverlayWritePlan))
	if width := measureStyledWidth(card); width >= 200*86/100 {
		t.Errorf("the plan is %d cells wide on a screen of 200", width)
	}
}
