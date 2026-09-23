package present

import (
	"strings"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
)

// An ER diagram drawn with box characters: the table in the middle, the tables that refer to
// it on the left, and the tables it refers to on the right.

// DiagramColumn is one column of a box of the diagram.
type DiagramColumn struct {
	Name    string
	Type    string
	Primary bool
	Foreign bool
}

// DiagramTable is one table of a diagram.
type DiagramTable struct {
	Schema      string
	Name        string
	Columns     []DiagramColumn
	ForeignKeys []query.ForeignKey
}

// The size of one box, and the number of columns it shows.
const (
	diagramBoxWidth    = 30
	diagramGap         = 8
	diagramHeaderLines = 3
	diagramMaxColumns  = 10
	diagramTypeWidth   = 8
	diagramNameWidth   = diagramBoxWidth - 2 - 3 - diagramTypeWidth - 2
)

// DiagramMarks are the glyphs of a primary key column and of a foreign key column.
type DiagramMarks struct {
	Primary string
	Foreign string
}

// DiagramSpanKind is the part of a box one span covers.
type DiagramSpanKind string

// The parts of a box drawn in a colour of their own.
const (
	DiagramSpanPrimary DiagramSpanKind = "primary"
	DiagramSpanForeign DiagramSpanKind = "foreign"
	DiagramSpanType    DiagramSpanKind = "type"
)

// DiagramSpan is the cells one part of a box covers, on one row of the diagram.
type DiagramSpan struct {
	Kind  DiagramSpanKind
	X     int
	Y     int
	Width int
}

// DiagramBox is the table one box draws, and the cells the box covers.
type DiagramBox struct {
	Schema string
	Name   string
	X      int
	Y      int
	Width  int
	Height int
}

// ErDiagram is the drawn diagram: its lines, its boxes left to right and top to bottom, and
// the parts drawn in a colour of their own. Root is the index of the root box.
type ErDiagram struct {
	Lines []string
	Width int
	Boxes []DiagramBox
	Spans []DiagramSpan
	Root  int
}

// QualifyDiagramTable joins the schema and the name of a table into one name.
func QualifyDiagramTable(schema, name string) string {
	return schema + "." + name
}

// diagramCanvas is the cell grid the boxes and the arrows are drawn into.
type diagramCanvas struct {
	rows [][]rune
}

// set writes the text from that cell to the right and replaces the previous content.
func (canvas *diagramCanvas) set(x, y int, text string) {
	for len(canvas.rows) <= y {
		canvas.rows = append(canvas.rows, []rune{})
	}
	row := canvas.rows[y]
	for index, character := range []rune(text) {
		for len(row) <= x+index {
			row = append(row, ' ')
		}
		row[x+index] = character
	}
	canvas.rows[y] = row
}

// setSoft writes into a blank cell only, so it never overwrites a box.
func (canvas *diagramCanvas) setSoft(x, y int, character rune) {
	for len(canvas.rows) <= y {
		canvas.rows = append(canvas.rows, []rune{})
	}
	row := canvas.rows[y]
	for len(row) <= x {
		row = append(row, ' ')
	}
	if row[x] == ' ' {
		row[x] = character
	}
	canvas.rows[y] = row
}

func (canvas *diagramCanvas) toLines() []string {
	width := 0
	for _, row := range canvas.rows {
		if len(row) > width {
			width = len(row)
		}
	}
	lines := make([]string, 0, len(canvas.rows))
	for _, row := range canvas.rows {
		lines = append(lines, PadText(string(row), width))
	}
	return lines
}

// padDiagramCell cuts a cell with an ellipsis, or pads it with spaces.
func padDiagramCell(text string, width int) string {
	if len([]rune(text)) > width {
		return string([]rune(text)[:width-1]) + "…"
	}
	return PadText(text, width)
}

// buildDiagramBox returns the box of one table: the name, and then the columns with the mark
// and the type of each one.
func buildDiagramBox(table DiagramTable, marks DiagramMarks) []string {
	inner := diagramBoxWidth - 2
	shown := listShownDiagramColumns(table)
	lines := []string{
		"╭" + strings.Repeat("─", inner) + "╮",
		"│" + padDiagramCell(" "+
			QualifyDiagramTable(table.Schema, table.Name), inner) + "│",
		"├" + strings.Repeat("─", inner) + "┤",
	}
	for _, column := range shown {
		mark := " "
		switch {
		case column.Primary:
			mark = fitDiagramMark(marks.Primary)
		case column.Foreign:
			mark = fitDiagramMark(marks.Foreign)
		}
		lines = append(lines, "│ "+mark+" "+padDiagramCell(column.Name, diagramNameWidth)+" "+
			padDiagramCell(column.Type, diagramTypeWidth)+" │")
	}
	if len(table.Columns) > len(shown) {
		lines = append(lines, "│"+padDiagramCell(
			" … "+FormatCount(int64(len(table.Columns)-len(shown)))+" more", inner)+"│")
	}
	return append(lines, "╰"+strings.Repeat("─", inner)+"╯")
}

// listShownDiagramColumns returns the columns a box draws.
func listShownDiagramColumns(table DiagramTable) []DiagramColumn {
	if len(table.Columns) > diagramMaxColumns {
		return table.Columns[:diagramMaxColumns]
	}
	return table.Columns
}

// fitDiagramMark returns a glyph one cell wide, or a blank for a glyph of another width.
func fitDiagramMark(glyph string) string {
	if len([]rune(glyph)) != 1 || MeasureText(glyph) != 1 {
		return " "
	}
	return glyph
}

// listDiagramSpans returns the marks and the types of a box drawn at that corner.
func listDiagramSpans(table DiagramTable, x, y int) []DiagramSpan {
	spans := []DiagramSpan{}
	for index, column := range listShownDiagramColumns(table) {
		row := y + diagramHeaderLines + index
		switch {
		case column.Primary:
			spans = append(spans, DiagramSpan{Kind: DiagramSpanPrimary, X: x + 2, Y: row, Width: 1})
		case column.Foreign:
			spans = append(spans, DiagramSpan{Kind: DiagramSpanForeign, X: x + 2, Y: row, Width: 1})
		}
		if column.Type != "" {
			spans = append(spans, DiagramSpan{
				Kind: DiagramSpanType, X: x + 5 + diagramNameWidth, Y: row,
				Width: min(len([]rune(column.Type)), diagramTypeWidth),
			})
		}
	}
	return spans
}

// findDiagramColumnRow returns the row of a box that holds that column.
func findDiagramColumnRow(table DiagramTable, columnName string) (int, bool) {
	for index, column := range listShownDiagramColumns(table) {
		if strings.EqualFold(column.Name, columnName) {
			return diagramHeaderLines + index, true
		}
	}
	return 0, false
}

// diagramPlacement is the position and the height of one box.
type diagramPlacement struct {
	table  DiagramTable
	x      int
	y      int
	height int
}

func placeDiagramBox(
	canvas *diagramCanvas, drawn *ErDiagram, table DiagramTable, marks DiagramMarks, x, y int,
) diagramPlacement {
	lines := buildDiagramBox(table, marks)
	for index, line := range lines {
		canvas.set(x, y+index, line)
	}
	drawn.Boxes = append(drawn.Boxes, DiagramBox{
		Schema: table.Schema, Name: table.Name,
		X: x, Y: y, Width: diagramBoxWidth, Height: len(lines),
	})
	drawn.Spans = append(drawn.Spans, listDiagramSpans(table, x, y)...)
	return diagramPlacement{table: table, x: x, y: y, height: len(lines)}
}

// connectDiagram draws an arrow from one row to another through a vertical channel.
func connectDiagram(canvas *diagramCanvas, fromX, fromY, toX, toY int) {
	step := max((toX-fromX)/2, 2)
	channel := fromX + step

	for x := fromX; x < channel; x++ {
		canvas.setSoft(x, fromY, '─')
	}

	top, bottom := fromY, toY
	if top > bottom {
		top, bottom = toY, fromY
	}
	for y := top; y <= bottom; y++ {
		canvas.setSoft(channel, y, '│')
	}

	switch {
	case fromY == toY:
		canvas.set(channel, fromY, "─")
	case fromY < toY:
		canvas.set(channel, fromY, "╮")
		canvas.set(channel, toY, "╰")
	default:
		canvas.set(channel, fromY, "╯")
		canvas.set(channel, toY, "╭")
	}

	for x := channel + 1; x < toX-1; x++ {
		canvas.setSoft(x, toY, '─')
	}
	canvas.set(toX-1, toY, "▶")
}

// RenderErDiagram draws the table and its neighbours and connects every foreign key column
// to the column it refers to.
func RenderErDiagram(root DiagramTable, related []DiagramTable, marks DiagramMarks) ErDiagram {
	canvas := &diagramCanvas{}
	drawn := ErDiagram{}
	rootName := QualifyDiagramTable(root.Schema, root.Name)

	findRelated := func(schema, name string) (DiagramTable, bool) {
		for _, table := range related {
			if QualifyDiagramTable(table.Schema, table.Name) ==
				QualifyDiagramTable(schema, name) {
				return table, true
			}
		}
		return DiagramTable{}, false
	}

	outgoing := []query.ForeignKey{}
	for _, key := range root.ForeignKeys {
		if _, found := findRelated(key.TargetSchema, key.TargetTable); found {
			outgoing = append(outgoing, key)
		}
	}

	type incomingKey struct {
		table DiagramTable
		key   query.ForeignKey
	}
	incoming := []incomingKey{}
	for _, table := range related {
		for _, key := range table.ForeignKeys {
			if QualifyDiagramTable(key.TargetSchema, key.TargetTable) == rootName {
				incoming = append(incoming, incomingKey{table: table, key: key})
			}
		}
	}

	leftX := 0
	middleX := 0
	if len(incoming) > 0 {
		middleX = diagramBoxWidth + diagramGap
	}
	rightX := middleX + diagramBoxWidth + diagramGap

	leftY := 0
	leftPlacements := make([]diagramPlacement, 0, len(incoming))
	for _, held := range incoming {
		placed := placeDiagramBox(canvas, &drawn, held.table, marks, leftX, leftY)
		leftY += placed.height + 1
		leftPlacements = append(leftPlacements, placed)
	}

	drawn.Root = len(drawn.Boxes)
	rootPlacement := placeDiagramBox(canvas, &drawn, root, marks, middleX, 0)

	rightY := 0
	for _, key := range outgoing {
		target, found := findRelated(key.TargetSchema, key.TargetTable)
		if !found {
			continue
		}
		placed := placeDiagramBox(canvas, &drawn, target, marks, rightX, rightY)
		rightY += placed.height + 1

		fromRow, hasFrom := findDiagramColumnRow(root, firstOf(key.Columns))
		toRow, hasTo := findDiagramColumnRow(placed.table, firstOf(key.TargetColumns))
		if !hasFrom || !hasTo {
			continue
		}
		connectDiagram(canvas,
			middleX+diagramBoxWidth, rootPlacement.y+fromRow, placed.x, placed.y+toRow)
	}

	for index, held := range incoming {
		if index >= len(leftPlacements) {
			continue
		}
		placed := leftPlacements[index]
		fromRow, hasFrom := findDiagramColumnRow(placed.table, firstOf(held.key.Columns))
		toRow, hasTo := findDiagramColumnRow(root, firstOf(held.key.TargetColumns))
		if !hasFrom || !hasTo {
			continue
		}
		connectDiagram(canvas,
			leftX+diagramBoxWidth, placed.y+fromRow, middleX, rootPlacement.y+toRow)
	}

	drawn.Lines = canvas.toLines()
	drawn.Width = measureDiagramWidth(drawn.Lines)
	return drawn
}

func firstOf(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// CollectDiagramNeighbours returns the names of the tables at both ends of a foreign key of
// this table.
func CollectDiagramNeighbours(
	root DiagramTable, relationships []db.Relationship,
) map[string]bool {
	rootName := QualifyDiagramTable(root.Schema, root.Name)
	names := map[string]bool{}
	for _, key := range root.ForeignKeys {
		names[QualifyDiagramTable(key.TargetSchema, key.TargetTable)] = true
	}
	for _, relationship := range relationships {
		if QualifyDiagramTable(
			relationship.TargetSchema, relationship.TargetTable) == rootName {
			names[QualifyDiagramTable(relationship.Schema, relationship.Table)] = true
		}
	}
	return names
}

// The query builder draws its tables with the same boxes and the same connector, with a
// tick before every column and the type after it.

// BuilderColumnBox is one column of a box of the builder diagram.
type BuilderColumnBox struct {
	Name string
	// Kind is the short mark of the type of the column.
	Kind   string
	Picked bool
	// Note is the aggregate or the sort drawn after the name.
	Note string
}

// BuilderBox is one table of the builder diagram.
type BuilderBox struct {
	Title   string
	Columns []BuilderColumnBox
	// Reason stands in place of the columns while the server has not answered.
	Reason string
}

// BuilderLink joins two boxes on one column each.
type BuilderLink struct {
	From       int
	FromColumn string
	To         int
	ToColumn   string
}

// BuilderCell is where one column of the diagram was drawn.
type BuilderCell struct {
	Box    int
	Column int
	X      int
	Y      int
	Width  int
}

// BuilderDiagram is the drawn diagram: its lines, and where each column landed.
type BuilderDiagram struct {
	Lines  []string
	Cells  []BuilderCell
	Titles []BuilderCell
}

// The size of one box of the builder diagram.
const (
	builderBoxWidth = 28
	builderBoxGap   = 6
	// builderHeaderLines are the top border and the title of a box.
	builderHeaderLines = 2
)

// RenderBuilderDiagram draws the tables left to right and connects every join.
func RenderBuilderDiagram(boxes []BuilderBox, links []BuilderLink) BuilderDiagram {
	canvas := &diagramCanvas{}
	drawn := BuilderDiagram{}
	if len(boxes) == 0 {
		return drawn
	}

	for index, box := range boxes {
		x := index * (builderBoxWidth + builderBoxGap)
		for row, line := range buildBuilderBox(box) {
			canvas.set(x, row, line)
		}
		drawn.Titles = append(drawn.Titles, BuilderCell{
			Box: index, Column: -1, X: x + 1, Y: 1, Width: builderBoxWidth - 2,
		})
		for at := range box.Columns {
			drawn.Cells = append(drawn.Cells, BuilderCell{
				Box: index, Column: at, X: x + 1, Y: builderHeaderLines + at,
				Width: builderBoxWidth - 2,
			})
		}
	}

	lane := countDiagramHeight(boxes)
	for _, link := range links {
		left, right := link.From, link.To
		leftColumn, rightColumn := link.FromColumn, link.ToColumn
		if right < left {
			left, right = right, left
			leftColumn, rightColumn = rightColumn, leftColumn
		}
		leftRow, hasLeft := findBuilderColumnRow(boxes, left, leftColumn)
		rightRow, hasRight := findBuilderColumnRow(boxes, right, rightColumn)
		if !hasLeft || !hasRight {
			continue
		}

		fromX := left*(builderBoxWidth+builderBoxGap) + builderBoxWidth
		toX := right * (builderBoxWidth + builderBoxGap)
		if right-left == 1 {
			connectDiagram(canvas, fromX, leftRow, toX, rightRow)
			continue
		}
		connectBuilderLane(canvas, fromX, leftRow, toX, rightRow, lane)
	}

	drawn.Lines = canvas.toLines()
	return drawn
}

// countDiagramHeight returns the row under every box, which is the lane a long connector
// runs along.
func countDiagramHeight(boxes []BuilderBox) int {
	height := 0
	for _, box := range boxes {
		height = max(height, len(buildBuilderBox(box)))
	}
	return height
}

// connectBuilderLane draws a connector between two boxes that do not stand side by side. It
// leaves the left box, runs along the lane under every box, and comes up into the right box.
func connectBuilderLane(canvas *diagramCanvas, fromX, fromY, toX, toY, lane int) {
	leftChannel, rightChannel := fromX+2, toX-3
	for x := fromX; x < leftChannel; x++ {
		canvas.setSoft(x, fromY, '─')
	}
	canvas.set(leftChannel, fromY, "╮")
	for y := fromY + 1; y < lane; y++ {
		canvas.setSoft(leftChannel, y, '│')
	}
	canvas.set(leftChannel, lane, "╰")
	for x := leftChannel + 1; x < rightChannel; x++ {
		canvas.setSoft(x, lane, '─')
	}
	canvas.set(rightChannel, lane, "╯")
	for y := toY + 1; y < lane; y++ {
		canvas.setSoft(rightChannel, y, '│')
	}
	canvas.set(rightChannel, toY, "╭")
	for x := rightChannel + 1; x < toX-1; x++ {
		canvas.setSoft(x, toY, '─')
	}
	canvas.set(toX-1, toY, "▶")
}

// buildBuilderBox returns the lines of one table: the name, then the columns with the tick
// of each one.
func buildBuilderBox(box BuilderBox) []string {
	inner := builderBoxWidth - 2
	lines := []string{
		"╭" + strings.Repeat("─", inner) + "╮",
		"│" + padDiagramCell(" "+box.Title, inner) + "│",
	}
	if box.Reason != "" {
		lines = append(lines, "│"+padDiagramCell(" "+box.Reason, inner)+"│")
	}
	for _, column := range box.Columns {
		lines = append(lines, "│"+padDiagramCell(buildBuilderColumn(column, inner), inner)+"│")
	}
	return append(lines, "╰"+strings.Repeat("─", inner)+"╯")
}

// buildBuilderColumn writes one column row: the tick, the name, the note and the type.
func buildBuilderColumn(column BuilderColumnBox, inner int) string {
	tick := "[ ] "
	if column.Picked {
		tick = "[x] "
	}
	right := column.Kind
	if column.Note != "" {
		right = column.Note + " " + column.Kind
	}
	room := inner - len(tick) - len([]rune(right)) - 2
	if room < 1 {
		room = 1
	}
	return " " + tick + padDiagramCell(column.Name, room) + " " + right
}

// findBuilderColumnRow returns the row of a box that holds that column.
func findBuilderColumnRow(boxes []BuilderBox, box int, column string) (int, bool) {
	if box < 0 || box >= len(boxes) {
		return 0, false
	}
	offset := builderHeaderLines
	if boxes[box].Reason != "" {
		offset++
	}
	for at, held := range boxes[box].Columns {
		if strings.EqualFold(held.Name, column) {
			return offset + at, true
		}
	}
	return 1, true
}

// ScrollBuilderDiagram returns the diagram windowed to the width of the pane, drawn from
// that column. Every cell it reports moves with the lines.
func ScrollBuilderDiagram(drawn BuilderDiagram, offset, width int) BuilderDiagram {
	offset = max(offset, 0)
	if offset == 0 && measureDiagramWidth(drawn.Lines) <= width {
		return drawn
	}

	held := BuilderDiagram{}
	for _, line := range drawn.Lines {
		runes := []rune(line)
		if offset >= len(runes) {
			held.Lines = append(held.Lines, "")
			continue
		}
		held.Lines = append(held.Lines, string(runes[offset:]))
	}
	held.Cells = moveBuilderCells(drawn.Cells, offset, width)
	held.Titles = moveBuilderCells(drawn.Titles, offset, width)
	return held
}

// FindBuilderColumnOffset returns the column the diagram is drawn from, so the box of that
// index stands whole inside the width of the pane. A diagram that fits is drawn from its
// first column.
func FindBuilderColumnOffset(offset, box, boxes, width int) int {
	if width <= 0 || boxes <= 0 {
		return 0
	}
	widest := boxes*(builderBoxWidth+builderBoxGap) - builderBoxGap
	offset = min(max(offset, 0), max(widest-width, 0))
	// A diagram the wheel moved follows no cursor until the cursor moves again.
	if box < 0 {
		return offset
	}

	left := box * (builderBoxWidth + builderBoxGap)
	if left < offset {
		return left
	}
	if right := left + builderBoxWidth; right > offset+width {
		return right - width
	}
	return offset
}

// moveBuilderCells returns the cells the window draws whole, at the columns they moved to.
// A cell the window cuts is left out, so a press never lands on half a column.
func moveBuilderCells(cells []BuilderCell, offset, width int) []BuilderCell {
	moved := make([]BuilderCell, 0, len(cells))
	for _, cell := range cells {
		cell.X -= offset
		if cell.X < 0 || cell.X+cell.Width > width {
			continue
		}
		moved = append(moved, cell)
	}
	return moved
}

// measureDiagramWidth returns the columns the widest line of a diagram takes.
func measureDiagramWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		width = max(width, len([]rune(line)))
	}
	return width
}
