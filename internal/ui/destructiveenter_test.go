package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

func TestEnterCancelsADestructiveQuestion(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	answered := map[bool]bool{}
	connection.Open(app.Overlay{
		Kind: app.OverlayConfirm, Title: " delete this notebook ", Body: "Delete it?",
		Yes: "delete notebook", Destructive: true,
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			answered[confirmed] = true
			return nil
		}},
	})

	model.chooseOverlayRow(connection, connection.Active(), &connection.Overlay, false)
	if answered[true] || !answered[false] {
		t.Errorf("Enter answered %v, wanted no", answered)
	}
	if connection.Overlay.IsOpen() {
		t.Error("the question stayed open")
	}
}

func TestDiscardInTheReviewAsksFirst(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	tab.Pending.Edits = map[string]core.CellEdit{
		core.BuildEditKey(0, 1): {RowIndex: 0, ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "x"}},
	}
	connection.Open(app.Overlay{Kind: app.OverlayChanges})

	model.runOverlayAction(connection, tab, &connection.Overlay,
		Match{Scope: cfg.ScopeDialog, Action: ActionDiscardChanges})
	if core.CountChanges(tab.Pending) != 1 {
		t.Error("the staged change was discarded without a question")
	}
	if connection.Overlay.Kind != app.OverlayConfirm || !connection.Overlay.Destructive {
		t.Errorf("the card is %q, wanted a destructive question", connection.Overlay.Kind)
	}
}
