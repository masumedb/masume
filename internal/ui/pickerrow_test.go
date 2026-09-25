package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

func TestThePickerRowKeepsTheFileNameAndShowsTheDescription(t *testing.T) {
	model := buildOfflineModel(t, 140, 40)
	model.screen = ScreenPickingProfile
	model.connections = openConnections{}
	model.profiles = []cfg.Profile{{
		Name: "shop", Engine: core.EngineSqlite, Environment: cfg.EnvironmentDev,
		Database:    "/srv/data/exports/2026/september/archive/very/deep/folder/shop.db",
		Description: "demo shop",
	}}

	drawn := stripEscapes(model.renderPicker())
	for _, wanted := range []string{"shop.db", "demo shop", "/ filter connections"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the picker does not show %q:\n%s", wanted, drawn)
		}
	}
	if strings.Contains(drawn, "❯ filter") {
		t.Errorf("the filter field uses the mark of the selected row:\n%s", drawn)
	}
}
