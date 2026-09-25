package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

// findRowText returns the column of the first cell after the gutter that has text.
func findRowText(cells []string, from int) int {
	for at := from; at < len(cells); at++ {
		if strings.TrimSpace(cells[at]) != "" {
			return at
		}
	}
	return -1
}

func TestThePointerStandsInAGutterOfItsOwn(t *testing.T) {
	for _, held := range []struct {
		name  string
		build func(*testing.T) (*Model, func(*Model) rowsHit)
		// lead is the column of the list that only some rows fill.
		lead int
	}{
		{
			name: "the palette", lead: paletteGroupWidth,
			build: func(t *testing.T) (*Model, func(*Model) rowsHit) {
				model := buildLoadedModel(t, 1, 3, 8, 3)
				connection := model.Active()
				connection.Overlay = app.Overlay{
					Kind: app.OverlayPalette, Draft: app.NewEditorBuffer("", 0),
					Palette: model.buildPaletteActions(connection),
				}
				return model, func(model *Model) rowsHit { return model.layout.overlayRows }
			},
		},
		{
			name: "the object menu",
			build: func(t *testing.T) (*Model, func(*Model) rowsHit) {
				model, _ := buildObjectMenuModel(t)
				return model, func(model *Model) rowsHit { return model.layout.overlayRows }
			},
		},
		{
			name: "the connection picker",
			build: func(t *testing.T) (*Model, func(*Model) rowsHit) {
				model := buildOfflineModel(t, 120, 34)
				model.screen = ScreenPickingProfile
				model.connections = openConnections{}
				model.profiles = []cfg.Profile{
					{Name: "alpha", Engine: "postgres", Environment: cfg.EnvironmentDev},
					{Name: "bravo", Engine: "mysql", Environment: cfg.EnvironmentProd},
				}
				return model, func(model *Model) rowsHit { return model.layout.pickerRows }
			},
		},
		{
			name: "the completion list",
			build: func(t *testing.T) (*Model, func(*Model) rowsHit) {
				model, _, _ := buildListingModel(t, "select * from ")
				return model, func(model *Model) rowsHit { return model.layout.completionRows }
			},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, readBlock := held.build(t)
			frame := strings.Split(model.render(), "\n")
			block := readBlock(model)
			pointer := model.icons.Icon(cfg.IconPrompt)

			selected := readRowCells(frame[block.top])
			other := readRowCells(frame[block.top+1])
			at := -1
			for column := block.from; column < len(selected); column++ {
				if selected[column] == pointer {
					at = column
					break
				}
			}
			if at < 0 {
				t.Fatalf("the selected row %q has no pointer", strings.Join(selected, ""))
			}
			if selected[at+1] != " " || other[at] != " " || other[at+1] != " " {
				t.Errorf("the gutter reads %q over %q",
					strings.Join(selected[at:at+2], ""), strings.Join(other[at:at+2], ""))
			}
			if first, next := findRowText(selected, at+2+held.lead),
				findRowText(other, at+2+held.lead); first != next {
				t.Errorf("the selected row starts at column %d, the next row at %d", first, next)
			}
		})
	}
}
