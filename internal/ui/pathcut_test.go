package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/filepicker"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query/result"
)

const longExportPath = "/tmp/claude-1000/-home-turan-Projects-masume-masume-go/scratchpad/app/orders.csv"

// A path wider than the export card is cut from the start, and the file name stays in view
// with the caret after it and without it.
func TestTheExportFileFieldKeepsTheFileName(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayExport, Field: exportPathField,
		Export: app.ExportRequest{
			Path: longExportPath, Format: result.ExportCSV, CSV: result.DefaultCSVOptions(),
		},
		Draft: app.NewEditorBuffer(longExportPath, len(longExportPath)),
	}

	for _, field := range []int{exportPathField, exportPathField + 1} {
		connection.Overlay.Field = field
		card := strings.Split(model.renderExport(connection.Overlay, 70), "\n")
		row := stripEscapes(card[2])
		if !strings.Contains(row, "/app/orders.csv") {
			t.Errorf("with the cursor on field %d the file row is %q", field, row)
		}
		if present.MeasureText(row) != 70 {
			t.Errorf("with the cursor on field %d the file row measures %d cells, wanted 70",
				field, present.MeasureText(row))
		}
	}
}

// The directory over the files of a picker keeps its end.
func TestThePickerDirectoryKeepsItsEnd(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	picker := filepicker.New()
	picker.CurrentDirectory = "/tmp/claude-1000/-home-turan-Projects-masume-masume-go/scratchpad/app"

	header := stripEscapes(model.buildPickerLines(&picker, 30)[0])
	if header != "…/scratchpad/app" {
		t.Errorf("the directory reads %q, wanted %q", header, "…/scratchpad/app")
	}
}
