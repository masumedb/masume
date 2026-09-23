package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

// buildImportFormOverlay returns the first stage of an import of a CSV file.
func buildImportFormOverlay() app.Overlay {
	overlay := app.Overlay{
		Kind: app.OverlayImport, Draft: app.NewEditorBuffer("", 0),
		Import: app.ImportRequest{Stage: app.ImportFile},
	}
	overlay.Import.Plan.Path = "/tmp/orders.csv"
	overlay.Import.Plan.Options.Format = "csv"
	overlay.Import.Plan.Options.Delimiter = ","
	return overlay
}

func TestEveryCardLeadsWithItsPrimaryButton(t *testing.T) {
	for _, held := range []struct {
		name    string
		open    func(*testing.T) *Model
		primary ActionID
		label   string
	}{
		{
			name: "the connection form",
			open: func(t *testing.T) *Model {
				model := buildOfflineModel(t, 120, 40)
				model.screen = ScreenEditingConnection
				model.form = NewFormState(cfg.Profile{Name: "alpha", Engine: "postgres"}, true, nil)
				return model
			},
			primary: ActionSaveForm, label: "save",
		},
		{
			name: "the password card",
			open: func(t *testing.T) *Model {
				useMockKeyring(t)
				model := buildOfflineModel(t, 120, 40)
				model.screen = ScreenPromptingPassword
				model.picker.askPassword(buildPromptingProfile("shop"))
				return model
			},
			primary: ActionChooseRow, label: "connect",
		},
		{
			name: "the save connection question",
			open: func(t *testing.T) *Model {
				model := buildOfflineModel(t, 120, 40)
				model.recordUnsavedConnection(buildUnsavedProfile("shop"))
				model.readKey(pressCtrlC())
				return model
			},
			primary: ActionAnswerYes, label: "save and quit",
		},
		{
			name: "the import form",
			open: func(t *testing.T) *Model {
				model, connection, _ := buildBatchModel(t)
				connection.Overlay = buildImportFormOverlay()
				return model
			},
			primary: ActionSaveForm, label: "read the file",
		},
		{
			name: "the question over the workspace",
			open: func(t *testing.T) *Model {
				model, connection, _ := buildEditingModel(t, "delete from orders", 0)
				connection.Overlay = app.Overlay{
					Kind: app.OverlayConfirm, Title: " run it ", Body: "this reads no rows back",
				}
				return model
			},
			primary: ActionAnswerYes, label: "run",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model := held.open(t)
			frame := strings.Split(model.render(), "\n")

			button, found := findCardButton(model, held.primary)
			if !found || button.row == model.layout.hintRow || button.row == model.layout.titleRow {
				t.Fatalf("the card has no %q button", held.primary)
			}
			if first := readCardButtons(model, button.row)[0]; first != held.primary {
				t.Errorf("the first button runs %q, wanted %q", first, held.primary)
			}
			if text := cutRowText(frame[button.row], button.from, button.to); !strings.Contains(
				text, held.label) {
				t.Errorf("the primary button reads %q, wanted %q", strings.TrimSpace(text), held.label)
			}
		})
	}
}

func TestAPressOnAnAnswerOfTheSaveQuestionAnswersIt(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.readKey(pressCtrlC())
	model.render()

	button, found := findCardButton(model, ActionAnswerNo)
	if !found {
		t.Fatal("the question has no button for no")
	}
	model.readMouse(tea.MouseClickMsg{X: button.from, Y: button.row, Button: tea.MouseLeft})
	if model.confirm != nil || !model.quitting {
		t.Error("the press on quit without saving did not answer the question")
	}
}

func TestAPressOnCancelLeavesThePasswordCard(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.screen = ScreenPromptingPassword
	model.picker.askPassword(buildPromptingProfile("shop"))
	model.render()

	button, found := findCardButton(model, ActionClose)
	if !found {
		t.Fatal("the password card has no cancel button")
	}
	model.readMouse(tea.MouseClickMsg{X: button.from, Y: button.row, Button: tea.MouseLeft})
	if model.screen == ScreenPromptingPassword {
		t.Error("the press on cancel left the password card open")
	}
}

func TestTheImportFormGroupsItsFieldsAndMarksProduction(t *testing.T) {
	model, connection, _ := buildBatchModel(t)
	connection.Session.(*offlineSession).profile.Environment = cfg.EnvironmentProd
	connection.Overlay = buildImportFormOverlay()
	drawn := stripEscapes(model.render())

	for _, said := range []string{"SOURCE", "PRODUCTION", `delimiter                   ","`} {
		if !strings.Contains(drawn, said) {
			t.Errorf("the import card shows nothing of %q:\n%s", said, drawn)
		}
	}
}

func TestCtrlSReadsTheImportFileFromThePathRow(t *testing.T) {
	model, connection, _ := buildBatchModel(t)
	connection.Overlay = buildImportFormOverlay()
	model.render()

	model.readOverlayKey(connection, tea.Key{Code: 's', Mod: tea.ModCtrl})
	if connection.Overlay.Import.Stage == app.ImportPick {
		t.Error("Ctrl+S on the path row opened the file picker")
	}
	if !connection.Overlay.Import.Running && connection.Overlay.Notice == "" {
		t.Error("Ctrl+S on the path row did not read the file")
	}
}

func TestAPressOnAHeadingRowMarksNoField(t *testing.T) {
	block := rowsHit{top: 10, count: 5, gap: 2, from: 0, to: 20}
	for y, wanted := range map[int]int{10: 0, 11: 1, 13: 2, 14: 3} {
		if item, found := block.holds(5, y); !found || item != wanted {
			t.Errorf("row %d holds item %d, found %v, wanted %d", y, item, found, wanted)
		}
	}
	if _, found := block.holds(5, 12); found {
		t.Error("the heading row holds a field")
	}
}
