package ui

import (
	"strconv"
	"unicode/utf8"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query/editor"
)

// wrapRow is one row of the editor while long lines wrap: the line, the bytes of the line the
// row draws, and the cell of the line the row starts at.
type wrapRow struct {
	line      int
	from, to  int
	startCell int
	last      bool
}

// buildWrapRows splits every line into rows of at most width cells. A row breaks after the
// last blank in it, and inside a word that has no blank.
func buildWrapRows(lines []string, width int) []wrapRow {
	rows := []wrapRow{}
	for index, line := range lines {
		from, cells, breakAt := 0, 0, -1
		for at := 0; at < len(line); {
			character, size := utf8.DecodeRuneInString(line[at:])
			cell := present.MeasureText(string(character))
			if cells > 0 && cells+cell > width {
				cut := at
				if breakAt > from {
					cut = breakAt
				}
				rows = append(rows, wrapRow{
					line: index, from: from, to: cut,
					startCell: present.MeasureText(line[:from]),
				})
				from, breakAt = cut, -1
				cells = present.MeasureText(line[from:at])
			}
			cells += cell
			at += size
			if character == ' ' {
				breakAt = at
			}
		}
		rows = append(rows, wrapRow{
			line: index, from: from, to: len(line),
			startCell: present.MeasureText(line[:from]), last: true,
		})
	}
	return rows
}

// findWrapRow returns the row the byte of the line stands in.
func findWrapRow(rows []wrapRow, line, column int) int {
	for at, row := range rows {
		if row.line == line && column >= row.from && (column < row.to || row.last) {
			return at
		}
	}
	return max(len(rows)-1, 0)
}

// holdsCaret is true for the row the caret of the line stands in.
func (row wrapRow) holdsCaret(line, column int) bool {
	return row.line == line && column >= row.from && (column < row.to || row.last)
}

// wrappedEditor is what the wrapped rows of the editor are drawn from.
type wrappedEditor struct {
	lines                  []string
	faults                 []editor.Diagnostic
	body                   int
	gutterWidth, textWidth int
	focused                bool
}

// renderWrappedLines draws the statement with every long line wrapped onto the rows under it.
// The first row of a line carries its number.
func (model *Model) renderWrappedLines(
	connection *app.Connection, tab *app.Tab, held wrappedEditor,
) []string {
	theme := model.styles.Theme
	lines, body := held.lines, held.body
	caretLine, caretColumn := tab.Editor.CaretPosition()

	rows := buildWrapRows(lines, held.textWidth)
	caretRow := findWrapRow(rows, caretLine, caretColumn)
	offset := tab.EditorRowOffset
	if !tab.EditorRolled {
		offset = scrollTo(caretRow, tab.EditorRowOffset, body, len(rows))
	}
	offset = clampOffset(offset, body, len(rows))
	tab.EditorRowOffset, tab.EditorColumnOffset = offset, 0

	caretCell := present.MeasureText(lines[caretLine][:caretColumn]) - rows[caretRow].startCell
	model.caretRow = model.paneTop + (caretRow - offset)
	model.caretColumn = model.editorLeft + 1 + held.gutterWidth + caretCell

	model.layout.editorTextLeft = model.editorLeft + 1 + held.gutterWidth
	model.layout.editorTextTop = model.paneTop + 1
	model.layout.editorTextWidth = held.textWidth
	model.layout.editorTextRows = body
	model.layout.editorFirstLine = rows[min(offset, len(rows)-1)].line
	model.layout.editorColumnOffset = 0
	shown := rows[min(offset, len(rows)):min(offset+body, len(rows))]
	model.layout.editorWrapRows = shown

	selectFrom, selectTo := tab.Editor.SelectionRange()
	selects := tab.Editor.HasSelection()
	lineStarts := make([]int, len(lines))
	for at := 1; at < len(lines); at++ {
		lineStarts[at] = lineStarts[at-1] + len(lines[at-1]) + 1
	}

	firstLine, lastLine := 0, 0
	if len(shown) > 0 {
		firstLine, lastLine = shown[0].line, shown[len(shown)-1].line+1
	}
	highlights := model.buildEditorHighlights(
		connection, tab, lines, held.faults, firstLine, lastLine)
	faulty := findFaultyLines(tab, held.faults)

	written := make([]string, 0, body)
	for _, row := range shown {
		onCaretLine := row.line == caretLine && held.focused
		gutterGround := theme.Panel
		if onCaretLine {
			gutterGround = theme.Zebra
		}
		sign := paintOn(gutterGround, " ")
		if faulty[row.line] && row.from == 0 {
			sign = paintText(theme.Error, gutterGround, model.describeProblemSign())
		}
		numberInk := theme.Faint
		if onCaretLine {
			numberInk = theme.Accent
		}
		label := ""
		if row.from == 0 {
			label = strconv.Itoa(row.line + 1)
		}
		number := sign + paintText(numberInk, gutterGround,
			buildGutterText(label, held.gutterWidth-1))

		drawn := codeLine{
			text: lines[row.line][:row.to], spans: highlights[row.line],
			width: held.textWidth, columnOffset: row.startCell, caretColumn: caretColumn,
			showCaret: held.focused && row.holdsCaret(caretLine, caretColumn),
		}
		if selects {
			from, to := resolveLineSelection(selectFrom-lineStarts[row.line],
				selectTo-lineStarts[row.line], len(lines[row.line]))
			if !row.last {
				to = min(to, row.to)
			}
			drawn.selectFrom, drawn.selectTo = from, to
		}
		written = append(written, number+model.renderCodeLine(drawn)+paintOn(gutterGround, " "))
	}
	return written
}
