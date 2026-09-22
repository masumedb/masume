package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

// A terminal narrower than the tree plus a readable pane draws the tree and the pane side by
// side, and the two together measure wider than the screen. Every row of the frame has to
// keep the width of the screen, or the borders of the panes break.
func TestANarrowTerminalDrawsNoRowWiderThanItself(t *testing.T) {
	for _, width := range []int{24, 30, 36, 44, 60} {
		model := buildOfflineModel(t, width, 20)
		if !model.Active().SidebarVisible {
			t.Fatal("the tree is not on screen, so the narrow frame is not measured")
		}
		for at, row := range readFrameRows(model.View().Content) {
			if measured := measureStyledWidth(row); measured > width {
				t.Errorf("at width %d row %d measures %d: %q",
					width, at, measured, strings.TrimRight(row, " "))
			}
		}
	}
}

// A question longer than the screen is cut to the rows that fit, and the rows it cut are
// counted. Without the count the reader answers a question whose body was silently short.
func TestALongQuestionIsCutToTheScreenAndCountsTheRowsItCut(t *testing.T) {
	model := buildOfflineModel(t, 80, 14)
	held, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	model = held.(*Model)

	body := []string{"Save 40 connections to the config file?", ""}
	for at := range 40 {
		body = append(body, "shop-"+string(rune('a'+at%26))+"  postgres://host/shop")
	}
	model.confirm = &confirmState{
		Title: " save ", Body: strings.Join(body, "\n"), Yes: "save", No: "discard",
	}

	lines := model.buildConfirmLines(model.confirm, 64)
	if len(lines) > model.height-confirmCardChrome {
		t.Fatalf("the question holds %d rows on a screen of %d",
			len(lines), model.height)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "more rows") {
		t.Errorf("the question counts no cut rows: %q", joined)
	}
	if !strings.Contains(joined, "save") {
		t.Errorf("the question lost its key row: %q", joined)
	}
}

// A card once kept a floor of 40 columns after the clamp to the screen, so on a narrower
// terminal every card drew past the right edge and lost its border.
func TestEveryCardFitsATerminalNarrowerThanTheNarrowestCard(t *testing.T) {
	kinds := []app.OverlayKind{
		app.OverlayHelp, app.OverlayPalette, app.OverlayAiChat, app.OverlayActivity,
		app.OverlayRowDetail, app.OverlayConfirm, app.OverlayExport, app.OverlayImport,
		app.OverlayCellEdit, app.OverlayThemePicker, app.OverlayNotebooks,
		app.OverlayHistory, app.OverlaySaved, app.OverlayChart, app.OverlayValueFilter,
	}
	for _, width := range []int{20, 24, 30, 36, 39, 40, 80} {
		model := buildOfflineModel(t, width, 24)
		for _, kind := range kinds {
			got := model.resolveOverlayWidth(kind)
			if got > width {
				t.Errorf("on a screen of %d the %s card draws %d wide", width, kind, got)
			}
			if got < 1 {
				t.Errorf("on a screen of %d the %s card draws %d wide", width, kind, got)
			}
		}
	}
}

// A card that scrolls writes its bar over the last column of each row. On a terminal
// narrower than the border of the card that column stood before the first one, and the
// frame of the whole client failed to draw.
func TestACardDrawsOnATerminalNarrowerThanItsBorder(t *testing.T) {
	for _, width := range []int{1, 2, 3, 4, 6, 10} {
		for _, height := range []int{1, 3, 8, 24} {
			model := buildOfflineModel(t, 120, 34)
			connection := model.Active()
			tab := connection.Active()
			tab.Results.Start([]string{"select 1"}, 100)
			tab.Results.Succeed(0, db.ComposedRead{Text: "select 1"}, db.QueryResult{
				Columns: []db.ResultColumn{
					{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"},
				},
				Rows: [][]any{{int64(1), "one"}, {int64(2), "two"}},
			})
			shape := model.buildGridShape(connection, tab)
			connection.Open(app.Overlay{
				Kind: app.OverlayRowDetail,
				Window: app.RowWindow{
					Columns: shape.Columns, Rows: shape.Rows, Index: 0,
				},
			})

			held, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
			model = held.(*Model)
			if frame := model.View().Content; frame == "" && width > 2 {
				t.Errorf("the frame at %dx%d is empty", width, height)
			}
		}
	}
}
