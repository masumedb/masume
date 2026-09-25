package ui

import (
	"image/color"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/present"
)

// The widths of one row of the picker. Each part is measured and cut to fit, so a row that
// is too wide does not clip: its parts share the width.
const (
	widestPickerCard    = 96
	narrowestPickerCard = 48
	// The password card holds one field, so it is narrower than the list.
	widestPasswordCard    = 60
	narrowestPasswordCard = 32
	pickerNameWidth       = 24
	pickerEnvWidth        = 4
	// The mark of a connection that is already open, and the blank after it.
	pickerOpenWidth = 2
	// `ro`, or two spaces for a connection that can be written to.
	pickerModeWidth = 2
	// `project`, and the blank after it. The column stands empty where no connection
	// comes from a project file.
	pickerSourceWidth = 8
	// The border, the padding of the card, and the gutter of a row.
	pickerChrome = 7
	pickerGap    = 1
	// The narrowest target column. A card that cannot hold this beside the engine name
	// drops the engine column.
	pickerTargetWidth = 12
	// pickerCardChrome is the border, the blank row inside it, the blank row over the
	// keys, and the row of keys.
	pickerCardChrome = 6
)

// The picker reads these list scope actions while the filter field has the focus.
var pickerListActions = collectScopeActions(cfg.ScopeList)

// pickerActions are the actions the profile picker handles. The dialog scope binds `n` to
// `answer-no` as well as to `new-connection`, so the screen names the ones it takes.
var pickerActions = append(slices.Clone(pickerListActions),
	ActionClose, ActionNewConnection, ActionEditConnection, ActionDeleteConnection,
	ActionFilterConnections)

// readPickerKey returns what one press does in the profile picker.
func (model *Model) readPickerKey(key tea.Key) (tea.Model, tea.Cmd) {
	if model.picker.filtersList() {
		return model.readPickerFilterKey(key)
	}
	// Escape belongs to no action. It closes the picker and goes back to the connection
	// that is open.
	if key.Code == tea.KeyEscape {
		if model.connections.count() > 0 {
			model.screen = ScreenWorking
		}
		return model, nil
	}

	match, matched := model.keymap.MatchOnly(key, pickerActions, cfg.ScopeDialog, cfg.ScopeList)
	if !matched {
		return model, nil
	}
	return model.runPickerAction(match)
}

// readPickerFilterKey handles one key press while the filter field has the focus. List keys
// move the cursor, and every other press goes into the field.
func (model *Model) readPickerFilterKey(key tea.Key) (tea.Model, tea.Cmd) {
	if key.Code == tea.KeyEscape {
		model.picker.stopFilter()
		return model, nil
	}
	if match, matched := model.keymap.MatchOnly(
		key, pickerListActions, cfg.ScopeList); matched {
		return model.runPickerAction(match)
	}
	switch key.Code {
	case tea.KeyBackspace:
		model.picker.filter.DeleteBackward()
	case tea.KeyDelete:
		model.picker.filter.DeleteForward()
	case tea.KeyLeft:
		model.picker.filter.MoveCaret(-1, false)
		return model, nil
	case tea.KeyRight:
		model.picker.filter.MoveCaret(1, false)
		return model, nil
	default:
		if key.Text == "" || key.Mod.Contains(uv.ModCtrl) || key.Mod.Contains(uv.ModAlt) {
			return model, nil
		}
		model.picker.filter.Insert(key.Text)
	}
	model.picker.cursor = 0
	return model, nil
}

// runPickerAction runs one action of the connection picker, whether a key or a press asked
// for it.
func (model *Model) runPickerAction(match Match) (tea.Model, tea.Cmd) {
	count := len(model.shownProfiles())
	switch match.Action {
	case ActionCursorUp:
		model.picker.step(-1, count)
	case ActionCursorDown:
		model.picker.step(1, count)
	case ActionCursorPageUp:
		model.picker.page(-listPage, count)
	case ActionCursorPageDown:
		model.picker.page(listPage, count)
	case ActionCursorFirstRow:
		model.picker.focus(0, count)
	case ActionCursorLastRow:
		model.picker.focus(count-1, count)
	case ActionChooseRow:
		if profile, found := model.pickedProfile(); found {
			return model.chooseProfile(profile)
		}
	case ActionFilterConnections:
		model.picker.startFilter()
	case ActionNewConnection:
		model.form = NewFormState(cfg.Profile{}, false, model.secretStoreNames())
		model.formPicker = nil
		model.screen = ScreenEditingConnection
	case ActionEditConnection:
		if profile, found := model.pickedProfile(); found {
			model.form = NewFormState(profile, true, model.secretStoreNames())
			model.formPicker = nil
			model.screen = ScreenEditingConnection
		}
	case ActionDeleteConnection:
		if profile, found := model.pickedProfile(); found {
			return model.askDeleteProfile(profile)
		}
	}
	return model, nil
}

// listPage is how many rows a page key moves in a list.
const listPage = 10

func wrap(index, count int) int {
	if count <= 0 {
		return 0
	}
	return ((index % count) + count) % count
}

func clamp(index, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index > count-1 {
		return count - 1
	}
	return index
}

// pickerProblemRows is how many faults of the config the picker names before it counts
// the rest.
const pickerProblemRows = 3

// describePickerProblems returns the lines the picker draws for the faults of the config,
// with a blank line over them. A run with none draws nothing.
func describePickerProblems(problems []string) []string {
	if len(problems) == 0 {
		return nil
	}
	lines := []string{""}
	for at, problem := range problems {
		if at == pickerProblemRows {
			lines = append(lines, present.FormatCountOf(
				int64(len(problems)-at), "more problem", "more problems"))
			break
		}
		lines = append(lines, problem)
	}
	return lines
}

// isProfileOpen is true where a connection on this profile is already open.
func (model *Model) isProfileOpen(name string) bool {
	for _, connection := range model.connections.all() {
		if connection.Profile().Name == name {
			return true
		}
	}
	return false
}

// pickedProfile returns the profile the cursor stands on.
func (model *Model) pickedProfile() (cfg.Profile, bool) {
	return model.picker.pick(model.shownProfiles())
}

// shownProfiles returns the profiles the list draws after the filter.
func (model *Model) shownProfiles() []cfg.Profile {
	return model.picker.keepFilteredProfiles(model.profiles)
}

// focusProfile selects the profile of that name. An unlisted name selects the first row.
func (model *Model) focusProfile(name string) {
	shown := model.shownProfiles()
	at, _ := findProfileIndex(shown, name)
	model.picker.focus(at, len(shown))
}

// renderPicker draws the connections of the config file and of the project file, one row
// each. The screen draws its own rows, because a row holds parts that each keep their own
// width.
// measureLongestEngineName returns the columns the longest engine name of the list takes.
func measureLongestEngineName(profiles []cfg.Profile) int {
	longest := 0
	for _, profile := range profiles {
		longest = max(longest, present.MeasureText(string(profile.Engine)))
	}
	return longest
}

// The unfocused filter field shows this placeholder. The focused field is empty, with the
// caret.
const pickerFilterHint = "filter connections"

// renderPickerFilter draws the filter field above the rows. renderCard pads every line, so
// the field is two columns narrower than the card. The match count is drawn beside a filter
// that is set.
func (model *Model) renderPickerFilter(cardWidth, count int) string {
	placeholder := pickerFilterHint
	if model.picker.filtersList() {
		placeholder = ""
	}
	return model.renderFilterLine(model.picker.filter, cardWidth-2, placeholder, count,
		" / ", model.picker.filtersList())
}

func (model *Model) renderPicker() string {
	theme := model.styles.Theme
	profiles := model.shownProfiles()
	cardWidth := present.ResolveCardWidth(widestPickerCard, narrowestPickerCard, model.width)
	// The source column stands empty where no connection comes from a project file, so a
	// user without one loses no room to it.
	sourceWidth := 0
	if slices.ContainsFunc(model.profiles, func(profile cfg.Profile) bool {
		return profile.ProjectFile != ""
	}) {
		sourceWidth = pickerSourceWidth
	}
	fixedWidth := pickerChrome + pickerOpenWidth + pickerNameWidth + pickerEnvWidth +
		pickerModeWidth + sourceWidth + pickerGap*3
	// The engine column is as wide as the longest engine name of every profile, filtered
	// out ones included, so no column moves while the filter changes. It stands only where
	// the target keeps its own room beside it.
	room := cardWidth - fixedWidth
	engineWidth := measureLongestEngineName(model.profiles)
	if room-engineWidth-pickerGap < pickerTargetWidth {
		engineWidth = 0
	} else {
		room -= engineWidth + pickerGap
	}
	targetWidth := max(room, pickerTargetWidth)

	// The filter field and a blank row stand above the list.
	lines := []string{model.renderPickerFilter(cardWidth, len(profiles)), ""}
	if len(profiles) == 0 {
		const prefix = "no connection in "
		empty := prefix + present.TruncatePath(
			cfg.ResolveConfigPath(), cardWidth-4-present.MeasureText(prefix))
		if model.picker.readFilterTerm() != "" {
			empty = "no match"
		}
		lines = append(lines, model.styles.Muted().Render(
			present.TruncateText(empty, cardWidth-4)))
	}

	// Where the rows land on the screen, so a press opens the row it looks like. The card
	// stands in the middle of everything under the title bar, with a blank row inside its
	// border.
	cardRows := len(profiles) + len(lines) + pickerCardChrome +
		len(describePickerProblems(model.problems))
	if model.picker.problem != "" {
		cardRows += 2
	}
	if model.connections.count() > 0 {
		cardRows++
	}
	if model.project.Path != "" {
		cardRows++
	}
	left := halfRoundedUp(model.width - cardWidth)
	// The card stands under the title bar, which takes the first row of the screen.
	cardTop := titleBarRows + halfRoundedUp(model.height-2-cardRows)
	model.layout.pickerRows = rowsHit{
		top:   cardTop + cardBodyRow + len(lines),
		count: len(profiles),
		from:  left + 1, to: left + cardWidth - 2,
	}

	for index, profile := range profiles {
		selected := index == model.picker.cursor
		name := present.FitText(profile.Name, pickerNameWidth)
		environment := present.FitText(string(profile.Environment), pickerEnvWidth)
		mode := "  "
		if profile.AccessMode == cfg.AccessReadOnly {
			mode = "ro"
		}
		source := ""
		if profile.ProjectFile != "" {
			source = "project"
		}
		// The description takes up to half of the room, and the target the rest. A path is
		// cut at its start, which keeps the file name.
		description := ""
		shownTarget := targetWidth
		if profile.Description != "" && targetWidth > 2*pickerTargetWidth {
			description = present.TruncateText(profile.Description, targetWidth/2-pickerGap)
			shownTarget = targetWidth - present.MeasureText(description) - pickerGap
		}
		target := present.TruncateText(cfg.DescribeProfileTarget(profile), shownTarget)
		if core.OpensFile(profile.Engine) {
			target = present.TruncatePath(cfg.DescribeProfileTarget(profile), shownTarget)
		}

		row := lipgloss.NewStyle().Background(theme.Panel)
		nameStyle := model.styles.Ink().Background(theme.Panel)
		envStyle := lipgloss.NewStyle().
			Foreground(model.styles.EnvironmentColor(profile.Environment)).Background(theme.Panel)
		modeStyle := model.styles.Muted().Background(theme.Panel)
		targetStyle := model.styles.Faint().Background(theme.Panel)

		if selected {
			row = row.Background(theme.Accent)
			ink := lipgloss.NewStyle().Foreground(theme.OnAccent).Background(theme.Accent)
			nameStyle, envStyle, modeStyle, targetStyle = ink, ink, ink, ink
		}

		open := " "
		if model.isProfileOpen(profile.Name) {
			open = model.icons.Icon(cfg.IconDot)
		}

		// One line, not five columns, which would share the width and cut every name short.
		written := modeStyle.Render(present.FitText(open, pickerOpenWidth)) +
			nameStyle.Render(name+" ") + envStyle.Render(environment+" ") +
			modeStyle.Render(mode+" ")
		if sourceWidth > 0 {
			written += modeStyle.Render(present.FitText(source, sourceWidth))
		}
		if engineWidth > 0 {
			written += modeStyle.Render(
				present.FitText(string(profile.Engine), engineWidth) + " ")
		}
		if description != "" {
			target = present.PadText(target, shownTarget+pickerGap)
		}
		written += targetStyle.Render(target) + modeStyle.Render(description)
		lines = append(lines, row.Width(cardWidth-2).Render(
			nameStyle.Render(model.buildRowGutter(selected))+written))
	}

	if model.picker.problem != "" {
		lines = append(lines, "", model.styles.Error().Render(
			present.TruncateText(model.picker.problem, cardWidth-4)))
	}
	// A config file the client could not read leaves the list empty, so the card says what
	// the file got wrong rather than letting the empty list stand for it.
	for _, problem := range describePickerProblems(model.problems) {
		lines = append(lines, model.styles.Error().Render(
			present.TruncateText(problem, cardWidth-4)))
	}

	keys := model.buildKeyLineOf(pickerKeySpecs, keyScene{})
	// The keys are cut rather than wrapped, because the card keeps one row for them.
	text := present.TruncateText(keys.buildText(), cardWidth-4)
	lines = model.appendCardKeyRow(
		lines, keys, text, cardTop+cardBodyRow, left+cardBodyColumn)
	if model.project.Path != "" {
		lines = append(lines, model.styles.Muted().Render(
			"project file "+present.TruncatePath(model.project.Path, cardWidth-17)))
	}
	if model.connections.count() > 0 {
		text := model.icons.Icon(cfg.IconDot) + " already open"
		if model.showsKeyHints() && !model.picker.filtersList() {
			text += " · Esc returns to the workspace"
		}
		lines = append(lines, model.styles.Muted().Render(text))
	}

	return model.renderCard(" connections ", cardWidth, lines, plainCard)
}

// renderCard draws a card with its title on the top border.
func (model *Model) renderCard(title string, width int, lines []string, destructive bool) string {
	return model.renderNotedCard(title, "", width, lines, destructive)
}

// renderNotedCard draws a card with its title and a note on the top border.
func (model *Model) renderNotedCard(
	title, note string, width int, lines []string, destructive bool,
) string {
	inner := max(width-4, 1)
	padded := make([]string, 0, len(lines)+2)
	padded = append(padded, "")
	for _, line := range lines {
		padded = append(padded, " "+padStyledOn(line, inner, model.styles.Theme.Panel)+" ")
	}
	padded = append(padded, "")

	return model.styles.RenderBox(BoxOptions{
		Width: width, Height: len(padded) + 2, Title: title, Note: note,
		Focused: true, Destructive: destructive, Lines: padded,
	})
}

// renderPassword draws the field a password is typed into, and the profile it is for.
func (model *Model) renderPassword() string {
	theme := model.styles.Theme
	cardWidth := present.ResolveCardWidth(
		widestPasswordCard, narrowestPasswordCard, model.width)
	profile := model.picker.pending

	opening := "connecting to "
	if model.picker.testsForm {
		opening = "testing "
	}
	lines := []string{
		model.styles.Muted().Render(opening) +
			model.styles.Ink().Render(profile.Name) +
			model.styles.Muted().Render(" · ") +
			paintText(model.styles.EnvironmentColor(profile.Environment), nil, string(profile.Environment)),
		model.styles.Muted().Render(present.TruncateText(
			cfg.DescribeProfileTarget(profile), cardWidth-4)),
		"",
		model.renderField(model.picker.password, cardWidth-4, FieldLook{
			Ground: theme.Header, Ink: theme.Text,
			Masked: true, Focused: !model.picker.keyringFocused, Placeholder: "password",
		}),
	}
	if model.picker.offersKeyring() {
		lines = append(lines, "", model.renderKeyringBox(cardWidth))
	}
	lines = append(lines, "")

	connect := model.buildCardButton(cfg.ScopeList, ActionChooseRow,
		describePasswordUse(keyScene{model: model}))
	connect.primary = true
	cardRows := len(lines) + 1 + present.CardChrome
	left := halfRoundedUp(model.width - cardWidth)
	cardTop := titleBarRows + halfRoundedUp(model.height-2-cardRows)
	lines = append(lines, model.renderButtonRow([]cardButton{
		connect, model.buildCardButton(cfg.ScopeDialog, ActionClose, "cancel"),
	}, cardTop+cardBodyRow+len(lines), left+cardBodyColumn))
	return model.renderNotedCard(" password ",
		model.renderEnvironmentBadge(profile.Environment), cardWidth, lines, plainCard)
}

// renderKeyringBox draws the box that keeps the typed password in the keyring of the
// operating system.
func (model *Model) renderKeyringBox(cardWidth int) string {
	mark := "[ ]"
	style := model.styles.Muted()
	if model.picker.keepInKeyring {
		mark = "[x]"
		style = model.styles.Ink()
	}
	if model.picker.keyringFocused {
		style = model.styles.Accent()
	}
	return style.Render(present.TruncateText(
		mark+" remember in the keyring", cardWidth-4))
}

// FieldLook says how one field of a form or a card is drawn.
type FieldLook struct {
	// Ground is what the field stands on. Only the field being typed into takes a ground
	// of its own, so a row of fields does not read as one dark block.
	Ground color.Color
	// Ink is the colour of the value. A field without the caret is quieter.
	Ink color.Color
	// Masked writes a dot per character, for a password.
	Masked bool
	// Focused is true for the field the caret is in, which draws the caret.
	Focused bool
	// Placeholder stands where the field is empty and does not hold the caret.
	Placeholder string
	// KeepsPlaceholder draws the placeholder in a field that is empty and holds the caret,
	// which is what the question of the chat does.
	KeepsPlaceholder bool
}

// renderDraftRows draws a multiline field and keeps the caret visible.
func (model *Model) renderDraftRows(
	buffer *app.EditorBuffer, width, rows int, look FieldLook,
) []string {
	if rows < 1 {
		rows = 1
	}
	if buffer.Text == "" {
		return append([]string{model.renderField(buffer, width, look)},
			buildBlankRows(look.Ground, width, rows-1)...)
	}

	held := buffer.Lines()
	caretLine, caretColumn := buffer.CaretPosition()
	offset := scrollTo(caretLine, 0, rows, len(held))

	drawn := make([]string, 0, rows)
	for at := offset; at < len(held) && len(drawn) < rows; at++ {
		drawn = append(drawn, model.renderDraftRow(
			held[at], width, look, at == caretLine, caretColumn))
	}
	return append(drawn, buildBlankRows(look.Ground, width, rows-len(drawn))...)
}

// renderDraftRow draws one line of a draft, with the caret on its cell where it stands on this
// line.
func (model *Model) renderDraftRow(
	line string, width int, look FieldLook, holdsCaret bool, caretColumn int,
) string {
	theme := model.styles.Theme
	// The field holds what the server sent, so what is drawn from it is made safe first.
	// The buffer keeps the value as it stands, because a save writes it back.
	line = present.SafeText(line)
	if !holdsCaret || !look.Focused {
		return padStyledOn(paintText(look.Ink, look.Ground,
			present.TruncateText(line, width)), width, look.Ground)
	}

	head, under, tail := splitAtCaret(line, len([]rune(line[:min(caretColumn, len(line))])))
	return padStyledOn(truncateStyled(paintText(look.Ink, look.Ground, head)+
		paintText(theme.OnAccent, theme.Accent, under)+
		paintText(look.Ink, look.Ground, tail), width), width, look.Ground)
}

// splitAtCaret returns the text before the caret, the character under it and the text after
// it. A caret at the end of the line stands on a blank, so the field always draws one.
func splitAtCaret(line string, caret int) (string, string, string) {
	runes := []rune(line)
	head := string(runes[:min(max(caret, 0), len(runes))])
	if caret < 0 || caret >= len(runes) {
		return head, " ", ""
	}
	return head, string(runes[caret]), string(runes[caret+1:])
}

// buildBlankRows returns the rows of a field that hold no line of the draft.
func buildBlankRows(ground color.Color, width, rows int) []string {
	blanks := make([]string, 0, max(0, rows))
	for range rows {
		blanks = append(blanks, paintBlanks(ground, width))
	}
	return blanks
}

// renderField draws a one-line field, with the caret where it stands.
func (model *Model) renderField(
	buffer *app.EditorBuffer, width int, look FieldLook,
) string {
	theme := model.styles.Theme
	style := lipgloss.NewStyle().Background(look.Ground).Foreground(look.Ink)

	written := buffer.Text
	if look.Masked {
		written = strings.Repeat("•", len([]rune(written)))
	}
	if written == "" && (!look.Focused || look.KeepsPlaceholder) {
		placeholder := model.styles.Muted().Background(look.Ground).
			Render(present.TruncateText(look.Placeholder, width))
		if !look.Focused {
			return style.Width(width).Render(placeholder)
		}
		// The caret stands on the first cell of the placeholder, because that is where a
		// typed character lands.
		return style.Width(width).Render(paintCaretOverStart(
			placeholder, theme.Accent, theme.OnAccent))
	}
	if !look.Focused {
		return style.Width(width).Render(present.TruncateText(written, width))
	}

	// The caret is drawn on the cell it stands on, so the field needs no cursor of its own.
	caret := len([]rune(written))
	if !look.Masked {
		caret = len([]rune(buffer.Text[:min(buffer.Caret, len(buffer.Text))]))
	}
	head, under, tail := splitAtCaret(written, caret)
	head = present.TruncateTextStart(head, width-present.MeasureText(under))
	tail = present.TruncateText(tail, width-present.MeasureText(head+under))
	caretStyle := lipgloss.NewStyle().Background(theme.Accent).Foreground(theme.OnAccent)
	return style.Width(width).Render(head + caretStyle.Render(under) + tail)
}

// readPasswordKey returns what one press does in the password prompt.
func (model *Model) readPasswordKey(key tea.Key) (tea.Model, tea.Cmd) {
	if match, matched := model.keymap.MatchOnly(key, FindDialogActions("password"),
		cfg.ScopeDialog, cfg.ScopeList); matched {
		if held, command, ran := model.runPasswordAction(match.Action); ran {
			return held, command
		}
	}

	if key.Code == tea.KeyEscape {
		model.leavePasswordPrompt()
		return model, nil
	}
	if model.picker.keyringFocused {
		return model, nil
	}
	switch key.Code {
	case tea.KeyBackspace:
		model.picker.password.DeleteBackward()
		return model, nil
	case tea.KeyDelete:
		model.picker.password.DeleteForward()
		return model, nil
	case tea.KeyLeft:
		model.picker.password.MoveCaret(-1, false)
		return model, nil
	case tea.KeyRight:
		model.picker.password.MoveCaret(1, false)
		return model, nil
	}
	if key.Text != "" && !key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		model.picker.password.Insert(key.Text)
	}
	return model, nil
}

// runPasswordAction runs one action of the password card, and reports whether the action
// belonged to the card.
func (model *Model) runPasswordAction(action ActionID) (tea.Model, tea.Cmd, bool) {
	picker := &model.picker
	switch action {
	case ActionClose:
		model.leavePasswordPrompt()
		return model, nil, true
	case ActionPreviousField, ActionNextField:
		picker.keyringFocused = picker.offersKeyring() && !picker.keyringFocused
		return model, nil, true
	case ActionToggleValue:
		if !picker.keyringFocused {
			return model, nil, false
		}
		picker.keepInKeyring = !picker.keepInKeyring
		return model, nil, true
	case ActionChooseRow:
		if picker.testsForm {
			held, command := model.testFormWithTypedPassword()
			return held, command, true
		}
		model.screen = ScreenConnecting
		return model, connect(model.adapters, picker.pending, picker.password.Text), true
	}
	return model, nil, false
}

// pressPassword runs the button of the password card a press landed on.
func (model *Model) pressPassword(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	_, action, key, pressed := findButton(model.layout.buttons, mouse.X, mouse.Y)
	if !pressed {
		return model, nil
	}
	model.frame.flashKey(key)
	held, command, _ := model.runPasswordAction(action)
	return held, command
}

// leavePasswordPrompt returns to the screen that asked for the password.
func (model *Model) leavePasswordPrompt() {
	if model.picker.testsForm {
		model.picker.testsForm = false
		if model.form != nil {
			model.screen = ScreenEditingConnection
			return
		}
	}
	model.screen = ScreenPickingProfile
}

// pasteIntoPassword writes what the terminal pasted into the password field.
func (model *Model) pasteIntoPassword(written string) (tea.Model, tea.Cmd) {
	if model.confirm != nil {
		return model, nil
	}
	model.picker.password.Insert(flattenPaste(written))
	return model, nil
}

// flattenPaste turns the breaks of a paste into spaces, because a field is one line.
func flattenPaste(written string) string {
	written = strings.ReplaceAll(strings.ReplaceAll(written, "\r\n", " "), "\r", " ")
	return strings.ReplaceAll(written, "\n", " ")
}
