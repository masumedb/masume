package present

import (
	"slices"
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

// The narrowest gap between two lanes of boxes, the rows over the first column of a box, the
// columns a box of a related table shows, and the narrowest box.
const (
	diagramGapLeast    = 8
	diagramHeaderLines = 3
	diagramMaxColumns  = 10
	diagramBoxLeast    = 24
	// diagramRowChrome is the border, the mark and the blanks around the name and the type.
	diagramRowChrome = 7
)

// diagramSizes are the widths every box of one lane shares: the box, the name column and
// the type column.
type diagramSizes struct {
	box, name, kind int
}

// DiagramMarks are the glyphs of a primary key column and of a foreign key column.
type DiagramMarks struct {
	Primary string
	Foreign string
}

// DiagramSpanKind is the part of a box one span covers.
type DiagramSpanKind string

// The parts of a box drawn in a colour of their own.
const (
	DiagramSpanTitle   DiagramSpanKind = "title"
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

// DiagramCell is one cell of the diagram.
type DiagramCell struct {
	X int
	Y int
}

// DiagramLink is one foreign key of the diagram: the box of the referencing column, the box
// of the referenced column, and the cells of its line.
type DiagramLink struct {
	From  int
	To    int
	Cells []DiagramCell
}

// ErDiagram is the drawn diagram: its lines, its boxes left to right and top to bottom, the
// parts drawn in a colour of their own, and the foreign keys. Root is the index of the root
// box.
type ErDiagram struct {
	Lines []string
	Width int
	Boxes []DiagramBox
	Spans []DiagramSpan
	Links []DiagramLink
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

// diagramNode is one table of the diagram, with the columns its box shows and the corner of
// the box.
type diagramNode struct {
	table DiagramTable
	shown []DiagramColumn
	sizes diagramSizes
	x, y  int
	box   int
}

// countHeight returns the rows the box takes.
func (node *diagramNode) countHeight() int {
	height := diagramHeaderLines + len(node.shown) + 1
	if len(node.table.Columns) > len(node.shown) {
		height++
	}
	return height
}

// findColumnOffset returns the row of the box that shows that column, counted from the top
// border.
func (node *diagramNode) findColumnOffset(name string) (int, bool) {
	for index, column := range node.shown {
		if strings.EqualFold(column.Name, name) {
			return diagramHeaderLines + index, true
		}
	}
	return 0, false
}

// findColumnRow returns the row of the diagram that shows that column.
func (node *diagramNode) findColumnRow(name string) (int, bool) {
	offset, found := node.findColumnOffset(name)
	return node.y + offset, found
}

// listShownDiagramColumns returns the columns a box draws: the first ones, and every column
// in keep.
func listShownDiagramColumns(table DiagramTable, keep map[string]bool) []DiagramColumn {
	shown := []DiagramColumn{}
	for index, column := range table.Columns {
		if index < diagramMaxColumns || keep[strings.ToLower(column.Name)] {
			shown = append(shown, column)
		}
	}
	return shown
}

// measureDiagramSizes returns the widths that hold the longest title, name and type of the
// boxes.
func measureDiagramSizes(nodes []*diagramNode) diagramSizes {
	title, name, kind := 0, 0, 0
	for _, node := range nodes {
		title = max(title, len([]rune(QualifyDiagramTable(node.table.Schema, node.table.Name))))
		for _, column := range node.shown {
			name = max(name, len([]rune(column.Name)))
			kind = max(kind, len([]rune(column.Type)))
		}
	}
	box := max(name+kind+diagramRowChrome, title+3, diagramBoxLeast)
	return diagramSizes{box: box, name: box - diagramRowChrome - kind, kind: kind}
}

// buildDiagramBox returns the box of one table: the name, and then the columns with the mark
// and the type of each one.
func buildDiagramBox(node *diagramNode, marks DiagramMarks) []string {
	sizes := node.sizes
	inner := sizes.box - 2
	lines := []string{
		"╭" + strings.Repeat("─", inner) + "╮",
		"│" + padDiagramCell(" "+
			QualifyDiagramTable(node.table.Schema, node.table.Name), inner) + "│",
		"├" + strings.Repeat("─", inner) + "┤",
	}
	for _, column := range node.shown {
		mark := " "
		switch {
		case column.Primary:
			mark = fitDiagramMark(marks.Primary)
		case column.Foreign:
			mark = fitDiagramMark(marks.Foreign)
		}
		lines = append(lines, "│ "+mark+" "+padDiagramCell(column.Name, sizes.name)+" "+
			padDiagramCell(column.Type, sizes.kind)+" │")
	}
	if hidden := len(node.table.Columns) - len(node.shown); hidden > 0 {
		lines = append(lines, "│"+padDiagramCell(
			" … "+FormatCount(int64(hidden))+" more", inner)+"│")
	}
	return append(lines, "╰"+strings.Repeat("─", inner)+"╯")
}

// fitDiagramMark returns a glyph one cell wide, or a blank for a glyph of another width.
func fitDiagramMark(glyph string) string {
	if len([]rune(glyph)) != 1 || MeasureText(glyph) != 1 {
		return " "
	}
	return glyph
}

// listDiagramSpans returns the title, the marks and the types of a box.
func listDiagramSpans(node *diagramNode) []DiagramSpan {
	x, y, sizes := node.x, node.y, node.sizes
	title := len([]rune(QualifyDiagramTable(node.table.Schema, node.table.Name)))
	spans := []DiagramSpan{{
		Kind: DiagramSpanTitle, X: x + 2, Y: y + 1, Width: min(title, sizes.box-3),
	}}
	for index, column := range node.shown {
		row := y + diagramHeaderLines + index
		switch {
		case column.Primary:
			spans = append(spans, DiagramSpan{Kind: DiagramSpanPrimary, X: x + 2, Y: row, Width: 1})
		case column.Foreign:
			spans = append(spans, DiagramSpan{Kind: DiagramSpanForeign, X: x + 2, Y: row, Width: 1})
		}
		if column.Type != "" {
			spans = append(spans, DiagramSpan{
				Kind: DiagramSpanType, X: x + 5 + sizes.name, Y: row,
				Width: min(len([]rune(column.Type)), sizes.kind),
			})
		}
	}
	return spans
}

// diagramKey is one foreign key the diagram draws, from the referencing column to the
// referenced one.
type diagramKey struct {
	from, to             *diagramNode
	fromColumn, toColumn string
}

// RenderErDiagram draws the table and its neighbours and connects every foreign key column
// to the column it refers to. A table the root refers to is on the right, and every other
// table is on the left.
func RenderErDiagram(root DiagramTable, related []DiagramTable, marks DiagramMarks) ErDiagram {
	rootName := QualifyDiagramTable(root.Schema, root.Name)
	rootNode := &diagramNode{table: root}
	nodes := map[string]*diagramNode{rootName: rootNode}
	neighbours := []*diagramNode{}
	for _, table := range related {
		name := QualifyDiagramTable(table.Schema, table.Name)
		if _, seen := nodes[name]; seen {
			continue
		}
		node := &diagramNode{table: table}
		nodes[name] = node
		neighbours = append(neighbours, node)
	}

	keys := []diagramKey{}
	for _, key := range root.ForeignKeys {
		target, found := nodes[QualifyDiagramTable(key.TargetSchema, key.TargetTable)]
		if found {
			keys = append(keys, diagramKey{
				from: rootNode, to: target,
				fromColumn: firstOf(key.Columns), toColumn: firstOf(key.TargetColumns),
			})
		}
	}
	for _, node := range neighbours {
		for _, key := range node.table.ForeignKeys {
			if QualifyDiagramTable(key.TargetSchema, key.TargetTable) == rootName {
				keys = append(keys, diagramKey{
					from: node, to: rootNode,
					fromColumn: firstOf(key.Columns), toColumn: firstOf(key.TargetColumns),
				})
			}
		}
	}

	keep := map[*diagramNode]map[string]bool{}
	keepColumn := func(node *diagramNode, column string) {
		if keep[node] == nil {
			keep[node] = map[string]bool{}
		}
		keep[node][strings.ToLower(column)] = true
	}
	for _, key := range keys {
		keepColumn(key.from, key.fromColumn)
		keepColumn(key.to, key.toColumn)
	}
	rootNode.shown = root.Columns
	for _, node := range neighbours {
		node.shown = listShownDiagramColumns(node.table, keep[node])
	}

	left, right := splitDiagramLanes(rootNode, neighbours, keys)
	rootNode.sizes = measureDiagramSizes([]*diagramNode{rootNode})
	for _, lane := range [][]*diagramNode{left, right} {
		sizes := measureDiagramSizes(lane)
		for _, node := range lane {
			node.sizes = sizes
		}
	}
	placeDiagramLane(rootNode, left, keys)
	placeDiagramLane(rootNode, right, keys)

	drawn := ErDiagram{}
	boxes := append(append(append([]*diagramNode{}, left...), rootNode), right...)
	for index, node := range boxes {
		node.box = index
	}
	drawn.Root = rootNode.box

	leftLinks, rightLinks := []gapLink{}, []gapLink{}
	for index, key := range keys {
		fromRow, hasFrom := key.from.findColumnRow(key.fromColumn)
		toRow, hasTo := key.to.findColumnRow(key.toColumn)
		drawn.Links = append(drawn.Links, DiagramLink{From: key.from.box, To: key.to.box})
		if !hasFrom || !hasTo {
			continue
		}
		link := gapLink{link: index, fromRow: fromRow, toRow: toRow}
		switch {
		case key.from == rootNode && key.to == rootNode:
			if fromRow == toRow {
				continue
			}
			link.fromLeft, link.toLeft = true, true
			rightLinks = append(rightLinks, link)
		case key.from == rootNode:
			link.fromLeft = true
			rightLinks = append(rightLinks, link)
		case slices.Contains(right, key.from):
			link.toLeft = true
			rightLinks = append(rightLinks, link)
		default:
			link.fromLeft = true
			leftLinks = append(leftLinks, link)
		}
	}

	leftNets, rightNets := groupGapNets(leftLinks), groupGapNets(rightLinks)
	if len(left) > 0 {
		rootNode.x = left[0].sizes.box + measureGap(leftNets)
	}
	rightX := rootNode.x + rootNode.sizes.box + measureGap(rightNets)
	for _, node := range right {
		node.x = rightX
	}

	canvas := &diagramCanvas{}
	for _, node := range boxes {
		for index, line := range buildDiagramBox(node, marks) {
			canvas.set(node.x, node.y+index, line)
		}
		drawn.Boxes = append(drawn.Boxes, DiagramBox{
			Schema: node.table.Schema, Name: node.table.Name,
			X: node.x, Y: node.y, Width: node.sizes.box, Height: node.countHeight(),
		})
		drawn.Spans = append(drawn.Spans, listDiagramSpans(node)...)
	}

	strokes := &diagramStrokes{masks: map[DiagramCell]uint8{}, arrows: map[DiagramCell]rune{}}
	if len(left) > 0 {
		routeGap(strokes, drawn.Links, leftNets, left[0].sizes.box, rootNode.x-1)
	}
	rightEdge := rootNode.x + rootNode.sizes.box
	routeGap(strokes, drawn.Links, rightNets, rightEdge, rightX-1)
	strokes.drawInto(canvas)

	drawn.Lines = canvas.toLines()
	drawn.Width = measureDiagramWidth(drawn.Lines)
	return drawn
}

// splitDiagramLanes returns the tables on the left and on the right of the root. A table the
// root refers to is on the right. Each lane is ordered by the first root row it links to.
func splitDiagramLanes(
	root *diagramNode, neighbours []*diagramNode, keys []diagramKey,
) (left, right []*diagramNode) {
	for _, node := range neighbours {
		referred := slices.ContainsFunc(keys, func(key diagramKey) bool {
			return key.from == root && key.to == node
		})
		if referred {
			right = append(right, node)
			continue
		}
		left = append(left, node)
	}
	for _, lane := range [][]*diagramNode{left, right} {
		slices.SortStableFunc(lane, func(first, second *diagramNode) int {
			return findRootAnchor(root, first, keys) - findRootAnchor(root, second, keys)
		})
	}
	return left, right
}

// findRootAnchor returns the first root column a key between the root and that table joins.
func findRootAnchor(root, node *diagramNode, keys []diagramKey) int {
	anchor := len(root.table.Columns)
	for _, key := range keys {
		switch {
		case key.from == root && key.to == node:
			anchor = min(anchor, findColumnIndex(root.table.Columns, key.fromColumn))
		case key.from == node && key.to == root:
			anchor = min(anchor, findColumnIndex(root.table.Columns, key.toColumn))
		}
	}
	return anchor
}

// findColumnIndex returns the position of the column, or the column count where it is absent.
func findColumnIndex(columns []DiagramColumn, name string) int {
	for index, column := range columns {
		if strings.EqualFold(column.Name, name) {
			return index
		}
	}
	return len(columns)
}

// placeDiagramLane stacks the boxes of one lane from the top. Each box moves down to put its
// linked column on the row of the root column it links to, when the boxes above leave room.
func placeDiagramLane(root *diagramNode, lane []*diagramNode, keys []diagramKey) {
	next := 0
	for _, node := range lane {
		node.y = next
		for _, key := range keys {
			rootColumn, nodeColumn := "", ""
			switch {
			case key.from == root && key.to == node:
				rootColumn, nodeColumn = key.fromColumn, key.toColumn
			case key.from == node && key.to == root:
				rootColumn, nodeColumn = key.toColumn, key.fromColumn
			default:
				continue
			}
			rootRow, hasRoot := root.findColumnRow(rootColumn)
			offset, hasNode := node.findColumnOffset(nodeColumn)
			if hasRoot && hasNode {
				node.y = max(next, rootRow-offset)
			}
			break
		}
		next = node.y + node.countHeight() + 1
	}
}

func firstOf(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// gapLink is one foreign key line through the gap between two lanes. Each end stands on the
// left or on the right edge of the gap, and the arrow is at the referenced end.
type gapLink struct {
	link             int
	fromRow, toRow   int
	fromLeft, toLeft bool
}

// gapNet is the lines that end at one arrow. They share one channel of the gap.
type gapNet struct {
	links               []gapLink
	leftRows, rightRows []int
	top, bottom         int
}

// groupGapNets returns the links grouped by the arrow they end at.
func groupGapNets(links []gapLink) []*gapNet {
	type arrow struct {
		left bool
		row  int
	}
	found := map[arrow]*gapNet{}
	nets := []*gapNet{}
	for _, link := range links {
		key := arrow{left: link.toLeft, row: link.toRow}
		net := found[key]
		if net == nil {
			net = &gapNet{top: link.toRow, bottom: link.toRow}
			found[key] = net
			nets = append(nets, net)
		}
		net.links = append(net.links, link)
		for _, end := range []struct {
			row  int
			left bool
		}{{link.fromRow, link.fromLeft}, {link.toRow, link.toLeft}} {
			if end.left {
				net.leftRows = append(net.leftRows, end.row)
			} else {
				net.rightRows = append(net.rightRows, end.row)
			}
			net.top, net.bottom = min(net.top, end.row), max(net.bottom, end.row)
		}
	}
	orderGapNets(nets)
	return nets
}

// orderGapNets orders the channels from the left edge of the gap outwards, with the fewest
// lines crossing.
func orderGapNets(nets []*gapNet) {
	slices.SortStableFunc(nets, func(first, second *gapNet) int {
		if first.top != second.top {
			return first.top - second.top
		}
		return first.bottom - second.bottom
	})
	for swapped := true; swapped; {
		swapped = false
		for at := 0; at+1 < len(nets); at++ {
			if countGapCrossings(nets[at+1], nets[at]) < countGapCrossings(nets[at], nets[at+1]) {
				nets[at], nets[at+1] = nets[at+1], nets[at]
				swapped = true
			}
		}
	}
}

// countGapCrossings returns how often the lines of two nets cross, with inner on the channel
// nearer the left edge.
func countGapCrossings(inner, outer *gapNet) int {
	crossings := 0
	for _, row := range outer.leftRows {
		if row >= inner.top && row <= inner.bottom {
			crossings++
		}
	}
	for _, row := range inner.rightRows {
		if row >= outer.top && row <= outer.bottom {
			crossings++
		}
	}
	return crossings
}

// measureGap returns the columns between two lanes: a lead, one channel per net with a blank
// column between two channels, and the arrow.
func measureGap(nets []*gapNet) int {
	return max(diagramGapLeast, 2*len(nets)+3)
}

// The four arms a line cell joins.
const (
	lineUp uint8 = 1 << iota
	lineDown
	lineLeft
	lineRight
)

// diagramStrokes is the lines of a diagram: the arms each cell joins, and the arrowheads.
type diagramStrokes struct {
	masks  map[DiagramCell]uint8
	arrows map[DiagramCell]rune
}

// joinCell adds arms to one cell and records the cell on the link.
func (strokes *diagramStrokes) joinCell(link *DiagramLink, x, y int, mask uint8) {
	cell := DiagramCell{X: x, Y: y}
	strokes.masks[cell] |= mask
	if !slices.Contains(link.Cells, cell) {
		link.Cells = append(link.Cells, cell)
	}
}

// runRow draws a line along one row from the edge of the gap to the cell before the channel.
func (strokes *diagramStrokes) runRow(link *DiagramLink, y, edge, channel int) {
	for x := min(edge, channel); x <= max(edge, channel); x++ {
		if x != channel {
			strokes.joinCell(link, x, y, lineLeft|lineRight)
		}
	}
}

// runColumn draws a line along one column between two rows, both ends excluded.
func (strokes *diagramStrokes) runColumn(link *DiagramLink, x, from, to int) {
	for y := min(from, to) + 1; y < max(from, to); y++ {
		strokes.joinCell(link, x, y, lineUp|lineDown)
	}
}

// routeGap draws the nets of one gap. Each net takes its own channel, and each line runs from
// its edge to the channel, along the channel, and on to its other edge.
func routeGap(strokes *diagramStrokes, links []DiagramLink, nets []*gapNet, leftEdge, rightEdge int) {
	for index, net := range nets {
		channel := leftEdge + 2 + 2*index
		for _, held := range net.links {
			link := &links[held.link]
			for _, end := range []struct {
				row  int
				left bool
			}{{held.fromRow, held.fromLeft}, {held.toRow, held.toLeft}} {
				edge, arm := rightEdge, lineRight
				if end.left {
					edge, arm = leftEdge, lineLeft
				}
				strokes.runRow(link, end.row, edge, channel)
				strokes.joinCell(link, channel, end.row, arm)
			}
			if held.fromRow != held.toRow {
				strokes.runColumn(link, channel, held.fromRow, held.toRow)
				top, bottom := min(held.fromRow, held.toRow), max(held.fromRow, held.toRow)
				strokes.joinCell(link, channel, top, lineDown)
				strokes.joinCell(link, channel, bottom, lineUp)
			}
			if held.toLeft {
				strokes.arrows[DiagramCell{X: leftEdge, Y: held.toRow}] = '◀'
				continue
			}
			strokes.arrows[DiagramCell{X: rightEdge, Y: held.toRow}] = '▶'
		}
	}
}

// drawInto writes every line cell and every arrowhead into the canvas.
func (strokes *diagramStrokes) drawInto(canvas *diagramCanvas) {
	for cell, mask := range strokes.masks {
		canvas.set(cell.X, cell.Y, string(pickLineGlyph(mask)))
	}
	for cell, arrow := range strokes.arrows {
		canvas.set(cell.X, cell.Y, string(arrow))
	}
}

// pickLineGlyph returns the box character that joins these arms.
func pickLineGlyph(mask uint8) rune {
	switch mask {
	case lineLeft, lineRight, lineLeft | lineRight:
		return '─'
	case lineUp, lineDown, lineUp | lineDown:
		return '│'
	case lineDown | lineRight:
		return '╭'
	case lineDown | lineLeft:
		return '╮'
	case lineUp | lineRight:
		return '╰'
	case lineUp | lineLeft:
		return '╯'
	case lineUp | lineDown | lineRight:
		return '├'
	case lineUp | lineDown | lineLeft:
		return '┤'
	case lineLeft | lineRight | lineDown:
		return '┬'
	case lineLeft | lineRight | lineUp:
		return '┴'
	}
	return '┼'
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
	// BoxWidth is the width every box of the diagram shares.
	BoxWidth int
}

// The narrowest box of the builder diagram, and the space between two boxes.
const (
	builderBoxWidth = 28
	builderBoxGap   = 6
	// builderHeaderLines are the top border and the title of a box.
	builderHeaderLines = 2
)

// RenderBuilderDiagram draws the tables left to right and connects every join.
func RenderBuilderDiagram(boxes []BuilderBox, links []BuilderLink) BuilderDiagram {
	canvas := &diagramCanvas{}
	boxWidth := MeasureBuilderBoxWidth(boxes)
	drawn := BuilderDiagram{BoxWidth: boxWidth}
	if len(boxes) == 0 {
		return drawn
	}

	for index, box := range boxes {
		x := index * (boxWidth + builderBoxGap)
		for row, line := range buildBuilderBox(box, boxWidth-2) {
			canvas.set(x, row, line)
		}
		drawn.Titles = append(drawn.Titles, BuilderCell{
			Box: index, Column: -1, X: x + 1, Y: 1, Width: boxWidth - 2,
		})
		for at := range box.Columns {
			drawn.Cells = append(drawn.Cells, BuilderCell{
				Box: index, Column: at, X: x + 1, Y: builderHeaderLines + at,
				Width: boxWidth - 2,
			})
		}
	}

	lane := countDiagramHeight(boxes, boxWidth-2)
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

		fromX := left*(boxWidth+builderBoxGap) + boxWidth
		toX := right * (boxWidth + builderBoxGap)
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
func countDiagramHeight(boxes []BuilderBox, inner int) int {
	height := 0
	for _, box := range boxes {
		height = max(height, len(buildBuilderBox(box, inner)))
	}
	return height
}

// MeasureBuilderBoxWidth returns the width every box of the diagram shares: the one that
// holds the longest title and the longest column row, and never less than builderBoxWidth.
func MeasureBuilderBoxWidth(boxes []BuilderBox) int {
	inner := builderBoxWidth - 2
	for _, box := range boxes {
		inner = max(inner, len([]rune(box.Title))+2, len([]rune(box.Reason))+2)
		for _, column := range box.Columns {
			inner = max(inner, len(builderTick)+len([]rune(column.Name))+
				len([]rune(describeBuilderColumnRight(column)))+builderColumnChrome)
		}
	}
	return inner + 2
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
func buildBuilderBox(box BuilderBox, inner int) []string {
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
	tick := builderTick
	if column.Picked {
		tick = "[x] "
	}
	right := describeBuilderColumnRight(column)
	room := inner - len(tick) - len([]rune(right)) - builderColumnChrome
	if room < 1 {
		room = 1
	}
	return " " + tick + padDiagramCell(column.Name, room) + " " + right + " "
}

// The tick of a column that is not picked, and the blanks around the name and the type.
const (
	builderTick         = "[ ] "
	builderColumnChrome = 3
)

// describeBuilderColumnRight returns what a column row writes after the name: the note and
// the type.
func describeBuilderColumnRight(column BuilderColumnBox) string {
	if column.Note != "" {
		return column.Note + " " + column.Kind
	}
	return column.Kind
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

	held := BuilderDiagram{BoxWidth: drawn.BoxWidth}
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
func FindBuilderColumnOffset(offset, box, boxes, boxWidth, width int) int {
	if width <= 0 || boxes <= 0 {
		return 0
	}
	widest := boxes*(boxWidth+builderBoxGap) - builderBoxGap
	offset = min(max(offset, 0), max(widest-width, 0))
	// A diagram the wheel moved follows no cursor until the cursor moves again.
	if box < 0 {
		return offset
	}

	left := box * (boxWidth + builderBoxGap)
	if left < offset {
		return left
	}
	if right := left + boxWidth; right > offset+width {
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
