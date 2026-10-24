package ui

import (
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query/build"
)

// The widths a card of an overlay is drawn at.
const (
	widestOverlayCard    = 96
	narrowestOverlayCard = 40
)

// renderOverlayOver draws the overlay in the middle of the frame it covers.
func (model *Model) renderOverlayOver(
	connection *app.Connection, tab *app.Tab, frame []string, height int,
) []string {
	// A card records what it draws in the cells of its own block, because only this knows
	// where the block lands on the screen.
	placedBars := len(model.layout.scrollbars)
	placedKeys := len(model.layout.buttons)
	card := model.drawCard(func() string { return model.renderOverlay(connection, tab, height) })
	if card == "" {
		model.layout.scrollbars = model.layout.scrollbars[:placedBars]
		model.layout.buttons = model.layout.buttons[:placedKeys]
		return frame
	}
	// The card is drawn over the panes, which keep standing around it, so the reader
	// still sees what the overlay is about.
	rows := strings.Split(card, "\n")
	// The card is centred over everything under the title bar, the bottom bar included,
	// so the odd row sits over the card and not under it.
	top := max(halfRoundedUp(model.height-1-len(rows)), 0)
	left := max(halfRoundedUp(model.width-measureStyledWidth(card)), 0)
	// The frame this card is placed on is the workspace, and the title bar is put over it
	// afterwards, so a row of the screen is one more than a row of the frame.
	screenTop := top + titleBarRows

	// The inside of the card is the block a drag over it stays in, and it stands over the
	// panes, so it is looked at first.
	model.keepCardBlock(card, left, screenTop)

	model.placeCardHits(left, screenTop, placedBars, placedKeys)
	// A card owns the pointer while it is open, so the keys and the bars the panes behind
	// it drew are dropped. They are covered by the card or stand beside it, and a press or
	// a mark on one of them belongs to a pane the reader is not in.
	model.layout.buttons = append([]buttonHit{}, model.layout.buttons[placedKeys:]...)
	model.layout.scrollbars = append([]scrollHit{}, model.layout.scrollbars[placedBars:]...)
	theme := model.styles.Theme
	return placeOver(dimRows(frame, theme.Faint, theme.Background), card, left, top,
		theme.Background)
}

// keepCardBlock records the text cells of a card as the first selection block. The block
// leaves out the border and the blank column inside it.
func (model *Model) keepCardBlock(card string, left, top int) {
	rows := strings.Count(card, "\n") + 1
	block := blockRect{
		fromX: left + cardBodyColumn, toX: left + measureStyledWidth(card) - 1 - cardBodyColumn,
		fromY: top + 1, toY: top + rows - 2,
	}
	model.layout.selectionBlocks = append([]blockRect{block}, model.layout.selectionBlocks...)
}

// centerCard centres a card on the screen body and records its selection block.
func (model *Model) centerCard(card string, body int) []string {
	rows := strings.Count(card, "\n") + 1
	model.keepCardBlock(card, halfRoundedUp(model.width-measureStyledWidth(card)),
		titleBarRows+halfRoundedUp(body-rows))
	return model.styles.CenterRowsOn(card, model.width, body)
}

// placeCardHits carries every hit box a card recorded from the cells of the card onto the
// cells of the screen. A card is drawn before it is placed, so it counts from its own top
// left corner and this is where the corner is learnt.
func (model *Model) placeCardHits(left, top, placedBars, placedKeys int) {
	layout := &model.layout
	for at := placedBars; at < len(layout.scrollbars); at++ {
		layout.scrollbars[at].column += left
		layout.scrollbars[at].top += top
	}
	for at := placedKeys; at < len(layout.buttons); at++ {
		layout.buttons[at].row += top
		layout.buttons[at].from += left
		layout.buttons[at].to += left
		layout.buttons[at].keyTo += left
	}
	for _, chips := range [][]chipHit{layout.overlayChips, layout.activityHeadings} {
		for at := range chips {
			chips[at].row += top
			chips[at].from += left
			chips[at].to += left
		}
	}
	for at := range layout.formChoices {
		layout.formChoices[at].row += top
		layout.formChoices[at].back += left
		layout.formChoices[at].on += left
	}
	layout.cardBodyTop += top
	layout.cardBodyLeft += left
	for _, block := range []*rowsHit{&layout.overlayRows, &layout.formRows} {
		if block.count < 1 {
			continue
		}
		block.top += top
		block.from += left
		block.to += left
	}
}

// overlayShares name the share of the screen each card takes, where it takes a share
// rather than a fixed width. A card that lists statements is wide; one that asks a
// question is not.
var overlayShares = map[app.OverlayKind]int{
	app.OverlayPalette:       70,
	app.OverlayHistory:       92,
	app.OverlaySaved:         86,
	app.OverlayNotebooks:     86,
	app.OverlayChart:         66,
	app.OverlayActivity:      92,
	app.OverlayDiagram:       92,
	app.OverlayChanges:       86,
	app.OverlayWritePlan:     86,
	app.OverlayCellEdit:      80,
	app.OverlayCell:          80,
	app.OverlayValueFilter:   60,
	app.OverlayColumns:       60,
	app.OverlayBuilderTables: 60,
	app.OverlayParameters:    60,
	app.OverlayThemePicker:   52,
	app.OverlayAiChat:        80,
	app.OverlayAiChats:       88,
}

// overlayWidths name the cards that keep one width, whatever the screen is.
var overlayWidths = map[app.OverlayKind]int{
	app.OverlayHelp:         78,
	app.OverlayExport:       76,
	app.OverlayImport:       84,
	app.OverlayDump:         84,
	app.OverlayConfirm:      76,
	app.OverlayChoice:       76,
	app.OverlayPrompt:       76,
	app.OverlayBuilderJoin:  72,
	app.OverlayBuilderField: 72,
	app.OverlayMessage:      72,
	app.OverlayObjectMenu:   72,
	app.OverlayActionMenu:   72,
	app.OverlayCopyMenu:     70,
	app.OverlayRowDetail:    92,
}

// resolveOverlayWidth returns how wide this card draws. The screen is the limit,
// because a wider card would draw over the border of the pane under it.
func (model *Model) resolveOverlayWidth(kind app.OverlayKind) int {
	wanted := widestOverlayCard
	if share, held := overlayShares[kind]; held {
		wanted = model.width * share / 100
	}
	if fixed, held := overlayWidths[kind]; held {
		wanted = fixed
	}
	if wanted < narrowestOverlayCard {
		wanted = narrowestOverlayCard
	}
	// The screen is the last limit, so a terminal under the narrowest card still fits.
	return max(min(wanted, model.width), 1)
}

// cardTitleChrome is the cells the top border takes around the title and the note.
const cardTitleChrome = 6

// fitCardWidth returns the width of a card sized to its widest line, its keys and its title.
// The width the card is given is the maximum, and narrowestOverlayCard is the minimum.
func fitCardWidth(width, content, keys int, title, note string) int {
	wanted := max(content+present.CardChrome, keys+present.CardChrome,
		measureStyledWidth(title)+measureStyledWidth(note)+cardTitleChrome)
	return min(max(wanted, narrowestOverlayCard), width)
}

// measureCardLines returns the cells of the widest drawn line, without the padding at its end.
func measureCardLines(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, present.MeasureText(strings.TrimRight(ansi.Strip(line), " ")))
	}
	return widest
}

// The largest share of the screen a card may take, and the height under which a card is
// too small to read as a card. A card that filters a list takes the largest height, or the
// list would grow and shrink while the user types.
const (
	widestOverlayHeightShare = 70
	narrowestOverlayRow      = 7
)

// overlayHeightShares name the share of the screen height each card takes.
var overlayHeightShares = map[app.OverlayKind]int{
	app.OverlayHelp:        widestOverlayHeightShare,
	app.OverlayPalette:     widestOverlayHeightShare,
	app.OverlayHistory:     widestOverlayHeightShare,
	app.OverlaySaved:       widestOverlayHeightShare,
	app.OverlayNotebooks:   widestOverlayHeightShare,
	app.OverlayActivity:    widestOverlayHeightShare,
	app.OverlayThemePicker: widestOverlayHeightShare,
	app.OverlayObjectMenu:  60,
	app.OverlayActionMenu:  60,
	app.OverlayAiChat:      80,
	app.OverlayAiChats:     widestOverlayHeightShare,
}

// resolveOverlayHeight returns how tall one card draws: the share of the screen it is given,
// or the rows of what it shows with the chrome and the keys around them.
// The largest share is the ceiling, and a row more is taken where the rows left over cannot
// be shared evenly.
func (model *Model) resolveOverlayHeight(
	kind app.OverlayKind, contentRows, hintRows int,
) int {
	ceiling := model.height * widestOverlayHeightShare / 100
	asked := ceiling
	switch share, held := overlayHeightShares[kind]; {
	case held:
		asked = model.height * share / 100
		// A card keeps no more rows than its list holds, so a short list leaves no
		// empty band under it. The count is of the whole list and not of what a term
		// kept, so the card does not grow and shrink while the user types.
		if contentRows > 0 {
			asked = min(asked, contentRows+present.CardChrome+hintRows)
		}
	case contentRows > 0:
		asked = contentRows + present.CardChrome + hintRows
	}
	if asked > ceiling {
		asked = ceiling
	}
	if asked < narrowestOverlayRow {
		asked = narrowestOverlayRow
	}
	if asked > model.height {
		asked = model.height
	}
	return present.AlignCardHeight(asked, model.height)
}

// countHintRows returns the rows the keys of a card take, with the blank row over them.
func countHintRows(keys *KeyLine, width int) int {
	if !keys.hasButtons() {
		return 0
	}
	return 1 + keys.countKeyRows(max(width-present.CardChrome, 1))
}

// countHintLines returns the rows the keys of a card take. A card with no key takes none.
func countHintLines(keys *KeyLine, width int) int {
	return keys.countKeyRows(width)
}

// renderOverlay draws the overlay on top, whichever one is open.
func (model *Model) renderOverlay(
	connection *app.Connection, tab *app.Tab, height int,
) string {
	overlay := connection.Overlay
	width := model.resolveOverlayWidth(overlay.Kind)
	// The hits of the card drawn before are dropped, so a press never lands on a chip
	// another card left behind.
	model.layout.overlayRows = rowsHit{}
	model.layout.overlayChips = nil
	model.layout.formRows = rowsHit{}
	model.layout.formChoices = nil

	switch overlay.Kind {
	case app.OverlayHelp:
		return model.renderHelp(overlay, width)
	case app.OverlayPalette:
		return model.renderPalette(overlay, width)
	case app.OverlayHistory:
		return model.renderHistory(overlay, width)
	case app.OverlaySaved:
		return model.renderSaved(overlay, width)
	case app.OverlayNotebooks:
		return model.renderNotebooks(overlay, width)
	case app.OverlayChart:
		return model.renderChartForm(tab, overlay, width)
	case app.OverlayObjectMenu, app.OverlayCopyMenu, app.OverlayActionMenu:
		return model.renderMenu(overlay, width)
	case app.OverlayConfirm:
		return model.renderConfirm(overlay, width)
	case app.OverlayWritePlan:
		return model.renderWritePlan(overlay, width)
	case app.OverlayMessage:
		return model.renderMessage(overlay, width)
	case app.OverlayDiagram:
		return model.renderDiagram(overlay, width)
	case app.OverlayCell:
		return model.renderCellViewer(overlay, width)
	case app.OverlayParameters:
		return model.renderParameters(overlay, width)
	case app.OverlayCellEdit:
		return model.renderCellEditor(overlay, width)
	case app.OverlayRowDetail:
		return model.renderRowDetail(overlay, width)
	case app.OverlayChanges:
		return model.renderChanges(overlay, width)
	case app.OverlayValueFilter:
		return model.renderValueFilter(overlay, width)
	case app.OverlayColumns:
		return model.renderColumnChooser(overlay, width)
	case app.OverlayThemePicker:
		return model.renderThemePicker(overlay, width)
	case app.OverlayActivity:
		return model.renderActivity(connection, overlay, width)
	case app.OverlayExport:
		return model.renderExport(overlay, width)
	case app.OverlayImport:
		return model.renderImport(overlay, width)
	case app.OverlayDump:
		return model.renderDump(overlay, width)
	case app.OverlayAiChat:
		return model.renderAiChat(connection, overlay, width)
	case app.OverlayAiChats:
		return model.renderAiChats(connection, overlay, width)
	case app.OverlayPrompt:
		return model.renderPrompt(overlay, width)
	case app.OverlayChoice:
		return model.renderChoice(overlay, width)
	case app.OverlayBuilderTables:
		return model.renderBuilderTables(overlay, width)
	case app.OverlayBuilderJoin:
		return model.renderBuilderJoin(tab, overlay, width)
	case app.OverlayBuilderField:
		return model.renderBuilderField(tab, overlay, width)
	}
	return ""
}

// ListCard says what one card with a list in it draws.
type ListCard struct {
	Kind  app.OverlayKind
	Title string
	// The line at the top that searches the list, or nothing for a list without one.
	Filter string
	// The lines over the rows, which do not scroll and the cursor does not reach.
	Header []string
	Rows   []string
	Cursor int
	Offset int
	Width  int
	// The keys the card names at the bottom, wrapped rather than cut, so a hint whose
	// last key is missing does not read as though the key is not there.
	Keys *KeyLine
	// True while the wheel moved the rows away from the cursor.
	Rolled bool
	// True for a list that says so where the term kept nothing.
	ReportsNoMatch bool
	// The line a card with no rows draws instead of its list, already styled.
	EmptyReport string
	// A note held against the right corner of the top border, already styled.
	Note string
	// How a drag of the bar moves the card. A card that keeps how far it has scrolled in
	// its cursor rather than its offset gives its own, so a drag moves what the keys move.
	Scrolls func(*app.Overlay, int)
	// How many rows the card shows where its height follows what it holds.
	ContentRows int
}

// renderListCard draws a card whose body is a list, with the row under the cursor filled.
// The card keeps a blank row inside each border, and a blank row over the keys it names.
func (model *Model) renderListCard(card ListCard) string {
	content := max(card.Width-4, 1)

	hintRows := card.Keys.countKeyRows(content)
	height := model.resolveOverlayHeight(
		card.Kind, card.ContentRows, countHintRows(card.Keys, card.Width))

	body := countCardBodyRows(height, hintRows)
	// The line the list is filtered with stands over the rows, so it takes one of them.
	if card.Filter != "" {
		body = max(body-1, 1)
	}
	// So do the lines of the card's own header.
	body = max(body-len(card.Header), 1)

	lines := []string{""}
	if card.Filter != "" {
		lines = append(lines, card.Filter)
	}
	lines = append(lines, card.Header...)

	// Where the rows land in the card, so a press chooses the row it looks like. The top
	// border takes the first row, and the lines written above the list follow it.
	model.layout.overlayRows = rowsHit{
		top:    cardListRow + len(lines),
		offset: scrollFrom(card.Cursor, card.Offset, body, len(card.Rows), card.Rolled),
		from:   1, to: card.Width - 2,
	}

	written := 0
	if len(card.Rows) == 0 {
		if card.EmptyReport != "" {
			lines = append(lines, " "+model.styles.Muted().Render(card.EmptyReport))
			written++
		} else if card.ReportsNoMatch {
			lines = append(lines, " "+model.styles.Muted().Render("no match"))
			written++
		}
	}
	shown := scrollFrom(card.Cursor, card.Offset, body, len(card.Rows), card.Rolled)
	// The bar stands over the last cell of the content, and a list that fits gets none.
	thumb := buildScrollThumb(shown, body, len(card.Rows))
	// The list of every card of this kind is the one the open overlay holds, which is what
	// the keys and the wheel move as well. The border of the block takes its first row and
	// its first column, and the rows of the list follow the lines written above them.
	model.recordCardScrollbar(thumb, 1+card.Width-4, 1+len(lines), min(body, len(thumb)),
		shown, len(card.Rows), card.Scrolls)
	for at := shown; at < len(card.Rows) && written < body; at++ {
		row := card.Rows[at]
		if written < len(thumb) {
			row = model.styles.paintThumbColumn(
				row, thumb[written], card.Width-4, model.styles.Theme.Panel)
		}
		lines = append(lines, row)
		written++
	}
	for written < body {
		row := ""
		if written < len(thumb) {
			row = model.styles.paintThumbColumn(
				row, thumb[written], card.Width-4, model.styles.Theme.Panel)
		}
		lines = append(lines, row)
		written++
	}
	model.layout.overlayRows.count = written
	if hintRows > 0 {
		lines = append(lines, "")
		// The keys land under the list, one blank column inside the border, and the top
		// border takes the first row of the card.
		for _, line := range model.renderKeyButtons(card.Keys, content, hintRows,
			cardListRow+len(lines), cardBodyColumn) {
			lines = append(lines, paintOn(model.styles.Theme.Panel, " ")+line)
		}
	}
	lines = append(lines, "")

	return model.styles.RenderBox(BoxOptions{
		Width: card.Width, Height: height, Title: card.Title, Note: card.Note,
		Focused: true, Lines: lines,
	})
}

// The two ways a card is drawn: one that removes something carries the mark of a warning.
const (
	destructiveCard = true
	plainCard       = false
)

// renderTextCard draws a card that is as tall as what it holds, with the keys it names at
// the bottom. The card keeps a blank row inside each border, and a blank row over the keys.
func (model *Model) renderTextCard(
	kind app.OverlayKind, title string, width int, lines []string,
	keys *KeyLine, contentRows int, destructive bool,
) string {
	return model.renderNotedTextCard(kind, title, "", "", width, lines, keys, contentRows,
		destructive)
}

// renderNotedTextCard draws the same card with a note on its top and bottom borders.
func (model *Model) renderNotedTextCard(
	kind app.OverlayKind, title, note, bottomNote string, width int, lines []string,
	keys *KeyLine, contentRows int, destructive bool,
) string {
	content := max(width-present.CardChrome, 1)
	hintRows := keys.countKeyRows(content)
	height := model.resolveOverlayHeight(kind, contentRows, countHintRows(keys, width))
	body := countCardBodyRows(height, hintRows)

	// A body taller than the card scrolls, with a bar in its last column.
	if len(lines) > body {
		offset := 0
		if held := model.Active(); held != nil && held.Overlay.Kind == kind {
			offset = held.Overlay.List.Offset
		}
		shown := lines
		lines = model.scrollCardRows(len(shown), offset, body, content,
			model.styles.Theme.Panel, func(at int) string {
				return padStyledOn(shown[at], content, model.styles.Theme.Panel)
			})
	}

	written := make([]string, 0, height)
	written = append(written, "")
	for at := range body {
		if at < len(lines) {
			written = append(written, " "+lines[at])
			continue
		}
		written = append(written, "")
	}
	if hintRows > 0 {
		written = append(written, "")
		// The keys land under the body, one blank column inside the border, and the top
		// border takes the first row of the card.
		for _, line := range model.renderKeyButtons(keys, content, hintRows,
			cardListRow+len(written), cardBodyColumn) {
			written = append(written, paintOn(model.styles.Theme.Panel, " ")+line)
		}
	}
	written = append(written, "")

	return model.styles.RenderBox(BoxOptions{
		Width: width, Height: height, Title: title, Note: note, BottomNote: bottomNote,
		Focused: true, Destructive: destructive, Lines: written,
	})
}

// Where the body of a card that renderTextCard draws stands in the block of that card: its
// border takes the first row and the first column, it opens with a blank row, and it writes
// each line of its body after one blank column. A card with a list writes its own lines from
// the row under the border, so the list follows them.
const (
	cardBodyRow    = 2
	cardBodyColumn = 2
	cardListRow    = 1
)

// countCardBodyRows returns the rows a card of this height gives its content: its borders,
// the blank row inside each one, and the keys with the blank row over them all come off.
func countCardBodyRows(height, hintLines int) int {
	body := height - present.CardChrome - hintLines
	if hintLines > 0 {
		body--
	}
	if body < 1 {
		return 1
	}
	return body
}

// recordCardBody keeps where the first line of the body of a card was drawn, counted from
// the card itself. The placement carries the whole card onto the screen afterwards, so no
// renderer of a card works out where it landed.
func (model *Model) recordCardBody() {
	model.layout.cardBodyTop = cardBodyRow
	model.layout.cardBodyLeft = cardBodyColumn
}

// The parts of a row every list of a card draws: the gutter the pointer is drawn in, the
// column the scroll bar takes with a blank column before it, and the width of the detail
// where the row has a trail.
const (
	rowPaddingLeft    = 2
	rowScrollbarWidth = 2
	detailBesideTrail = 8
)

// ListRowSpec says what one row of a list holds and how wide each column is.
type ListRowSpec struct {
	// Lead stands before the label in muted text, such as the time a statement ran.
	Lead       string
	LeadWidth  int
	Label      string
	LabelWidth int
	// The glyph of what the row acts on, drawn before its name. A list where no row names
	// one keeps the column for none of them.
	Icon    cfg.IconKind
	HasIcon bool
	Detail  string
	// Trail stands after the detail, at the right of the row. HasTrail is what tells a
	// row with an empty trail from a row without one, which measure the detail
	// differently.
	Trail    string
	HasTrail bool
	// Key is drawn at the right end of the row. Trail and Key are not used together.
	Key         string
	Selected    bool
	Destructive bool
	// Match is the filter term, drawn in bold where the label or the detail contains it.
	Match string
	// Width is the width of the card the row is drawn in.
	Width int
}

// listRowIconWidth is the glyph a row draws before its name, and the blank after it.
const listRowIconWidth = 2

// renderListRow draws one row of a list. The lead, the name and what it does each keep
// their own column, so the names of a list read down the card rather than drifting with
// the length of a detail.
func (model *Model) renderListRow(row ListRowSpec) string {
	theme := model.styles.Theme
	ground := theme.Panel
	quiet, labelInk := theme.Muted, theme.Text
	if row.Destructive {
		labelInk = theme.Error
	}
	if row.Selected {
		ground = theme.Accent
		quiet, labelInk = theme.OnAccent, theme.OnAccent
	}
	matchInk := theme.Accent
	if row.Selected {
		matchInk = theme.OnAccent
	}
	paint := func(ink color.Color, text string) string {
		return paintText(ink, ground, text)
	}
	paintMatch := func(ink color.Color, text string) string {
		from, to, found := present.FindTextSpan(text, row.Match)
		if !found {
			return paint(ink, text)
		}
		return paint(ink, text[:from]) + paintBoldText(matchInk, ground, text[from:to]) +
			paint(ink, text[to:])
	}

	written := paint(labelInk, model.buildRowGutter(row.Selected))
	if row.LeadWidth > 0 {
		written += paint(quiet, present.FitText(row.Lead, row.LeadWidth))
	}
	if row.HasIcon {
		iconInk := model.styles.IconColor(row.Icon)
		if row.Selected {
			iconInk = theme.OnAccent
		}
		written += paint(iconInk, present.FitText(
			model.icons.Icon(row.Icon), listRowIconWidth))
	}
	written += paintMatch(labelInk, present.FitText(
		present.TruncateText(row.Label, row.LabelWidth-1), row.LabelWidth))

	room := max(row.Width-present.CardChrome-rowPaddingLeft-
		row.LeadWidth-row.LabelWidth-rowScrollbarWidth, 0)
	if row.HasIcon {
		room = max(room-listRowIconWidth, 0)
	}
	switch {
	case row.HasTrail:
		written += paint(quiet, present.FitText(
			present.TruncateText(row.Detail, detailBesideTrail), detailBesideTrail))
		written += paint(quiet, row.Trail)
	case row.Key != "":
		detailRoom := max(room-present.MeasureText(row.Key)-1, 0)
		written += paintMatch(quiet, present.FitText(
			present.TruncateText(row.Detail, detailRoom), detailRoom))
		written += paint(quiet, " "+row.Key)
	default:
		written += paintMatch(quiet, present.TruncateText(row.Detail, room))
	}

	// The card keeps a blank column of its own ground on each side, and the scroll bar
	// stands in the column before the last one, so the row ends there.
	pad := paintOn(theme.Panel, " ")
	return pad + padStyledOn(written, row.Width-4, ground) + pad + pad
}

// buildRowGutter returns the gutter before a row of a list, with the pointer on the
// selected row.
func (model *Model) buildRowGutter(selected bool) string {
	if !selected {
		return strings.Repeat(" ", rowPaddingLeft)
	}
	return present.FitText(model.icons.Icon(cfg.IconPrompt), rowPaddingLeft)
}

// The indent before a key of the help, and the gap after the widest key.
const (
	helpKeyIndent = 2
	helpColumnGap = 2
)

// The name column of the lists that carry no key of their own.
const (
	themeLabelWidth = 26
	valueMarkWidth  = 2
	valueLabelWidth = 34
)

// selectedThemeMark stands on the theme that was applied when the picker opened.
const selectedThemeMark = "✓"

// The name column of one row of a menu.
const (
	menuLabelWidth   = 26
	copyLabelWidth   = 22
	objectLabelWidth = 24
)

// The columns of one row of the palette, the history and the saved statements.
const (
	paletteLabelWidth = 38
	// paletteGroupWidth is the column the group of a palette row keeps.
	paletteGroupWidth = 13
	historyTimeWidth  = 12
	savedNameWidth    = 26
)

// readOverlayTerm returns the term typed into the line at the top of a list.
func (model *Model) readOverlayTerm(overlay app.Overlay) string {
	if overlay.Draft == nil {
		return ""
	}
	return strings.TrimSpace(overlay.Draft.Text)
}

// renderSearchField draws the same line with the mark of a search rather than of a filter,
// which is what the help draws.
func (model *Model) renderSearchField(
	overlay app.Overlay, width int, placeholder string, count int,
) string {
	return model.renderFilterLine(overlay.Draft, width, placeholder, count, " / ", fieldHasCaret)
}

// renderFilterFieldOf draws the same field with the word it asks for, and the count of
// the rows it kept where the list shows one.
func (model *Model) renderFilterFieldOf(
	overlay app.Overlay, width int, placeholder string, count int,
) string {
	return model.renderFilterLine(overlay.Draft, width, placeholder, count, " / ", fieldHasCaret)
}

// fieldHasCaret draws the field as focused, with the caret.
const fieldHasCaret = true

func (model *Model) renderFilterLine(
	draft *app.EditorBuffer, width int, placeholder string, count int, mark string,
	focused bool,
) string {
	theme := model.styles.Theme
	if draft == nil {
		return ""
	}
	written := draft.Text
	body := paintText(theme.Text, theme.Header, written)
	if written == "" {
		body = model.styles.Muted().Background(theme.Header).Render(placeholder)
	}
	caret := paintOn(theme.Accent, " ")
	if !focused {
		caret = paintOn(theme.Header, " ")
	}
	counted := ""
	if count >= 0 {
		counted = model.styles.Muted().Background(theme.Header).
			Render(" " + present.FormatCount(int64(count)) + " ")
	}

	field := paintText(theme.Accent, theme.Header, mark) + body + caret
	// The line runs the width of the content, so the ground of the card stands beside it
	// where the padding of the card is.
	gap := max(width-4-measureStyledWidth(field)-measureStyledWidth(counted), 0)
	pad := paintOn(theme.Panel, " ")
	return pad + field + paintOn(theme.Header, strings.Repeat(" ", gap)) + counted + pad
}

// renderPalette draws every action the palette can run, with the key of each.
func (model *Model) renderPalette(overlay app.Overlay, width int) string {
	actions := model.filterPalette(overlay)
	term := model.readOverlayTerm(overlay)
	rows := make([]string, 0, len(actions))
	for at, action := range actions {
		lead := ""
		if at == 0 || actions[at-1].Group != action.Group {
			lead = action.Group
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: lead, LeadWidth: paletteGroupWidth,
			Label: action.Label, LabelWidth: paletteLabelWidth,
			Detail: action.Detail, Key: action.Chord, Match: term,
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayPalette, Title: " command palette ",
		Filter: model.renderFilterFieldOf(overlay, width, "filter commands", len(actions)), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, ContentRows: len(overlay.Palette) + 1,
		Keys: model.buildCardKeys(app.OverlayPalette, keyScene{overlay: overlay}),
	})
}

// renderHistory draws every statement that ran on this profile, newest first.
func (model *Model) renderHistory(overlay app.Overlay, width int) string {
	entries := model.filterHistory(overlay)
	rows := make([]string, 0, len(entries))
	now := time.Now()

	elapsedWidth := present.MeasureText(historyElapsedHeading)
	for _, entry := range overlay.Entries {
		elapsedWidth = max(elapsedWidth, present.MeasureText(present.FormatDuration(entry.Elapsed)))
	}
	// The statement takes the width the other columns leave.
	sqlWidth := max(width-present.CardChrome-rowPaddingLeft-historyTimeWidth-
		detailBesideTrail-elapsedWidth-rowScrollbarWidth, 12)

	for at, entry := range entries {
		outcome := ""
		switch {
		case entry.ErrorMessage != "":
			outcome = "failed"
		case entry.HasRowCount:
			outcome = present.FormatCount(entry.RowCount)
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: present.FormatWhen(entry.RanAt, now), LeadWidth: historyTimeWidth,
			Label: core.CollapseWhitespace(entry.SQL), LabelWidth: sqlWidth,
			Detail: present.FitTextRight(outcome, detailBesideTrail-2) + "  ",
			Trail:  present.FitTextRight(present.FormatDuration(entry.Elapsed), elapsedWidth), HasTrail: true,
			Selected: at == overlay.List.Cursor, Destructive: entry.ErrorMessage != "",
			Width: width,
		}))
	}

	theme := model.styles.Theme
	heading := strings.Repeat(" ", rowPaddingLeft) +
		present.FitText("ran at", historyTimeWidth) + present.FitText("statement", sqlWidth) +
		present.FitTextRight("rows", detailBesideTrail-2) + "  " +
		present.FitTextRight(historyElapsedHeading, elapsedWidth)
	pad := paintOn(theme.Panel, " ")
	header := pad + padStyledOn(paintText(theme.Muted, theme.Panel, heading), width-4, theme.Panel) +
		pad + pad

	title := " query history "
	if connection := model.Active(); connection != nil {
		title = " query history · " + connection.Profile().Name + " "
	}
	keys := model.buildCardKeys(app.OverlayHistory, keyScene{overlay: overlay})
	return model.renderListCard(ListCard{
		Kind: app.OverlayHistory, Title: title,
		Filter: model.renderFilterFieldOf(
			overlay, width, "filter statements", len(entries)),
		Header: []string{header},
		Rows:   rows, Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: keys, ContentRows: len(overlay.Entries) + 2,
	})
}

// historyElapsedHeading is the heading of the column of the time a statement took.
const historyElapsedHeading = "time"

// savedSQLWidth is how much of a saved statement the row shows, and the filter reads.
const savedSQLWidth = 70

// The lead column of the saved statements, which marks the ones a project file holds.
const (
	savedProjectLead = "project"
	savedLeadWidth   = 8
)

// renderSaved draws the statements a reader kept by name, and the ones the project file
// holds for the whole team.
func (model *Model) renderSaved(overlay app.Overlay, width int) string {
	queries := model.filterSaved(overlay)
	// The lead column stands empty where no statement comes from a project file, so a
	// user without one loses no room to it.
	leadWidth := 0
	if slices.ContainsFunc(overlay.Saved, func(saved app.SavedRow) bool {
		return saved.IsFromProject()
	}) {
		leadWidth = savedLeadWidth
	}

	rows := make([]string, 0, len(queries))
	for at, saved := range queries {
		lead := ""
		if saved.IsFromProject() {
			lead = savedProjectLead
		}
		detail := core.CollapseWhitespace(saved.SQL)
		if saved.Description != "" {
			detail = saved.Description
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: lead, LeadWidth: leadWidth,
			Label: saved.Name, LabelWidth: savedNameWidth,
			Detail:   present.TruncateText(detail, savedSQLWidth),
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	keys := model.buildCardKeys(app.OverlaySaved, keyScene{overlay: overlay})
	return model.renderListCard(ListCard{
		Kind:   app.OverlaySaved,
		Title:  " saved queries · " + present.FormatCount(int64(len(overlay.Saved))) + " ",
		Filter: model.renderFilterFieldOf(overlay, width, "filter saved queries", len(queries)), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: keys, ContentRows: len(overlay.Saved) + 1,
	})
}

// renderMenu draws a menu of actions, one row each.
func (model *Model) renderMenu(overlay app.Overlay, width int) string {
	actions := model.filterMenu(overlay)
	labelWidth := menuLabelWidth
	switch overlay.Kind {
	case app.OverlayCopyMenu:
		labelWidth = copyLabelWidth
	case app.OverlayObjectMenu:
		labelWidth = objectLabelWidth
	}

	// A menu whose rows all act on the same thing drops the column that would hold a glyph,
	// and so does one whose set draws no glyph at all.
	icons := false
	for _, action := range overlay.Actions {
		if model.icons.Icon(action.Icon) != "" {
			icons = true
			break
		}
	}
	if icons {
		labelWidth -= listRowIconWidth
	}

	term := model.readOverlayTerm(overlay)
	rows := make([]string, 0, len(actions))
	for at, action := range actions {
		if action.Divider {
			rows = append(rows, model.renderMenuDivider(width))
			continue
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Icon: action.Icon, HasIcon: icons,
			Label: action.Label, LabelWidth: labelWidth, Detail: action.Detail,
			Key: action.Chord, Match: term,
			Selected: at == overlay.List.Cursor, Destructive: action.Destructive, Width: width,
		}))
	}
	title := overlay.Title
	if title == "" {
		title = " menu "
	} else if !strings.HasPrefix(title, " ") {
		title = " " + title + " "
	}
	placeholder := "filter actions"
	switch {
	case overlay.Placeholder != "":
		placeholder = overlay.Placeholder
	case overlay.Kind == app.OverlayCopyMenu:
		placeholder = "filter formats"
	}
	text := model.buildCardKeys(overlay.Kind, keyScene{overlay: overlay})
	return model.renderListCard(ListCard{
		Kind: overlay.Kind, Title: title,
		Filter: model.renderFilterFieldOf(overlay, width, placeholder, countMenuActions(actions)),
		Rows:   rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true, Keys: text,
		// The filter line stands over the rows and takes one of them.
		ContentRows: len(insertMenuDivider(overlay.Actions)) + 1,
	})
}

// countMenuActions counts the rows of a menu that run an action, without its divider.
func countMenuActions(actions []app.MenuAction) int {
	count := 0
	for _, action := range actions {
		if !action.Divider {
			count++
		}
	}
	return count
}

// renderMenuDivider draws the line above the destructive rows of a menu.
func (model *Model) renderMenuDivider(width int) string {
	theme := model.styles.Theme
	rule := strings.Repeat(" ", rowPaddingLeft) +
		strings.Repeat("─", max(width-4-rowPaddingLeft-rowScrollbarWidth, 0))
	pad := paintOn(theme.Panel, " ")
	return pad + padStyledOn(paintText(theme.Faint, theme.Panel, rule), width-4, theme.Panel) +
		pad + pad
}

// The columns of one answer of a question with more than two of them.
const (
	choiceKeyWidth   = 4
	choiceLabelWidth = 22
)

// renderConfirm draws a question with two answers. Each answer is a button.
func (model *Model) renderConfirm(overlay app.Overlay, width int) string {
	lines := model.wrapText(overlay.Body, width-present.CardChrome)
	if overlay.Destructive {
		lines = model.wrapErrorText(overlay.Body, width-present.CardChrome)
	}
	lines = append(lines, "")

	yes := model.buildCardButton(cfg.ScopeDialog, ActionAnswerYes, describeConfirmYes(keyScene{overlay: overlay}))
	yes.primary, yes.destructive = true, overlay.Destructive
	no := model.buildCardButton(cfg.ScopeDialog, ActionAnswerNo, describeConfirmNo(keyScene{overlay: overlay}))
	start := 0
	if overlay.Destructive {
		start = 1
	}
	model.recordCardBody()
	lines = append(lines, model.renderButtonRow([]cardButton{yes, no}, start,
		cardBodyRow+len(lines), cardBodyColumn))

	card := model.renderNotedTextCard(overlay.Kind, overlay.Title,
		model.renderActiveEnvironmentBadge(), "", width, lines, nil, len(lines), overlay.Destructive)
	return card
}

// describeConfirmYes returns the label of the answer that runs.
func describeConfirmYes(scene keyScene) string {
	if scene.overlay.Yes != "" {
		return scene.overlay.Yes
	}
	return "run"
}

// describeConfirmNo returns the label of the answer that cancels.
func describeConfirmNo(scene keyScene) string {
	if scene.overlay.No != "" {
		return scene.overlay.No
	}
	return "cancel"
}

// renderChoice draws a question with more than two answers, each on its own letter.
func (model *Model) renderChoice(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	lines := model.wrapText(overlay.Body, width-present.CardChrome)
	lines = append(lines, "")
	firstChoice := len(lines)
	for _, choice := range overlay.Choices {
		ink := theme.Text
		if choice.Destructive {
			ink = theme.Error
		}
		lines = append(lines,
			paintText(theme.Accent, theme.Panel, present.FitText(" "+choice.Key, choiceKeyWidth))+
				paintText(ink, theme.Panel, present.FitText(choice.Label, choiceLabelWidth))+
				model.styles.Muted().Render(choice.Detail))
	}
	keys := model.buildCardKeys(app.OverlayChoice, keyScene{overlay: overlay})
	model.recordCardBody()
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop + firstChoice, count: len(overlay.Choices),
		from: model.layout.cardBodyLeft - 1,
		to:   model.layout.cardBodyLeft + width - present.CardChrome,
	}
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines, keys, len(lines), plainCard)
}

// renderMessage draws a block of text that has to be read.
func (model *Model) renderMessage(overlay app.Overlay, width int) string {
	lines := []string{}
	source := overlay.Lines
	if len(source) == 0 {
		source = strings.Split(overlay.Body, "\n")
	}
	for _, line := range source {
		lines = append(lines, model.wrapText(line, width-present.CardChrome)...)
	}
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines,
		model.buildScrollingCardKeys(overlay, lines, len(lines), width), len(lines), plainCard)
}

// buildScrollingCardKeys returns the keys of a text card, with the keys that scroll it where
// its body is taller than the card.
func (model *Model) buildScrollingCardKeys(
	overlay app.Overlay, lines []string, contentRows, width int,
) *KeyLine {
	scene := keyScene{overlay: overlay}
	keys := model.buildCardKeys(overlay.Kind, scene)
	height := model.resolveOverlayHeight(overlay.Kind, contentRows, countHintRows(keys, width))
	if len(lines) > countCardBodyRows(height, countHintLines(keys, width-present.CardChrome)) {
		scene.scrolls = true
		keys = model.buildCardKeys(overlay.Kind, scene)
	}
	return keys
}

// renderDiagram draws the lines of an ER diagram. A line keeps its own shape and is never
// wrapped, because a box drawn over two rows would come apart.
func (model *Model) renderDiagram(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	drawn := overlay.Diagram
	keys := model.buildCardKeys(app.OverlayDiagram, keyScene{overlay: overlay})
	width = fitCardWidth(width, drawn.Width, model.measureKeyButtons(keys), overlay.Title, "")
	room := max(width-present.CardChrome, 1)
	height := model.resolveOverlayHeight(
		overlay.Kind, len(drawn.Lines), countHintRows(keys, width))
	body := countCardBodyRows(height, countHintLines(keys, room))
	model.recordCardBody()

	count := len(drawn.Lines)
	top := clampOffset(overlay.List.Cursor, body, count)
	thumb := buildScrollThumb(top, body, count)
	model.recordCardScrollbar(thumb, cardBodyColumn+room-1, cardBodyRow,
		min(body, len(thumb)), top, count, scrollDiagramToRow)
	shown := room
	if len(thumb) > 0 {
		shown--
	}
	model.layout.cardBody, model.layout.cardRoom = body, shown

	lines := make([]string, 0, body)
	for at := top; at < count && len(lines) < body; at++ {
		line := padStyledOn(model.paintDiagramRow(overlay, at, shown), room, theme.Panel)
		if len(lines) < len(thumb) {
			line = model.styles.paintThumbColumn(line, thumb[len(lines)], room-1, theme.Panel)
		}
		lines = append(lines, line)
	}
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines, keys, count,
		plainCard)
}

// scrollDiagramToRow moves a diagram to the row a drag of its bar reached.
func scrollDiagramToRow(overlay *app.Overlay, row int) {
	overlay.List.Cursor = max(row, 0)
}

// paintDiagramRow draws one row of a diagram from the column the card is panned to. The
// borders and the lines are muted, each key mark has the colour of its icon in the tree, and
// each type is muted. The focused box, and the lines of a focused neighbour, are in the
// accent.
func (model *Model) paintDiagramRow(overlay app.Overlay, row, room int) string {
	theme := model.styles.Theme
	drawn := overlay.Diagram
	runes := []rune(drawn.Lines[row])
	from := min(max(overlay.List.Offset, 0), len(runes))
	to := min(from+room, len(runes))

	inks := make([]color.Color, len(runes))
	bold := make([]bool, len(runes))
	for at := range inks {
		inks[at] = theme.Text
	}
	paint := func(x, width int, ink color.Color) {
		for at := max(x, 0); at < min(x+width, len(inks)); at++ {
			inks[at] = ink
		}
	}
	paintBorder := func(box present.DiagramBox, ink color.Color) {
		switch {
		case row == box.Y || row == box.Y+2 || row == box.Y+box.Height-1:
			paint(box.X, box.Width, ink)
		case row > box.Y && row < box.Y+box.Height-1:
			paint(box.X, 1, ink)
			paint(box.X+box.Width-1, 1, ink)
		}
	}
	for _, box := range drawn.Boxes {
		paintBorder(box, theme.Faint)
	}
	focused := overlay.Field >= 0 && overlay.Field < len(drawn.Boxes)
	for _, lit := range []bool{false, true} {
		for _, link := range drawn.Links {
			touches := focused && overlay.Field != drawn.Root &&
				(link.From == overlay.Field || link.To == overlay.Field)
			if touches != lit {
				continue
			}
			ink := theme.Muted
			if lit {
				ink = theme.Accent
			}
			for _, cell := range link.Cells {
				if cell.Y == row {
					paint(cell.X, 1, ink)
				}
			}
		}
	}
	for _, span := range drawn.Spans {
		if span.Y != row {
			continue
		}
		switch span.Kind {
		case present.DiagramSpanTitle:
			for at := max(span.X, 0); at < min(span.X+span.Width, len(bold)); at++ {
				bold[at] = true
			}
		case present.DiagramSpanPrimary:
			paint(span.X, span.Width, model.styles.IconColor(cfg.IconPrimaryKey))
		case present.DiagramSpanForeign:
			paint(span.X, span.Width, model.styles.IconColor(cfg.IconForeignKey))
		case present.DiagramSpanType:
			paint(span.X, span.Width, theme.Muted)
		}
	}
	if focused {
		box := drawn.Boxes[overlay.Field]
		paintBorder(box, theme.Accent)
		if row == box.Y+1 {
			paint(box.X+2, box.Width-3, theme.Accent)
		}
	}

	written := strings.Builder{}
	for start := from; start < to; {
		end := start + 1
		for end < to && inks[end] == inks[start] && bold[end] == bold[start] {
			end++
		}
		opening := resolveOpening(inks[start], theme.Panel)
		if bold[start] {
			opening = resolveBoldOpening(inks[start], theme.Panel)
		}
		writeOpenedText(&written, opening, string(runes[start:end]))
		start = end
	}
	return written.String()
}

// renderCellViewer draws one cell full size. A value taller than the card scrolls inside it,
// and the bar down its last column says how much of it is on screen.
func (model *Model) renderCellViewer(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	room := max(width-present.CardChrome, 1)
	written := present.FormatColumnForViewer(overlay.Cell.Value, overlay.Cell.Column,
		model.settings.TimeZone.ResolveLocation())
	held := []string{}
	for line := range strings.SplitSeq(present.SafeLines(written), "\n") {
		held = append(held, model.wrapText(line, room)...)
	}

	keys := model.buildCardKeys(app.OverlayCell, keyScene{overlay: overlay})

	height := model.resolveOverlayHeight(overlay.Kind, len(held), countHintRows(keys, width))
	body := countCardBodyRows(height, countHintLines(keys, room))
	ink := theme.Text
	if written == core.NullText {
		ink = theme.Muted
	}
	lines := model.scrollCardRows(len(held), overlay.List.Offset, body, room, theme.Panel,
		func(at int) string {
			return paintText(ink, theme.Panel, present.TruncateText(held[at], room))
		})
	return model.renderTextCard(overlay.Kind, " "+overlay.Cell.Column.Name+" ", width, lines,
		keys, len(held), plainCard)
}

// renderParameters draws the field the values of the `:name` marks are written in, as JSON,
// so a null stays a null and a number stays a number.
func (model *Model) renderParameters(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	keys := model.buildCardKeys(app.OverlayParameters, keyScene{overlay: overlay})
	height := model.resolveOverlayHeight(
		overlay.Kind, overlay.ContentRows, countHintRows(keys, width))
	lines := model.renderDraftRows(overlay.Draft, width-present.CardChrome,
		countCardBodyRows(height, countHintLines(keys, width-present.CardChrome)),
		FieldLook{Ground: theme.Panel, Ink: theme.Text, Focused: true})

	title := " " + present.FormatCountOf(
		int64(len(overlay.Names)), "parameter", "parameters") + " "
	return model.renderTextCard(overlay.Kind, title, width, lines,
		keys, overlay.ContentRows, plainCard)
}

// renderCellEditor draws the list a cell is picked from, or the field one cell or a whole new
// row is written in.
func (model *Model) renderCellEditor(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	keys := model.buildCardKeys(app.OverlayCellEdit, keyScene{overlay: overlay})
	title := model.buildCellEditorTitle(overlay)
	if overlay.ContentWidth > 0 {
		width = fitCardWidth(width, overlay.ContentWidth, model.measureKeyButtons(keys), title, "")
	}

	lines := model.renderCellChoices(overlay)
	height := model.resolveOverlayHeight(
		overlay.Kind, overlay.ContentRows, countHintRows(keys, width))
	body := countCardBodyRows(height, countHintLines(keys, width-present.CardChrome))
	if len(overlay.Cell.Choices) == 0 {
		lines = model.renderDraftRows(overlay.Draft, width-present.CardChrome, body,
			FieldLook{Ground: theme.Panel, Ink: theme.Text, Focused: true})
	} else if held := model.Active(); held != nil && held.Overlay.Kind == overlay.Kind {
		// The list follows the cursor, and the card scrolls it.
		held.Overlay.List.Offset = scrollTo(held.Overlay.List.Cursor,
			held.Overlay.List.Offset, body, len(overlay.Cell.Choices))
	}
	return model.renderTextCard(overlay.Kind, title, width,
		lines, keys, overlay.ContentRows, plainCard)
}

// buildCellEditorTitle names the column and the type the value is written for. A type that
// holds JSON is named as such, because the card prettifies one.
func (model *Model) buildCellEditorTitle(overlay app.Overlay) string {
	title := overlay.Cell.Column.Name + " · " + overlay.Cell.Column.DescribeType()
	if present.IsJSONType(overlay.Cell.Column.DataType) {
		title += " · json"
	}
	return " " + title + " "
}

// renderCellChoices draws the values the column takes, one to a row, with the one the cursor
// stands on marked.
func (model *Model) renderCellChoices(overlay app.Overlay) []string {
	theme := model.styles.Theme
	lines := make([]string, 0, len(overlay.Cell.Choices))
	for at, value := range overlay.Cell.Choices {
		ink, ground := theme.Text, theme.Background
		if at == overlay.List.Cursor {
			ink, ground = theme.OnAccent, theme.BorderFocus
		}
		lines = append(lines, paintText(ink, ground, " "+value))
	}
	return lines
}

// renderRowDetail draws every column of one row, so a wide row is read without scrolling.
func (model *Model) renderRowDetail(overlay app.Overlay, width int) string {
	if overlay.Window.Index < 0 || overlay.Window.Index >= len(overlay.Window.Rows) {
		return ""
	}
	theme := model.styles.Theme
	row := overlay.Window.Rows[overlay.Window.Index]
	inner := max(width-present.CardChrome, 1)
	plan := present.PlanFieldColumns(inner)
	for _, column := range overlay.Window.Columns {
		wanted := present.MeasureText(column.DescribeType()) + rowDetailTypeGap
		if wanted > plan.Type && inner-plan.Name-wanted >= rowDetailValueWidth {
			plan.Type = wanted
		}
	}
	room := max(inner-plan.Name-plan.Type, 1)
	target := app.EditTarget{}
	if connection := model.Active(); connection != nil {
		target = connection.Active().Target
	}
	cursor := clamp(overlay.List.Cursor, len(overlay.Window.Columns))

	// A value too long for its column wraps under itself, so the whole of it is read
	// without the two names giving way.
	held := []string{}
	fields := make([]lineSpan, 0, len(overlay.Window.Columns))
	for at, column := range overlay.Window.Columns {
		var value any
		if at < len(row) {
			value = row[at]
		}
		written := present.SafeLines(present.FormatColumnForViewer(value, column,
			model.settings.TimeZone.ResolveLocation()))
		ink, nameInk, typeInk := theme.Text, theme.Accent, theme.Muted
		if value == nil {
			ink = theme.Muted
		}
		ground := theme.Panel
		if at%2 == 1 {
			ground = theme.Zebra
		}
		if at == cursor {
			ground = theme.Accent
			ink, nameInk, typeInk = theme.OnAccent, theme.OnAccent, theme.OnAccent
		}
		from := len(held)
		mark := "  "
		if icon, marked := resolveColumnKeyIcon(target, column.Name); marked {
			mark = present.FitText(model.icons.Icon(icon), 2)
		}
		head := paintText(nameInk, ground, present.FitText(mark+column.Name, plan.Name)) +
			paintText(typeInk, ground, present.FitText(column.DescribeType(), plan.Type))
		for line, text := range model.wrapText(written, room) {
			if line > 0 {
				head = paintOn(ground, strings.Repeat(" ", plan.Name+plan.Type))
			}
			held = append(held, padStyledOn(
				head+paintText(ink, ground, present.TruncateText(text, room)), inner, ground))
		}
		fields = append(fields, lineSpan{from: from, to: len(held)})
	}
	model.layout.rowDetailFields = fields

	keys := model.buildCardKeys(app.OverlayRowDetail, keyScene{model: model, overlay: overlay})
	height := model.resolveOverlayHeight(
		overlay.Kind, len(overlay.Window.Columns), countHintRows(keys, width))
	body := countCardBodyRows(height, countHintLines(keys, inner))
	offset := overlay.List.Offset
	if !overlay.List.Rolled && cursor < len(fields) {
		offset = followLineSpan(fields[cursor], offset, body)
	}
	lines := model.scrollCardRows(len(held), offset, body, inner, theme.Panel,
		func(at int) string { return held[at] })

	title := " row " + present.FormatCount(int64(overlay.Window.Index+1)) + " of " +
		present.FormatCount(int64(len(overlay.Window.Rows)))
	if overlay.Window.Truncated {
		title += " loaded"
	}
	return model.renderTextCard(overlay.Kind, title+" ", width, lines,
		keys, len(overlay.Window.Columns), plainCard)
}

// lineSpan is the lines one field of the row card takes, from the first to the one after the
// last.
type lineSpan struct {
	from, to int
}

// followLineSpan returns the offset that keeps the span on screen, and moves as little as
// it can.
func followLineSpan(span lineSpan, offset, rows int) int {
	if span.from < offset {
		return span.from
	}
	if span.to > offset+rows {
		return max(span.to-rows, 0)
	}
	return offset
}

// The gap after the widest type of the row card, and the value width the type never takes.
const (
	rowDetailTypeGap    = 2
	rowDetailValueWidth = 20
)

// resolveColumnKeyIcon returns the icon of a key column of the table the rows were read
// from: the primary key first, then a foreign key.
func resolveColumnKeyIcon(target app.EditTarget, name string) (cfg.IconKind, bool) {
	for _, key := range target.KeyColumns {
		if strings.EqualFold(key, name) {
			return cfg.IconPrimaryKey, true
		}
	}
	if _, points := build.FindForeignKeyTarget(target.ForeignKeys, name); points {
		return cfg.IconForeignKey, true
	}
	return "", false
}

// scrollCardRows returns the rows a card that scrolls without a cursor draws: the offset
// clamped to what is there, the bar recorded and its thumb written over the last column of
// each row. `buildRow` draws one row of the content, so a card that is scrolled past draws
// only the rows it shows.
func (model *Model) scrollCardRows(
	count, offset, body, room int, ground color.Color, buildRow func(int) string,
) []string {
	from := clampOffset(offset, body, count)
	// The keys that scroll the card read these, because only the draw knows how many rows
	// the content takes at this width.
	model.layout.cardLines, model.layout.cardBody = count, body

	thumb := buildScrollThumb(from, body, count)
	model.recordCardScrollbar(thumb, cardBodyColumn+room-1, cardBodyRow,
		min(body, len(thumb)), from, count, nil)
	lines := make([]string, 0, body)
	for at := from; at < count && len(lines) < body; at++ {
		line := buildRow(at)
		if len(lines) < len(thumb) {
			line = model.styles.paintThumbColumn(line, thumb[len(lines)], room-1, ground)
		}
		lines = append(lines, line)
	}
	return lines
}

// recordCardScrollbar keeps where the bar of a card that scrolls without a cursor was drawn.
// The caller records the body of the card first, because only the sums that drew it know
// where it landed.
func (model *Model) recordCardScrollbar(
	thumb []string, column, top, rows, from, total int, scrolls func(*app.Overlay, int),
) {
	connection := model.Active()
	if len(thumb) == 0 || connection == nil {
		return
	}
	if scrolls == nil {
		scrolls = scrollCardByOffset
	}
	held := &connection.Overlay
	model.recordScrollbar(column, top, rows, from, total,
		func(offset int) tea.Cmd {
			scrolls(held, offset)
			return nil
		})
}

// scrollCardByOffset moves a card that keeps how far it has scrolled in its offset, which is
// every card whose list has a cursor of its own.
func scrollCardByOffset(overlay *app.Overlay, offset int) {
	overlay.List.Offset, overlay.List.Rolled = offset, true
}

// changeRows returns how many rows one staged change takes in the review: the readable
// statement, and the statement with its parameters when those are shown.
func changeRows(overlay app.Overlay) int {
	if overlay.ShowsStatements {
		return 3
	}
	return 1
}

// changeRow is one row of a staged change in the review.
type changeRow struct {
	indent, text string
	ink          color.Color
}

// renderChanges draws the staged changes of the tab, one statement each, and their buttons.
func (model *Model) renderChanges(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	scene := keyScene{overlay: overlay}
	apply := model.buildCardButton(cfg.ScopeDialog, ActionApplyChanges, "apply")
	apply.primary, apply.destructive = true, true
	buttons := []cardButton{
		apply,
		model.buildCardButton(cfg.ScopeDialog, ActionDiscardChanges, "discard"),
		model.buildCardButton(cfg.ScopeDialog, ActionToggleStatements,
			describeStatementsToggle(scene)),
		model.buildCardButton(cfg.ScopeDialog, ActionClose, "close"),
	}
	title := " staged changes · " + present.FormatCount(int64(len(overlay.Changes))) + " "
	widest := max(measureCardLines(
		model.renderChangeLines(overlay, width-present.CardChrome)), model.measureButtonRow(buttons))
	width = fitCardWidth(width, widest, 0, title, "")
	inner := width - present.CardChrome
	changes := model.renderChangeLines(overlay, inner)

	// The buttons stay under the list, so the list scrolls in the rows above them.
	contentRows := max(len(overlay.Changes)*changeRows(overlay), 1) + 2
	height := model.resolveOverlayHeight(overlay.Kind, contentRows, 0)
	listRows := max(countCardBodyRows(height, 0)-2, 1)
	scene.scrolls = len(changes) > listRows
	lines := model.scrollCardRows(len(changes), overlay.List.Offset, listRows, inner,
		theme.Panel, func(at int) string { return changes[at] })
	for len(lines) < listRows {
		lines = append(lines, "")
	}
	lines = append(lines, "")

	model.recordCardBody()
	lines = append(lines, model.renderButtonRow(buttons, noButtonFocus, cardBodyRow+len(lines), cardBodyColumn))
	card := model.renderTextCard(overlay.Kind, title, width, lines, nil, contentRows,
		destructiveCard)
	return card
}

// renderChangeLines draws the rows of every staged change on a ground that steps with the
// change, so one change is read apart from the next.
func (model *Model) renderChangeLines(overlay app.Overlay, inner int) []string {
	theme := model.styles.Theme
	if len(overlay.Changes) == 0 {
		return []string{model.styles.Muted().Render("no staged changes")}
	}
	lines := []string{}
	for at, change := range overlay.Changes {
		ground := theme.Panel
		if at%2 == 1 {
			ground = theme.Zebra
		}
		rows := []changeRow{{" ", change.Description, theme.Text}}
		if overlay.ShowsStatements {
			written := make([]string, 0, len(change.Params))
			for _, value := range change.Params {
				written = append(written, core.WriteJSONValue(value))
			}
			rows = append(rows,
				changeRow{"   ", change.Display, theme.Muted},
				changeRow{"   ", "parameters: " + strings.Join(written, ", "), theme.Muted})
		}
		// The indent of a row is its own, so only the text of the row is collapsed: a
		// statement written over several lines still takes one row.
		for _, row := range rows {
			drawn := present.TruncateText(row.indent+core.CollapseWhitespace(row.text), inner)
			lines = append(lines, padStyledOn(paintText(row.ink, ground, drawn), inner, ground))
		}
	}
	return lines
}

// renderValueFilter draws the values of one column, and which of them stay on screen.
func (model *Model) renderValueFilter(overlay app.Overlay, width int) string {
	rows := make([]string, 0, len(overlay.Values))
	for at, value := range overlay.Values {
		mark := "☐"
		if overlay.Kept[value.Value] {
			mark = "☑"
		}
		written := value.Value
		if written == "" {
			written = "(empty)"
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: mark, LeadWidth: valueMarkWidth,
			Label: written, LabelWidth: valueLabelWidth,
			Detail:   present.FormatCount(int64(value.Count)),
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}

	keys := model.buildCardKeys(app.OverlayValueFilter, keyScene{overlay: overlay})
	return model.renderListCard(ListCard{
		Kind: app.OverlayValueFilter, Title: overlay.Title, Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ContentRows: max(len(overlay.Values), 1), Keys: keys,
	})
}

// renderColumnChooser draws every column of the result, with a mark on the ones shown.
func (model *Model) renderColumnChooser(overlay app.Overlay, width int) string {
	rows := make([]string, 0, len(overlay.Values))
	for at, value := range overlay.Values {
		mark := "☐"
		if overlay.Kept[value.Value] {
			mark = "☑"
		}
		name := ""
		if at < len(overlay.Lines) {
			name = overlay.Lines[at]
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: mark, LeadWidth: valueMarkWidth, Label: name, LabelWidth: valueLabelWidth,
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	keys := model.buildCardKeys(app.OverlayColumns, keyScene{overlay: overlay})
	return model.renderListCard(ListCard{
		Kind: app.OverlayColumns, Title: overlay.Title, Note: overlay.Notice, Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled,
		Width: width, ContentRows: max(len(overlay.Values), 1), Keys: keys,
	})
}

// renderThemePicker draws every theme, and the one applied.
func (model *Model) renderThemePicker(overlay app.Overlay, width int) string {
	choices := model.filterThemes(overlay)
	rows := make([]string, 0, len(choices))
	for at, choice := range choices {
		// The theme the picker opened on carries the mark, so the reader knows what to
		// go back to while stepping through the rest.
		trail := ""
		if choice.Name == overlay.Body {
			trail = selectedThemeMark
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Label: choice.Title, LabelWidth: themeLabelWidth,
			Detail: string(choice.Appearance), Trail: trail, HasTrail: true,
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayThemePicker, Title: " theme ",
		Filter: model.renderFilterFieldOf(overlay, width, "filter themes", len(choices)), Rows: rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Rolled: overlay.List.Rolled, Width: width,
		ReportsNoMatch: true,
		ContentRows:    len(model.styles.registry.ListThemeChoices()) + 1,
		Keys:           model.buildCardKeys(app.OverlayThemePicker, keyScene{overlay: overlay}),
	})
}

// renderExport draws the file and the options an export is written with.
func (model *Model) renderExport(overlay app.Overlay, width int) string {
	fields := BuildExportFields(overlay)
	valueWidth := max(width-present.CardChrome-exportLabelWidth, 8)

	model.layout.formChoices = nil
	lines := make([]string, 0, len(fields)+3)
	for at, field := range fields {
		focused := at == overlay.Field
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}

		value := field.Value
		written := model.styles.Muted().Render(
			truncateFieldValue(field, valueWidth))
		switch {
		case len(field.Choices) > 0:
			written = model.renderChoiceField(value, valueWidth, at,
				cardBodyRow+at, cardBodyColumn+exportLabelWidth, focused)
		case focused:
			written = model.renderField(
				app.NewEditorBuffer(value, len(value)), valueWidth, FieldLook{
					Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
					Focused: true, Placeholder: field.Label,
				})
		}
		lines = append(lines, labelStyle.Render(
			marker+fitFieldLabel(field.Label, exportLabelWidth-present.MeasureText(marker)))+written)
	}

	// The problem line is always counted, so the card keeps its height.
	lines = append(lines, model.styles.Error().Render(
		present.TruncateText(FindExportProblem(overlay), width-4)))

	keys := model.buildCardKeys(app.OverlayExport, keyScene{overlay: overlay})
	// The keys are cut rather than wrapped here, because the card keeps one row for them.
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, width-4, cardBodyRow, cardBodyColumn)
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop, count: len(fields),
		from: model.layout.cardBodyLeft - 1, to: model.layout.cardBodyLeft + width - 4,
	}
	return model.renderCard(
		" export "+present.FormatRowCount(int64(overlay.Export.RowCount))+" ",
		width, lines, plainCard)
}

// exportLabelWidth is the column the mark and the name of a field of a form share.
const exportLabelWidth = 18

// fitFieldLabel holds the name of a field to the column it is given. A name too long for the
// column keeps the words that fit, because the row draws one row of the label.
func fitFieldLabel(written string, width int) string {
	wrapped := present.WrapWords(written, width)
	if len(wrapped) == 0 {
		return present.FitText(written, width)
	}
	return present.FitText(wrapped[0], width)
}

// wrapErrorText breaks a block of text into lines that fit the width, in the ink of a fault.
func (model *Model) wrapErrorText(text string, width int) []string {
	written := []string{}
	for _, line := range model.wrapText(text, width) {
		written = append(written, model.styles.Error().Render(line))
	}
	return written
}

// wrapText breaks a block of text into the lines it takes at a width. A line break of the
// text is kept, and each line between the breaks is wrapped on its own.
func (model *Model) wrapText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	written := []string{}
	for line := range strings.SplitSeq(text, "\n") {
		if line == "" {
			written = append(written, "")
			continue
		}
		written = append(written, present.WrapWords(line, width)...)
	}
	return written
}
