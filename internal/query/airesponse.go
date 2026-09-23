package query

import (
	"regexp"
	"strings"
)

// Model replies contain prose and proposed statements in code blocks.

// statementFences is the set of code block language tags recognized as statements.
const statementFences = "sql|js|javascript|mongodb|mongosh|redis"

// fencedBlock matches one fenced block, with or without the language named.
var fencedBlock = regexp.MustCompile(
	"(?is)```(?:" + statementFences + ")?[ \t]*\r?\n(.*?)```")

// The kinds of part a reply holds.
const (
	SegmentText = "text"
	SegmentSQL  = "sql"
)

// MessageSegment is one part of a reply: text, or a statement.
type MessageSegment struct {
	Kind    string
	Content string
}

// SplitMessageSegments returns a reply as its parts, in the order they were written.
func SplitMessageSegments(message string) []MessageSegment {
	segments := []MessageSegment{}
	cursor := 0

	for _, found := range fencedBlock.FindAllStringSubmatchIndex(message, -1) {
		start, end := found[0], found[1]
		if start > cursor {
			segments = append(segments,
				MessageSegment{Kind: SegmentText, Content: message[cursor:start]})
		}
		block := ""
		if found[2] != -1 {
			block = strings.TrimSpace(message[found[2]:found[3]])
		}
		segments = append(segments, MessageSegment{Kind: SegmentSQL, Content: block})
		cursor = end
	}
	if cursor < len(message) {
		segments = append(segments,
			MessageSegment{Kind: SegmentText, Content: message[cursor:]})
	}
	return segments
}

// FindSQLBlock returns the statement the assistant proposed, and whether it wrote one.
func FindSQLBlock(reply string) (string, bool) {
	for _, segment := range SplitMessageSegments(reply) {
		if segment.Kind == SegmentSQL && segment.Content != "" {
			return segment.Content, true
		}
	}
	return "", false
}

// MarkdownTable is one GitHub Flavored Markdown table of a reply.
type MarkdownTable struct {
	Headers []string
	Rows    [][]string
}

// delimiterCell matches one cell of the row under the header of a table, such as `---` or
// `:--:`.
var delimiterCell = regexp.MustCompile(`^:?-+:?$`)

// ReadMarkdownTable reads the table that starts at lines[at], and returns it with the index
// of the first line after it. It returns false where no table starts there.
func ReadMarkdownTable(lines []string, at int) (MarkdownTable, int, bool) {
	if at+1 >= len(lines) || !isTableRow(lines[at]) || !isTableRow(lines[at+1]) {
		return MarkdownTable{}, at, false
	}
	headers := splitTableRow(lines[at])
	delimiters := splitTableRow(lines[at+1])
	if len(delimiters) != len(headers) {
		return MarkdownTable{}, at, false
	}
	for _, cell := range delimiters {
		if !delimiterCell.MatchString(cell) {
			return MarkdownTable{}, at, false
		}
	}

	table := MarkdownTable{Headers: headers}
	next := at + 2
	for ; next < len(lines) && isTableRow(lines[next]); next++ {
		row := splitTableRow(lines[next])
		for len(row) < len(headers) {
			row = append(row, "")
		}
		table.Rows = append(table.Rows, row[:len(headers)])
	}
	return table, next, true
}

// isTableRow is true for a line that starts with a pipe.
func isTableRow(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

// splitTableRow returns the cells of one row of a table, trimmed.
func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for at, cell := range cells {
		cells[at] = strings.TrimSpace(cell)
	}
	return cells
}
