package ui

import (
	"os"
	"path/filepath"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/present"
)

// The connection form picks a certificate file with the picker the import card uses.

// formPickerState is the file picker the connection form has open on one field.
type formPickerState struct {
	// field is the key of the field the chosen path is written into.
	field  string
	label  string
	picker filepicker.Model
}

// openFormFilePicker opens the picker on that field, and returns the command that reads the
// directory it opens in. The picker offers every file: a certificate has no one extension.
func (model *Model) openFormFilePicker(field cfg.FormField) tea.Cmd {
	picker := model.buildFilePicker(nil)
	if directory := findPathDirectory(model.form.Draft.Text); directory != "" {
		picker.CurrentDirectory = directory
	}
	model.formPicker = &formPickerState{
		field: field.Key, label: field.Label, picker: picker,
	}
	model.layout.formRows = rowsHit{}
	model.layout.formChoices = nil
	return picker.Init()
}

// findPathDirectory returns the directory of the path the field holds, and an empty string
// where the field holds no path to a readable directory.
func findPathDirectory(written string) string {
	if written == "" {
		return ""
	}
	directory := filepath.Dir(core.ExpandHomePath(written))
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return ""
	}
	return directory
}

// closeFormFilePicker returns to the form without changing the field.
func (model *Model) closeFormFilePicker() {
	model.formPicker = nil
}

// readFormPickerKey hands a press to the picker of the form. A press bound to one of the
// keys of the picker reaches it as that key.
func (model *Model) readFormPickerKey(key tea.Key) (tea.Model, tea.Cmd) {
	if key.Code == tea.KeyEscape {
		model.closeFormFilePicker()
		return model, nil
	}
	held := key
	if match, matched := model.keymap.MatchOnly(key,
		FindDialogActions(importPickGroup), cfg.ScopeDialog, cfg.ScopeList); matched {
		if match.Action == ActionClose {
			model.closeFormFilePicker()
			return model, nil
		}
		if code, known := pickerKeyOfAction[match.Action]; known {
			held = tea.Key{Code: code}
		}
	}
	updated, command, _ := model.readFormPickerMessage(tea.KeyPressMsg(held))
	return updated, command
}

// readFormPickerMessage hands a message to the picker of the form, which reads a directory
// with a command of its own.
func (model *Model) readFormPickerMessage(message tea.Msg) (tea.Model, tea.Cmd, bool) {
	held := model.formPicker
	if held == nil || model.form == nil {
		return model, nil, false
	}

	picker, command := held.picker.Update(message)
	held.picker = picker
	chosen, path := picker.DidSelectFile(message)
	if !chosen {
		return model, command, true
	}
	model.writeFormFilePath(held.field, path)
	model.closeFormFilePicker()
	return model, nil, true
}

// writeFormFilePath takes the file the user picked into the field. A path under the home
// directory is written with `~`, as the config file holds it.
func (model *Model) writeFormFilePath(key, path string) {
	form := model.form
	form.Fields = form.apply(form.Fields, key, core.ShortenHomePath(path))
	form.openField()
}

// picksFormFile is true while the form stands on a field a file is picked for.
func picksFormFile(scene keyScene) bool {
	if scene.model == nil || scene.model.form == nil {
		return false
	}
	form := scene.model.form
	field, focused := form.findFocusedField()
	return focused && cfg.TakesFilePath(form.Fields, field.Key)
}

// renderFormPicker draws the card the file of one field is chosen in.
func (model *Model) renderFormPicker() string {
	held := model.formPicker
	cardWidth := present.ResolveCardWidth(widestFormCard, narrowestFormCard, model.width)
	lines := model.buildPickerLines(&held.picker, cardWidth-present.CardChrome)

	keys := model.buildKeyLineOf(importPickKeySpecs, keyScene{})
	if text := present.TruncateText(keys.buildText(), cardWidth-4); text != "" {
		lines = append(lines, "", model.styles.Muted().Render(text))
	}
	return model.renderCard(" "+held.label+" ", cardWidth, lines, plainCard)
}
