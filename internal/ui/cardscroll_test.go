package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

func TestAMessageTallerThanTheCardScrolls(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	model.height = 24
	connection := model.Active()
	lines := make([]string, 0, 40)
	for at := range 40 {
		lines = append(lines, "problem "+strconv.Itoa(at+1))
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: " config problems ", Body: strings.Join(lines, "\n"),
	}

	drawn := stripEscapes(model.renderMessage(connection.Overlay, 60))
	if !strings.Contains(drawn, "scroll") || strings.Contains(drawn, "problem 40") {
		t.Fatalf("the card does not scroll:\n%s", drawn)
	}
	model.runOverlayAction(connection, connection.Active(), &connection.Overlay,
		Match{Scope: cfg.ScopeList, Action: ActionCursorLastRow})
	if drawn := stripEscapes(model.renderMessage(connection.Overlay, 60)); !strings.Contains(drawn, "problem 40") {
		t.Errorf("the last line is not reached:\n%s", drawn)
	}
}

func TestTheCellValueListKeepsTheCursorInView(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	model.height = 24
	connection := model.Active()
	choices := make([]string, 0, 60)
	for at := range 60 {
		choices = append(choices, "value "+strconv.Itoa(at+1))
	}
	connection.Overlay = app.Overlay{Kind: app.OverlayCellEdit, List: app.ListState{Cursor: 59}}
	connection.Overlay.Cell.Choices = choices
	connection.Overlay.ContentRows = len(choices)

	if drawn := stripEscapes(model.renderCellEditor(connection.Overlay, 60)); !strings.Contains(drawn, "value 60") {
		t.Errorf("the value under the cursor is not drawn:\n%s", drawn)
	}
}

func TestASettingsPageTallerThanTheCardDrawsABar(t *testing.T) {
	model := buildOfflineModel(t, 120, 24)
	openSettings(t, model)
	showSection(t, model, cfg.SectionKeys)
	openPage(t, model, string(cfg.ScopeGlobal))

	if drawn := stripEscapes(model.renderSettings()); !strings.ContainsAny(drawn, thumbFull+thumbUpper+thumbLower) {
		t.Errorf("the page of keys draws no bar:\n%s", drawn)
	}
}

func TestTheFilePickerSaysWhereTheCursorIs(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	picker := newFilePicker("/data", nil)
	for at := range 30 {
		picker.rows = append(picker.rows, fileRow{name: "file" + strconv.Itoa(at) + ".csv"})
	}
	picker.cursor = 4

	if heading := stripEscapes(model.buildPickerLines(&picker, 60)[0]); !strings.HasSuffix(heading, "5 of 30") {
		t.Errorf("the heading reads %q", heading)
	}
}
