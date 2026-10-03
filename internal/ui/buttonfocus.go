package ui

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

// buttonFocus is the button of a card with the keyboard focus: the card, and the place of the
// button counted from zero. noButtonFocus is the fields or the list of the card.
type buttonFocus struct {
	card  string
	index int
}

// cardFields is the field cursor of the card on show. A card without fields has a count of
// zero.
type cardFields struct {
	cursor, count int
	step          func(int)
	// True for a card that takes typed text.
	typesText bool
	// True for a card that binds Tab to an action of its own.
	bindsTab bool
}

// describeCard returns a name for the card on show, or nothing on a screen without one. A new
// card starts with the focus of its own start.
func (model *Model) describeCard() string {
	if model.confirm != nil {
		return "confirm " + model.confirm.Title + "\x00" + model.confirm.Body
	}
	switch model.screen {
	case ScreenPickingProfile:
		return "picker"
	case ScreenEditingConnection:
		return "form"
	case ScreenPromptingPassword:
		return "password"
	case ScreenSettings:
		return "settings"
	case ScreenWorking:
		connection := model.Active()
		if connection == nil || !connection.Overlay.IsOpen() {
			return ""
		}
		overlay := connection.Overlay
		return fmt.Sprintf("overlay %d %s %s %s %s", model.ActiveID(), overlay.Kind,
			overlay.Title, overlay.Import.Stage, overlay.Dump.Stage)
	}
	return ""
}

// resolveButtonFocus returns the place of the button with the keyboard focus, out of count
// focusable buttons, or noButtonFocus.
func (model *Model) resolveButtonFocus(start, count int) int {
	index := start
	if card := model.describeCard(); card != "" && model.buttonFocus.card == card {
		index = model.buttonFocus.index
	}
	if count == 0 || index < 0 {
		return noButtonFocus
	}
	return min(index, count-1)
}

// isButtonFocused is true while a button of the card on show has the keyboard focus after a
// move from its fields or its list.
func (model *Model) isButtonFocused() bool {
	card := model.describeCard()
	return card != "" && model.buttonFocus.card == card && model.buttonFocus.index >= 0
}

// listCardButtons returns the buttons at the foot of the card on show, in their order.
func (model *Model) listCardButtons() []buttonHit {
	buttons := []buttonHit{}
	for _, held := range model.layout.buttons {
		if held.slot > 0 {
			buttons = append(buttons, held)
		}
	}
	slices.SortFunc(buttons, func(one, other buttonHit) int { return one.slot - other.slot })
	return buttons
}

// readButtonKey moves the keyboard focus between the fields of the card on show and its
// buttons, and runs the focused button. It reports whether it took the key.
func (model *Model) readButtonKey(key tea.Key, fields cardFields) (bool, tea.Model, tea.Cmd) {
	buttons := model.listCardButtons()
	card := model.describeCard()
	if len(buttons) == 0 || card == "" {
		return false, model, nil
	}
	start := model.layout.cardButtonStart
	focused := model.resolveButtonFocus(start, len(buttons))
	hasContent := start == noButtonFocus
	moveFocus := func(index int) { model.buttonFocus = buttonFocus{card: card, index: index} }

	tab := key.Code == tea.KeyTab && !key.Mod.Contains(uv.ModCtrl) &&
		!key.Mod.Contains(uv.ModAlt)
	back := tab && key.Mod.Contains(uv.ModShift)
	forward := tab && !back

	if focused == noButtonFocus {
		if !tab || fields.bindsTab {
			return false, model, nil
		}
		if forward {
			if fields.count > 0 && fields.cursor < fields.count-1 {
				return false, model, nil
			}
			moveFocus(0)
			return true, model, nil
		}
		if fields.count > 0 && fields.cursor > 0 {
			fields.step(-1)
			return true, model, nil
		}
		moveFocus(len(buttons) - 1)
		return true, model, nil
	}

	leave := func(toFirst bool) {
		moveFocus(noButtonFocus)
		if fields.count == 0 {
			return
		}
		if toFirst {
			fields.step(-fields.cursor)
		} else {
			fields.step(fields.count - 1 - fields.cursor)
		}
	}
	switch {
	case forward || key.Code == tea.KeyRight:
		switch {
		case focused+1 < len(buttons):
			moveFocus(focused + 1)
		case hasContent:
			leave(true)
		default:
			moveFocus(0)
		}
		return true, model, nil
	case back || key.Code == tea.KeyLeft:
		switch {
		case focused > 0:
			moveFocus(focused - 1)
		case hasContent:
			leave(false)
		default:
			moveFocus(len(buttons) - 1)
		}
		return true, model, nil
	case key.Code == tea.KeyUp && hasContent:
		leave(false)
		return true, model, nil
	case key.Code == tea.KeyEnter || key.Code == tea.KeySpace:
		held := buttons[focused]
		next, command := model.runButton(held, held.scope, held.action)
		return true, next, tea.Batch(command, wake(keyFlashWait))
	case key.Code == tea.KeyEscape:
		return false, model, nil
	case fields.typesText && key.Text != "" && !key.Mod.Contains(uv.ModCtrl) &&
		!key.Mod.Contains(uv.ModAlt):
		return true, model, nil
	}
	return false, model, nil
}

// bindsTabOfItsOwn is true where the card of the overlay binds this press of Tab to an action
// other than a step to the next field.
func (model *Model) bindsTabOfItsOwn(overlay app.Overlay, key tea.Key) bool {
	if key.Code != tea.KeyTab {
		return false
	}
	scopes := []cfg.KeyScope{cfg.ScopeDialog}
	if takesListKeys(overlay) {
		scopes = append(scopes, cfg.ScopeList)
	}
	match, matched := model.keymap.MatchOnly(key,
		FindDialogActions(describeOverlayGroup(overlay)), scopes...)
	return matched && match.Action != ActionNextField && match.Action != ActionPreviousField
}

// resolveOverlayFields returns the field cursor of the card of an overlay.
func (model *Model) resolveOverlayFields(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay,
) cardFields {
	fields := cardFields{
		typesText: overlay.Draft != nil,
		step:      func(step int) { stepOverlayField(model, tab, overlay, step) },
	}
	switch overlay.Kind {
	case app.OverlayBuilderField:
		fields.cursor, fields.count = overlay.Field, builderFieldRows
	case app.OverlayBuilderJoin:
		fields.cursor, fields.count = overlay.List.Cursor, builderJoinRows
	case app.OverlayImport:
		fields.cursor, fields.count = overlay.Field, len(BuildImportFields(*overlay))
	case app.OverlayChart:
		fields.cursor, fields.count = overlay.Field, chartFieldCount
	case app.OverlayExport:
		fields.cursor, fields.count = overlay.Field, len(BuildExportFields(*overlay))
	case app.OverlayDump:
		fields.cursor, fields.count = overlay.Field, len(BuildDumpFields(*overlay))
	}
	return fields
}
