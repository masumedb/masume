package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
)

// openQuestionOverAList opens a question over a list of notebooks and returns what the
// answer of the question was given.
func openQuestionOverAList(model *Model, connection *app.Connection) *bool {
	answered := new(bool)
	connection.Open(app.Overlay{
		Kind: app.OverlayNotebooks, Draft: app.NewEditorBuffer("", 0),
	})
	connection.OpenOver(app.Overlay{
		Kind: app.OverlayConfirm, Title: " remove it ", Body: "Remove the file?",
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			*answered = confirmed
			return nil
		}},
	})
	model.render()
	return answered
}

// The key that answers yes closes the question alone, and the card under it returns.
func TestTheKeyForYesReturnsToTheCardUnderTheQuestion(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	answered := openQuestionOverAList(model, connection)

	model.readKey(tea.Key{Code: 'y', Text: "y"})
	if !*answered {
		t.Error("the question was not answered yes")
	}
	if connection.Overlay.Kind != app.OverlayNotebooks {
		t.Errorf("yes left %q, wanted the list", connection.Overlay.Kind)
	}
}

// A press on an answer of a question closes the question alone, as the keys that answer it
// do.
func TestAPressOnAnAnswerReturnsToTheCardUnderIt(t *testing.T) {
	for _, held := range []struct {
		name string
		chip int
		want bool
	}{
		{"run", 0, true},
		{"cancel", 1, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			model := buildOfflineModel(t, 120, 40)
			connection := model.Active()
			answered := openQuestionOverAList(model, connection)
			if len(model.layout.overlayChips) != 2 {
				t.Fatalf("the question drew %d answers", len(model.layout.overlayChips))
			}

			chip := model.layout.overlayChips[held.chip]
			model.readMouse(tea.MouseClickMsg{
				X: chip.from, Y: chip.row, Button: tea.MouseLeft,
			})
			if *answered != held.want {
				t.Errorf("the press on %s answered %v", held.name, *answered)
			}
			if connection.Overlay.Kind != app.OverlayNotebooks {
				t.Errorf("the press on %s left %q, wanted the list",
					held.name, connection.Overlay.Kind)
			}
		})
	}
}
