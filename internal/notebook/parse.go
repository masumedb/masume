package notebook

import (
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/query/statement"
)

// FrontMatterMark opens and closes the front matter block.
const FrontMatterMark = "+++"

// fenceMark opens and closes a cell.
const fenceMark = "```"

// fenceWidth is the shortest fence. A cell whose text holds a fence is written with a longer
// one, and closes on a line of at least as many marks.
const fenceWidth = len(fenceMark)

// Parse reads the text of a notebook file.
func Parse(text string) Notebook {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	book := Notebook{Run: DefaultPolicy()}

	front, body := cutFrontMatter(lines)
	if front != "" {
		book.FrontMatter = front
		readFrontMatter(&book, front)
	}
	book.Cells = readCells(body)
	nameCells(&book)
	return book
}

// cutFrontMatter returns the front matter and the lines after it.
func cutFrontMatter(lines []string) (string, []string) {
	at := 0
	for at < len(lines) && strings.TrimSpace(lines[at]) == "" {
		at++
	}
	if at >= len(lines) || strings.TrimSpace(lines[at]) != FrontMatterMark {
		return "", lines
	}
	for end := at + 1; end < len(lines); end++ {
		if strings.TrimSpace(lines[end]) == FrontMatterMark {
			return strings.Join(lines[at+1:end], "\n"), lines[end+1:]
		}
	}
	return "", lines
}

// readFrontMatter reads the title, the profiles, the engine and the run policy.
func readFrontMatter(book *Notebook, front string) {
	document, err := cfg.DecodeDocument(front)
	if err != nil {
		book.Problems = append(book.Problems, "the front matter cannot be read: "+err.Error())
		book.UnreadableFrontMatter = true
		return
	}
	if title, held := cfg.FindString(document, "title"); held {
		book.Title = title
	}
	if profiles, held := cfg.FindStringList(document, "profiles"); held {
		book.Profiles = profiles
	}
	if engine, held := cfg.FindString(document, "engine"); held {
		book.Engine = engine
	}
	run, held := cfg.FindSection(document, "run")
	if !held {
		return
	}
	if written, found := cfg.FindString(run, "transaction"); found {
		if written != TransactionAutocommit && written != TransactionSingle {
			book.Problems = append(book.Problems,
				"transaction must be "+TransactionAutocommit+" or "+TransactionSingle)
		} else {
			book.Run.Transaction = written
		}
	}
	if written, found := cfg.FindString(run, "on_error"); found {
		if written != ErrorStop && written != ErrorContinue {
			book.Problems = append(book.Problems,
				"on_error must be "+ErrorStop+" or "+ErrorContinue)
		} else {
			book.Run.OnError = written
		}
	}
}

// readCells returns one cell per fence, and one text cell for the prose between two fences.
func readCells(lines []string) []Cell {
	cells := []Cell{}
	prose := []string{}

	keepProse := func() {
		if written := strings.Trim(strings.Join(prose, "\n"), "\n"); written != "" {
			cells = append(cells, Cell{Kind: CellText, Source: written})
		}
		prose = nil
	}

	for at := 0; at < len(lines); at++ {
		width, info, opens := cutFenceOpening(lines[at])
		if !opens {
			prose = append(prose, lines[at])
			continue
		}
		keepProse()
		source, after := readFenceBody(lines, at+1, width)
		at = after
		cells = append(cells, buildCell(info, source))
	}
	keepProse()
	return cells
}

// cutFenceOpening returns the marks of a fence line and the info text after them.
func cutFenceOpening(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	marks := countFenceMarks(trimmed)
	if marks < fenceWidth {
		return 0, "", false
	}
	return marks, strings.TrimSpace(trimmed[marks:]), true
}

// countFenceMarks returns the marks that open the line.
func countFenceMarks(trimmed string) int {
	marks := 0
	for marks < len(trimmed) && trimmed[marks] == '`' {
		marks++
	}
	return marks
}

// closesFence is true for a line of fence marks alone, as many as the fence that opened.
func closesFence(line string, width int) bool {
	trimmed := strings.TrimSpace(line)
	marks := countFenceMarks(trimmed)
	return marks >= width && marks == len(trimmed)
}

// readFenceBody returns the text up to the closing fence, and the line the fence closed on.
func readFenceBody(lines []string, from, width int) (string, int) {
	body := []string{}
	for at := from; at < len(lines); at++ {
		if closesFence(lines[at], width) {
			return strings.Join(body, "\n"), at
		}
		body = append(body, lines[at])
	}
	return strings.Join(body, "\n"), len(lines)
}

// buildCell returns the cell of one fence.
func buildCell(info, source string) Cell {
	language, attrs := readFenceInfo(info)
	cell := Cell{Kind: CellOther, Fence: info, Source: source, Attrs: attrs}
	switch language {
	case "sql":
		cell.Kind, cell.Fence = CellSQL, ""
	case "param":
		cell.Kind, cell.Fence = CellParam, ""
	case "chart":
		cell.Kind, cell.Fence = CellChart, ""
	case "md", "markdown":
		cell.Kind, cell.Fence = CellText, ""
	}
	if id, held := cell.FindAttr("id"); held {
		cell.ID = id
	}
	return cell
}

// readFenceInfo returns the language of a fence and its pairs.
func readFenceInfo(info string) (string, []Attribute) {
	parts := strings.Fields(info)
	if len(parts) == 0 {
		return "", nil
	}
	attrs := []Attribute{}
	for _, part := range parts[1:] {
		name, value, held := strings.Cut(part, "=")
		if !held {
			continue
		}
		attrs = append(attrs, Attribute{Name: name, Value: strings.Trim(value, `"`)})
	}
	return strings.ToLower(parts[0]), attrs
}

// nameCells gives every cell an id no other cell has.
func nameCells(book *Notebook) {
	taken := map[string]bool{}
	for at := range book.Cells {
		cell := &book.Cells[at]
		wanted := cell.ID
		if wanted == "" {
			wanted = BuildSlug(statement.FindQueryName(cell.Source))
		}
		if wanted == "" {
			wanted = "cell-" + strconv.Itoa(at+1)
		}
		cell.ID = resolveFreeID(wanted, taken)
	}
}

// resolveFreeID returns the id, or the id with a number where it is taken.
func resolveFreeID(wanted string, taken map[string]bool) string {
	id := wanted
	for count := 2; taken[id]; count++ {
		id = wanted + "-" + strconv.Itoa(count)
	}
	taken[id] = true
	return id
}
