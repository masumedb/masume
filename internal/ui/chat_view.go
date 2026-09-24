package ui

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/hist"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query"
)

// The chat panel contains the conversation, reply status, and question field.

// The question field grows from three to six rows.
const (
	chatFieldRowsLeast = 1
	chatFieldRowsMost  = 6
	chatNoticeRows     = 1
)

// resolveChatFieldRows returns the rows the field takes for this question: three while it is
// short, one per line as it grows, and never more than six.
func resolveChatFieldRows(buffer *app.EditorBuffer) int {
	if buffer == nil {
		return chatFieldRowsLeast
	}
	rows := len(buffer.Lines())
	if rows < chatFieldRowsLeast {
		return chatFieldRowsLeast
	}
	if rows > chatFieldRowsMost {
		return chatFieldRowsMost
	}
	return rows
}

// chatOpening is what the panel says before the first question, with the questions it offers.
var chatOpening = "Ask about this database, request a query, or ask for a count. " +
	"The chat asks for approval before running a statement to answer a data question."

// chatExamples are example questions, one line each, so a narrow panel keeps them whole.
var chatExamples = []string{
	"how many orders are unpaid",
	"write me a query for unpaid orders",
	"why does this fail",
}

// The width the scroll bar of the conversation keeps on the right.
const chatTrackWidth = 1

// renderAiChat draws the panel of the chat of this connection.
func (model *Model) renderAiChat(
	connection *app.Connection, overlay app.Overlay, width int,
) string {
	chat := connection.Chat
	content := max(width-present.CardChrome, 1)

	keys := model.describeChatKeys(chat)
	body := model.resolveChatBodyRows(connection, width, content)

	// The field is pinned to the foot of the panel, and the conversation takes what is left.
	below, answersRow := model.renderChatBelow(overlay, chat, content)
	room := max(body-len(below), 1)
	shown := model.renderChatBody(connection, chat, content, room)
	lines := append(shown, below...)

	model.recordCardBody()
	model.recordChatScrollbar(connection, content, room, len(shown))
	if answersRow >= 0 {
		// The row of returns opens with a blank column, so the first chip starts one
		// column in.
		left := model.layout.cardBodyLeft + 1
		row := model.layout.cardBodyTop + len(shown) + answersRow
		model.layout.overlayChips = []chipHit{
			{index: 0, row: row, from: left,
				to: left + present.MeasureText(chatRunChip) - 1},
			{index: 1, row: row, from: left + present.MeasureText(chatRunChip) + 1,
				to: left + present.MeasureText(chatRunChip) + 1 +
					present.MeasureText(chatRefuseChip) - 1},
		}
	}
	return model.renderNotedTextCard(app.OverlayAiChat,
		" "+model.icons.Prefix(cfg.IconAi)+"AI chat · "+
			model.describeChatSource()+" ",
		"", model.renderChatTokens(chat), width, lines, keys, 0, plainCard)
}

// renderChatTokens draws the tokens the chat has spent this session, for the bottom border.
func (model *Model) renderChatTokens(chat *app.Chat) string {
	total := chat.Usage.InputTokens + chat.Usage.OutputTokens
	if total == 0 {
		return ""
	}
	return model.styles.Faint().Background(model.styles.Theme.Panel).
		Render(" " + present.FormatCountOf(int64(total), "token", "tokens") + " ")
}

// resolveChatBodyRows returns the rows the panel gives the conversation and the field under
// it: the height of the card, less its border, the rows the keys wrap to and the row over
// them. The draw and the scroll bounds both read it here, so neither counts rows the other
// did not draw.
func (model *Model) resolveChatBodyRows(
	connection *app.Connection, width, content int,
) int {
	text := model.describeChatKeys(connection.Chat).buildText()
	height := model.resolveOverlayHeight(
		app.OverlayAiChat, 0, countHintRows(text, width))
	return max(height-present.CardChrome-countHintLines(text, content)-1, 1)
}

// chatViewRows returns how many rows of the conversation the panel shows. The panel is drawn
// from the height of the screen, so the same sums answer for the keys.
func (model *Model) chatViewRows(connection *app.Connection) int {
	width := model.resolveOverlayWidth(app.OverlayAiChat)
	content := max(width-present.CardChrome, 1)
	below, _ := model.renderChatBelow(connection.Overlay, connection.Chat, content)
	return max(model.resolveChatBodyRows(connection, width, content)-len(below), 1)
}

// chatLastOffset returns how far the conversation can be scrolled.
func (model *Model) chatLastOffset(connection *app.Connection) int {
	highest := len(model.chatRowsOf(connection)) - model.chatViewRows(connection)
	if highest < 0 {
		return 0
	}
	return highest
}

// findChatTurnRow returns the row one turn of the conversation begins on.
func (model *Model) findChatTurnRow(connection *app.Connection, turn int) int {
	_, starts := model.resolveChatRows(connection, model.resolveChatContent())
	if turn <= 0 || len(starts) == 0 {
		return 0
	}
	if turn >= len(starts) {
		turn = len(starts) - 1
	}
	return starts[turn]
}

// chatRowsOf returns the rows the conversation takes as it is drawn now.
func (model *Model) chatRowsOf(connection *app.Connection) []string {
	rows, _ := model.resolveChatRows(connection, model.resolveChatContent())
	return rows
}

// resolveChatContent returns the width the conversation is wrapped to: the width of the panel
// it is drawn in, less its border and the track of its scroll bar. The scroll bounds, a jump
// between turns and the draw all read it here, so none of them wraps to a width another one
// did not draw.
func (model *Model) resolveChatContent() int {
	content := max(model.resolveOverlayWidth(app.OverlayAiChat)-present.CardChrome, 1)
	return content - chatTrackWidth
}

// chatRowsCache holds the conversation as rows, and what they were drawn for.
type chatRowsCache struct {
	drawn  bool
	key    chatRowsKey
	rows   []string
	starts []int
}

// chatRowsKey identifies everything the rows of the conversation are drawn from. The turns
// are read in full: a reply is written into the last one while it streams, so its length
// alone does not say that it changed.
type chatRowsKey struct {
	// The connection ID for this conversation.
	connection int
	content    int
	revision   int
	// The panel ground, which is lifted while a card is drawn.
	ground   string
	messages uint64
	hasTurn  bool
	turnAt   int
	// The steps, the label and the wheel belong to the reply being written, and are read
	// only while one is.
	streaming bool
	activity  string
	startedAt int64
	spinnerAt int
	// The chord on the label of the most recent query.
	insertChord string
}

// resolveChatRows returns the conversation as rows, and the row each turn begins on, drawing
// them only where the ones it kept belong to another conversation, another width or another
// moment of a reply being written.
func (model *Model) resolveChatRows(
	connection *app.Connection, content int,
) ([]string, []int) {
	chat := connection.Chat
	key := model.buildChatRowsKey(connection, chat, content)
	if model.chatRows.drawn && model.chatRows.key == key {
		return model.chatRows.rows, model.chatRows.starts
	}
	rows, starts := model.renderChatTurns(chat, content)
	model.chatRows = chatRowsCache{drawn: true, key: key, rows: rows, starts: starts}
	return rows, starts
}

// buildChatRowsKey reads the state the rows of the conversation are drawn from.
func (model *Model) buildChatRowsKey(
	connection *app.Connection, chat *app.Chat, content int,
) chatRowsKey {
	key := chatRowsKey{
		connection: model.connections.idOf(connection),
		content:    content, revision: model.styles.Revision(),
		ground:   WriteHex(model.styles.Theme.Panel),
		messages: hashChatMessages(chat.Messages),
		hasTurn:  chat.HasTurn, turnAt: chat.TurnAt,
		insertChord: model.registry.FormatFirstActionChord(cfg.ScopeDialog, ActionInsertAiSQL),
	}
	if chat.IsStreaming() && chat.Pending == nil {
		key.streaming = true
		key.activity = chat.Activity
		key.startedAt = chat.StartedAt.UnixNano()
		key.spinnerAt = model.spinnerAt
	}
	return key
}

// hashChatMessages reads every turn of the conversation, so a turn whose text or calls
// changed while the count stayed the same is drawn again.
func hashChatMessages(messages []app.ChatMessage) uint64 {
	digest := fnv.New64a()
	for _, message := range messages {
		writeHashedText(digest, message.Role)
		writeHashedText(digest, message.Content)
		for _, part := range message.Parts {
			writeHashedText(digest, part.Step)
			digest.Write([]byte{markDoneStep(part.Done)})
		}
	}
	return digest.Sum64()
}

// markDoneStep returns the byte that tells a call that finished from one that runs.
func markDoneStep(done bool) byte {
	if done {
		return 1
	}
	return 0
}

// writeHashedText adds one piece of text to a digest, with its length, so two lists of pieces
// that run together to the same text read apart.
func writeHashedText(digest hash.Hash64, text string) {
	var counted [8]byte
	binary.LittleEndian.PutUint64(counted[:], uint64(len(text)))
	digest.Write(counted[:])
	digest.Write([]byte(text))
}

// renderChatBody draws the conversation, or what the panel says before the first question.
func (model *Model) renderChatBody(
	connection *app.Connection, chat *app.Chat, content, room int,
) []string {
	if len(chat.Messages) == 0 {
		return model.renderChatOpening(connection, content, room)
	}

	rows, _ := model.resolveChatRows(connection, model.resolveChatContent())
	chat.Offset = resolveChatOffset(chat, len(rows), room)

	// The bar stands beside the conversation, and a conversation that fits gets none.
	thumb := buildScrollThumb(chat.Offset, room, len(rows))
	lines := make([]string, 0, room)
	for at := range room {
		line := ""
		if chat.Offset+at < len(rows) {
			line = rows[chat.Offset+at]
		}
		lines = append(lines, model.paintChatTrack(line, thumb, at, content))
	}
	return lines
}

// recordChatScrollbar keeps where the bar beside the conversation was drawn, so a drag of its
// thumb moves the conversation as the wheel does.
func (model *Model) recordChatScrollbar(
	connection *app.Connection, content, room, drawn int,
) {
	chat := connection.Chat
	rows, _ := model.resolveChatRows(connection, model.resolveChatContent())
	if len(rows) <= room || drawn < 1 {
		return
	}
	model.recordScrollbar(
		cardBodyColumn+content-chatTrackWidth, cardBodyRow, min(room, drawn),
		chat.Offset, len(rows),
		func(offset int) tea.Cmd {
			// The mark belongs to a jump between turns, and this is a scroll.
			chat.HasTurn, chat.Notice = false, ""
			chat.Offset = offset
			chat.Follow = offset >= model.chatLastOffset(connection)
			return nil
		})
}

// paintChatTrack writes the track of the bar beside one row of the conversation.
func (model *Model) paintChatTrack(line string, thumb []string, at, content int) string {
	if len(thumb) == 0 {
		return line
	}
	glyph := " "
	if at < len(thumb) && thumb[at] != "" {
		glyph = thumb[at]
	}
	ground := model.styles.Theme.Panel
	return padStyledOn(line, content-chatTrackWidth, ground) +
		model.styles.renderThumbCell(glyph, ground)
}

// resolveChatOffset returns how far the conversation is scrolled. It follows the newest row
// until the reader scrolls away from it.
func resolveChatOffset(chat *app.Chat, rows, room int) int {
	highest := max(rows-room, 0)
	if chat.Follow {
		return highest
	}
	return core.ClampWithin(chat.Offset, highest)
}

// renderChatOpening draws what the panel says before the first question.
func (model *Model) renderChatOpening(
	connection *app.Connection, content, room int,
) []string {
	theme := model.styles.Theme
	lines := []string{}
	for _, line := range present.WrapWords(chatOpening, content) {
		lines = append(lines, model.styles.Ink().Render(line))
	}
	lines = append(lines, "")
	for _, asked := range chatExamples {
		lines = append(lines, paintText(theme.Info, nil, "  "+asked))
	}
	// A source that answers nothing is named before the first question rather than after
	// it.
	if missing := model.describeChatProblem(connection.Profile()); missing != "" {
		lines = append(lines, "")
		for _, line := range present.WrapWords(missing, content) {
			lines = append(lines, model.styles.Error().Render(line))
		}
	}
	for len(lines) < room {
		lines = append(lines, "")
	}
	return lines[:room]
}

// renderChatTurns draws every turn of the conversation, one row per line, and returns the row
// each turn begins on beside them.
func (model *Model) renderChatTurns(chat *app.Chat, content int) ([]string, []int) {
	rows := []string{}
	starts := make([]int, 0, len(chat.Messages))
	queryTurn := findLastQueryTurn(chat)
	for at, message := range chat.Messages {
		starts = append(starts, len(rows))
		marked := chat.HasTurn && chat.TurnAt == at
		written := model.renderChatTurn(chat, message, at, content-2, at == queryTurn)
		for _, line := range written {
			rows = append(rows, model.markChatRow(marked)+line)
		}
		// The turn keeps a blank row under it, so two turns are told apart.
		rows = append(rows, model.markChatRow(marked))
	}
	return rows, starts
}

// findLastQueryTurn returns the turn with the query that the insert key reads, or -1.
func findLastQueryTurn(chat *app.Chat) int {
	for at := len(chat.Messages) - 1; at >= 0; at-- {
		message := chat.Messages[at]
		if message.Role != hist.ChatRoleAssistant {
			continue
		}
		if _, wrote := query.FindSQLBlock(message.Content); wrote {
			return at
		}
	}
	return -1
}

// markChatRow draws the column that marks the turn a jump landed on. The column is there
// whether or not it is coloured, so the text of a turn does not shift when the walk moves off
// it.
func (model *Model) markChatRow(marked bool) string {
	ground := model.styles.Theme.Panel
	if marked {
		ground = model.styles.Theme.Accent
	}
	return paintOn(ground, " ") +
		paintOn(model.styles.Theme.Panel, " ")
}

// renderChatTurn draws one turn: the speaker, and the message in the order it arrived. A
// call of the model stands where it happened, between the blocks of text around it.
func (model *Model) renderChatTurn(
	chat *app.Chat, message app.ChatMessage, at, content int, labelsQuery bool,
) []string {
	writing := model.writesChatReply(chat, at)
	parts := model.readChatParts(message)
	running := false

	lines := []string{model.renderChatRole(message.Role)}
	for _, part := range parts {
		if !part.IsStep() {
			blocks, labeled := model.renderChatBlocks(part.Text, content, labelsQuery)
			lines = append(lines, blocks...)
			labelsQuery = labelsQuery && !labeled
			continue
		}
		// The call that runs carries the wheel, so it is drawn once and not twice.
		if writing && !part.Done {
			running = true
			lines = append(lines, model.renderThinkingLine(
				part.Step, chat.StartedAt, model.styles.Theme.Panel))
			continue
		}
		lines = append(lines, model.renderChatStep(part, content))
	}
	// A reply that runs no call is thinking, and says so on a row of its own.
	if writing && !running {
		lines = append(lines, model.renderThinkingLine(
			"Thinking", chat.StartedAt, model.styles.Theme.Panel))
	}
	return lines
}

// readChatParts returns the pieces of one turn. A question, and a reply stored before the
// pieces were kept, are one block of text.
func (model *Model) readChatParts(message app.ChatMessage) []app.ChatPart {
	if len(message.Parts) > 0 {
		return message.Parts
	}
	if message.Content == "" {
		return nil
	}
	return []app.ChatPart{{Text: message.Content}}
}

// renderChatBlocks draws one block of a turn, with a statement drawn as code. With
// labelsQuery, the first statement has the key that inserts it, and the second return value
// is true.
func (model *Model) renderChatBlocks(
	written string, content int, labelsQuery bool,
) ([]string, bool) {
	lines := []string{}
	labeled := false
	for _, segment := range query.SplitMessageSegments(written) {
		if segment.Kind == query.SegmentSQL {
			label := labelsQuery && !labeled && segment.Content != ""
			lines = append(lines, model.renderChatCode(segment.Content, content, label)...)
			labeled = labeled || label
			continue
		}
		lines = append(lines, model.renderChatText(segment.Content, content)...)
	}
	return lines, labeled
}

// writesChatReply is true where this turn is the reply being written.
func (model *Model) writesChatReply(chat *app.Chat, at int) bool {
	return chat.IsStreaming() && chat.Pending == nil && at == len(chat.Messages)-1
}

// renderChatRole draws who is speaking. Gold marks the user, as everywhere else. The chip is
// held to its own text: a gold band the width of the panel says nothing that three letters do
// not.
func (model *Model) renderChatRole(role string) string {
	theme := model.styles.Theme
	if role == hist.ChatRoleUser {
		return paintText(theme.OnAccent, theme.Accent, " you ")
	}
	return paintText(theme.Info, nil, "assistant")
}

// renderChatText draws the prose of a turn, wrapped at the width of the panel, with each
// Markdown table drawn as aligned columns.
func (model *Model) renderChatText(written string, content int) []string {
	lines := []string{}
	// Trimmed, because the block of code below already has the blank line.
	paragraphs := strings.Split(strings.TrimSpace(written), "\n")
	for at := 0; at < len(paragraphs); {
		if table, next, found := query.ReadMarkdownTable(paragraphs, at); found {
			lines = append(lines, model.renderChatTable(table, content)...)
			at = next
			continue
		}
		for _, line := range present.WrapWords(paragraphs[at], content) {
			lines = append(lines, model.styles.Ink().Render(line))
		}
		at++
	}
	return lines
}

// renderChatTable draws a Markdown table: the header in bold, a rule under it, and every
// column of numbers against its right edge.
func (model *Model) renderChatTable(table query.MarkdownTable, content int) []string {
	theme := model.styles.Theme
	widths := present.PlanDetailColumns(table.Headers, table.Rows, content, detailGap)
	numeric := findNumericTableColumns(table)
	gap := strings.Repeat(" ", detailGap)

	fitRow := func(cells []string) string {
		fitted := make([]string, len(cells))
		for at, cell := range cells {
			if numeric[at] {
				fitted[at] = present.FitTextRight(cell, widths[at])
				continue
			}
			fitted[at] = present.FitText(cell, widths[at])
		}
		return present.TruncateText(strings.Join(fitted, gap), content)
	}
	rules := make([]string, len(widths))
	for at, width := range widths {
		rules[at] = strings.Repeat("─", width)
	}

	lines := []string{
		model.styles.Bold(theme.Text, theme.Panel).Render(fitRow(table.Headers)),
		model.styles.Faint().Render(present.TruncateText(strings.Join(rules, gap), content)),
	}
	for _, row := range table.Rows {
		lines = append(lines, model.styles.Ink().Render(fitRow(row)))
	}
	return lines
}

// findNumericTableColumns returns the columns where every cell with text is a number.
func findNumericTableColumns(table query.MarkdownTable) map[int]bool {
	numeric := map[int]bool{}
	for column := range table.Headers {
		held := false
		for _, row := range table.Rows {
			if row[column] == "" {
				continue
			}
			if !present.IsNumberText(row[column]) {
				held = false
				break
			}
			held = true
		}
		numeric[column] = held
	}
	return numeric
}

// renderChatCode draws a statement the model proposed, coloured as the editor colours one.
// With label, the row under it has the key that inserts it.
func (model *Model) renderChatCode(sql string, content int, label bool) []string {
	ground := model.styles.Theme.Header
	held := strings.Split(sql, "\n")
	highlights := buildSQLLineHighlights(sql)

	// A blank row above and below, so the block stands apart from the prose.
	lines := []string{""}
	for at, line := range held {
		lines = append(lines,
			paintOn(ground, " ")+
				model.renderCodeLineOn(ground, codeLine{
					text: line, spans: highlights[at], width: content - 1}))
	}
	chord := model.registry.FormatFirstActionChord(cfg.ScopeDialog, ActionInsertAiSQL)
	if label && chord != "" {
		lines = append(lines, model.styles.Faint().Render(
			present.TruncateText(" "+chord+" insert", content)))
	}
	return append(lines, "")
}

// renderChatStep draws one call of a reply, where it happened.
func (model *Model) renderChatStep(part app.ChatPart, content int) string {
	theme := model.styles.Theme
	mark, ink := "✓ ", theme.Success
	if !part.Done {
		mark, ink = "· ", theme.Faint
	}
	return paintText(ink, nil, mark) +
		paintText(theme.Faint, nil, present.TruncateText(part.Step, content-2))
}

// renderChatBelow draws everything under the conversation: what failed, the statement that
// waits for a yes, the field, and the most recent report.
func (model *Model) renderChatBelow(
	overlay app.Overlay, chat *app.Chat, content int,
) ([]string, int) {
	theme := model.styles.Theme
	lines := []string{}
	answersRow := -1

	if chat.Status == app.ChatFailed && chat.Problem != "" {
		lines = append(lines, paintText(model.styles.InkOn(theme.Error), theme.Error, " failed "))
		for _, line := range present.WrapWords(chat.Problem, content-1) {
			lines = append(lines,
				paintText(theme.Error, nil, " "+line))
		}
	}
	if chat.Pending != nil {
		pending := model.renderChatPending(*chat.Pending, content)
		// The answers stand on the last row the question takes.
		answersRow = len(lines) + len(pending) - 1
		lines = append(lines, pending...)
	}

	// The field carries the marker in its placeholder rather than beside it, because it takes
	// the width of the row whatever sits next to it. A question is written over several
	// lines, so the field is as many rows and each of them is a row of the card.
	field := model.renderDraftRows(
		overlay.Draft, content-1, resolveChatFieldRows(overlay.Draft), FieldLook{
			Ground: theme.Header, Ink: theme.Text,
			Focused: chat.Pending == nil, Placeholder: chatPlaceholder,
			KeepsPlaceholder: true,
		})
	lines = append(lines, "")
	for _, row := range field {
		lines = append(lines, paintOn(theme.Header, " ")+row)
	}

	return append(lines, model.styles.Faint().Render(
		present.TruncateText(chat.Notice, content))), answersRow
}

// chatPlaceholder is what the field says while nothing is typed into it.
const chatPlaceholder = "› ask about this database, or for a query"

// The two answers to a statement the chat wants to run.
const (
	chatRunChip    = " y run "
	chatRefuseChip = " n do not run "
)

// renderChatPending draws the statement the chat wants to run, and the two answers.
func (model *Model) renderChatPending(pending app.PendingRun, content int) []string {
	theme := model.styles.Theme
	ground := theme.Header
	filled := lipgloss.NewStyle().Background(ground)

	lines := []string{"", filled.Render(" ") +
		padStyledOn(paintText(theme.Error, ground, present.TruncateText("run this? it "+pending.Summary, content-2)),
			content-1, ground)}
	for _, line := range pending.Plan {
		lines = append(lines, filled.Render(" ")+padStyledOn(
			paintText(theme.Muted, ground, present.TruncateText(line, content-2)),
			content-1, ground))
	}
	highlights := buildSQLLineHighlights(pending.SQL)
	for at, line := range strings.Split(pending.SQL, "\n") {
		lines = append(lines, filled.Render(" ")+
			model.renderCodeLineOn(ground, codeLine{
				text: line, spans: highlights[at], width: content - 1}))
	}

	// Each chip keeps a blank column after it, so the two answers do not run together.
	yes := paintText(model.styles.InkOn(theme.Error), theme.Error, chatRunChip)
	no := paintText(theme.Text, theme.Border, chatRefuseChip)
	answers := yes + filled.Render(" ") + no + filled.Render(" ")
	return append(lines, filled.Render(strings.Repeat(" ", content)),
		filled.Render(" ")+padStyledOn(answers, content-1, ground))
}

// describeChatKeys returns the keys of the chat panel: the answers to a question while one
// waits, and the keys of the panel otherwise.
func (model *Model) describeChatKeys(chat *app.Chat) *KeyLine {
	return model.buildCardKeys(app.OverlayAiChat, keyScene{chat: chat})
}

// The columns a row of the list of conversations keeps for the title and the time.
const (
	chatTitleWidth = 52
	chatWhenWidth  = 12
)

// renderAiChats draws the conversations of this profile, the most recent first.
func (model *Model) renderAiChats(
	connection *app.Connection, overlay app.Overlay, width int,
) string {
	chat := connection.Chat
	held := model.filterConversations(overlay, chat)
	now := time.Now()

	rows := make([]string, 0, len(held))
	for at, conversation := range held {
		trail := ""
		if conversation.ID == chat.OpenID {
			trail = "open"
		}
		rows = append(rows, model.renderListRow(ListRowSpec{
			Lead: present.FormatWhen(conversation.UpdatedAt, now), LeadWidth: chatWhenWidth,
			Label: conversation.Title, LabelWidth: chatTitleWidth,
			Detail: describeTurnCount(conversation.TurnCount),
			Trail:  trail, HasTrail: trail != "",
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}

	keys := model.buildCardKeys(app.OverlayAiChats, keyScene{overlay: overlay})
	return model.renderListCard(ListCard{
		Kind: app.OverlayAiChats, Title: " " + model.icons.Prefix(cfg.IconAi) + "AI chats ",
		Filter: model.renderFilterFieldOf(
			overlay, width, "search the conversations", len(held)),
		Rows: rows, Cursor: overlay.List.Cursor, Offset: overlay.List.Offset, Width: width,
		ReportsNoMatch: true, Keys: keys, ContentRows: len(chat.Conversations) + 1,
	})
}

// describeTurnCount writes how many turns a conversation holds.
func describeTurnCount(count int) string {
	return present.FormatCountOf(int64(count), "turn", "turns")
}

// filterConversations returns the conversations the term of the overlay keeps.
func (model *Model) filterConversations(
	overlay app.Overlay, chat *app.Chat,
) []hist.ChatConversation {
	term := strings.TrimSpace(overlay.Draft.Text)
	if term == "" {
		return chat.Conversations
	}
	kept := []hist.ChatConversation{}
	for _, conversation := range chat.Conversations {
		if present.MatchesText(conversation.Title, term) {
			kept = append(kept, conversation)
		}
	}
	return kept
}
