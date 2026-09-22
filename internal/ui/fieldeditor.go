package ui

import (
	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

// A form of fields, the caret in them, and the text being typed into one. The connection
// form and the settings screen share it.

// fieldEditor holds the fields of one form and the caret in them.
type fieldEditor struct {
	Fields []cfg.FormField
	// Which of the shown fields holds the caret.
	Cursor int
	// The text of the field under the caret.
	Draft *app.EditorBuffer
	// shown returns the fields the form draws now, which follow what the other fields hold.
	shown func([]cfg.FormField) []cfg.FormField
	// apply writes one changed field, and the fields that follow it.
	apply func([]cfg.FormField, string, string) []cfg.FormField
}

// Shown returns the fields the form draws now.
func (editor *fieldEditor) Shown() []cfg.FormField {
	return editor.shown(editor.Fields)
}

// openField puts the text of the field under the caret into the draft.
func (editor *fieldEditor) openField() {
	shown := editor.Shown()
	if len(shown) == 0 {
		editor.Draft = app.NewEditorBuffer("", 0)
		return
	}
	if editor.Cursor >= len(shown) {
		editor.Cursor = len(shown) - 1
	}
	if editor.Cursor < 0 {
		editor.Cursor = 0
	}
	value := shown[editor.Cursor].Value
	editor.Draft = app.NewEditorBuffer(value, len(value))
}

// findFocusedField returns the field under the caret, and false where the form shows none.
func (editor *fieldEditor) findFocusedField() (cfg.FormField, bool) {
	shown := editor.Shown()
	if editor.Cursor < 0 || editor.Cursor >= len(shown) {
		return cfg.FormField{}, false
	}
	return shown[editor.Cursor], true
}

// keepField writes the draft back into the field under the caret.
func (editor *fieldEditor) keepField() {
	shown := editor.Shown()
	if editor.Cursor < 0 || editor.Cursor >= len(shown) {
		return
	}
	editor.Fields = editor.apply(editor.Fields, shown[editor.Cursor].Key, editor.Draft.Text)
}

// StepField moves the caret to another field, and keeps what was typed into this one.
func (editor *fieldEditor) StepField(step int) {
	editor.keepField()
	shown := editor.Shown()
	editor.Cursor = wrap(editor.Cursor+step, len(shown))
	editor.openField()
}

// StepChoice steps a field that offers a list of values through them.
func (editor *fieldEditor) StepChoice(step int) bool {
	shown := editor.Shown()
	if editor.Cursor < 0 || editor.Cursor >= len(shown) {
		return false
	}
	field := shown[editor.Cursor]
	if len(field.Choices) == 0 {
		return false
	}

	at := 0
	for index, choice := range field.Choices {
		if choice == editor.Draft.Text {
			at = index
			break
		}
	}
	wanted := field.Choices[wrap(at+step, len(field.Choices))]
	editor.Fields = editor.apply(editor.Fields, field.Key, wanted)
	editor.openField()
	return true
}
