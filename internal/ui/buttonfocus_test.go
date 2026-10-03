package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
)

// isButtonFilled is true where the first cell of the button is drawn on the accent or the
// error colour.
func isButtonFilled(model *Model, held buttonHit) bool {
	cell := mapCells(strings.Split(model.render(), "\n")[held.row])[held.from]
	_, ground := readCellColors(cell.sgr)
	theme := model.styles.Theme
	return ground != nil &&
		(WriteHex(ground) == WriteHex(theme.Accent) || WriteHex(ground) == WriteHex(theme.Error))
}

func TestTabMovesTheFocusToTheOtherAnswer(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	answered := openQuestionOverAList(model, connection)
	yes, _ := findCardButton(model, ActionAnswerYes)
	no, _ := findCardButton(model, ActionAnswerNo)
	if !isButtonFilled(model, yes) || isButtonFilled(model, no) {
		t.Fatal("the question did not start with the focus on yes")
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if isButtonFilled(model, yes) || !isButtonFilled(model, no) {
		t.Fatal("tab left the focus on yes")
	}
	*answered = true
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if *answered {
		t.Error("enter on no answered yes")
	}
	if connection.Overlay.Kind != app.OverlayNotebooks {
		t.Errorf("enter left %q, wanted the list", connection.Overlay.Kind)
	}
}

func TestADestructiveQuestionStartsWithTheFocusOnNo(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	answered := false
	connection.Open(app.Overlay{
		Kind: app.OverlayConfirm, Title: " delete this notebook ", Body: "Delete it?",
		Yes: "delete notebook", Destructive: true,
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			answered = confirmed
			return nil
		}},
	})
	model.render()
	yes, _ := findCardButton(model, ActionAnswerYes)
	no, _ := findCardButton(model, ActionAnswerNo)
	if isButtonFilled(model, yes) || !isButtonFilled(model, no) {
		t.Fatal("the destructive question did not start with the focus on no")
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !answered {
		t.Error("enter on yes did not answer yes")
	}
}

func TestTabMovesTheFocusInAQuestionOfTheClient(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.readKey(pressCtrlC())
	if model.confirm == nil {
		t.Fatal("quitting asked nothing")
	}
	model.render()
	no, _ := findCardButton(model, ActionAnswerNo)

	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !isButtonFilled(model, no) {
		t.Fatal("tab left the focus off the second answer")
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.confirm != nil || !model.quitting {
		t.Error("enter on quit without saving did not answer the question")
	}
}

// findFilledButton returns the action of the card button drawn filled, or nothing.
func findFilledButton(model *Model) ActionID {
	for _, held := range model.listCardButtons() {
		if isButtonFilled(model, held) {
			return held.action
		}
	}
	return ""
}

func TestTabFromTheLastFieldReachesTheButtons(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.form = NewFormState(buildPromptingProfile("shop"), true, nil)
	model.screen = ScreenEditingConnection
	model.render()
	if filled := findFilledButton(model); filled != ActionSaveForm {
		t.Fatalf("the form opened with %q filled, wanted save", filled)
	}

	model.form.StepField(len(model.form.Shown()) - 1 - model.form.Cursor)
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model.render()
	if focused := model.resolveButtonFocus(noButtonFocus, 3); focused != 0 {
		t.Fatalf("tab on the last field left the focus at %d", focused)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model.render()
	if filled := findFilledButton(model); filled != ActionTestConnection {
		t.Errorf("the second tab filled %q, wanted test", filled)
	}
	model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if strings.Contains(model.form.Draft.Text, "x") {
		t.Error("a letter typed on a button reached the field")
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model.render()
	if model.resolveButtonFocus(noButtonFocus, 3) != noButtonFocus || model.form.Cursor != 0 {
		t.Errorf("tab past the last button left the focus at field %d", model.form.Cursor)
	}

	model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: uv.ModShift})
	model.render()
	if filled := findFilledButton(model); filled != ActionClose {
		t.Fatalf("shift+tab on the first field filled %q, wanted cancel", filled)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.screen != ScreenPickingProfile {
		t.Errorf("enter on cancel left the screen %v", model.screen)
	}
}

func TestTabReachesTheButtonsOfAList(t *testing.T) {
	model, connection, _ := buildMenuModel(t)
	model.render()
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model.render()
	buttons := model.listCardButtons()
	if len(buttons) == 0 || findFilledButton(model) != buttons[0].action {
		t.Fatalf("tab in the list filled %q", findFilledButton(model))
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	model.render()
	last := buttons[len(buttons)-1].action
	if model.resolveButtonFocus(noButtonFocus, len(buttons)) != noButtonFocus {
		t.Fatal("left on the first button kept the focus on the buttons")
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: uv.ModShift})
	model.render()
	if filled := findFilledButton(model); filled != last {
		t.Fatalf("shift+tab in the list filled %q, wanted %q", filled, last)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if last == ActionClose && connection.Overlay.IsOpen() {
		t.Error("enter on close left the card open")
	}
}
