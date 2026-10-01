package present

import (
	"slices"
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/query"
)

func TestRenderErDiagram(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		Columns: []DiagramColumn{
			{Name: "AlbumId", Type: "int", Primary: true},
			{Name: "Title", Type: "text"},
			{Name: "ArtistId", Type: "int", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{{
			Name: "fk", Columns: []string{"ArtistId"},
			TargetSchema: "main", TargetTable: "Artist",
			TargetColumns: []string{"ArtistId"},
		}},
	}
	related := []DiagramTable{{
		Schema: "main", Name: "Artist",
		Columns: []DiagramColumn{{Name: "ArtistId", Primary: true}, {Name: "Name"}},
	}}

	drawn := RenderErDiagram(root, related, DiagramMarks{Primary: "◆", Foreign: "→"})
	lines := drawn.Lines
	text := strings.Join(lines, "\n")
	for _, wanted := range []string{"main.Album", "main.Artist", "◆ AlbumId", "→ ArtistId", "▶"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the diagram has no %q:\n%s", wanted, text)
		}
	}
	width := MeasureText(lines[0])
	for at, line := range lines {
		if MeasureText(line) != width {
			t.Errorf("row %d is %d wide, wanted %d", at, MeasureText(line), width)
		}
	}
	if drawn.Width != width {
		t.Errorf("the diagram is %d wide, wanted %d", drawn.Width, width)
	}
}

// Every box and every span covers the cells of its own text.
func TestRenderErDiagramReportsTheBoxesAndTheSpans(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		Columns: []DiagramColumn{
			{Name: "AlbumId", Type: "int", Primary: true},
			{Name: "ArtistId", Type: "bigint", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{{
			Columns: []string{"ArtistId"}, TargetSchema: "main", TargetTable: "Artist",
			TargetColumns: []string{"ArtistId"},
		}},
	}
	related := []DiagramTable{{
		Schema: "main", Name: "Artist",
		Columns: []DiagramColumn{{Name: "ArtistId", Type: "int", Primary: true}},
	}}

	drawn := RenderErDiagram(root, related, DiagramMarks{Primary: "*", Foreign: ">"})
	if len(drawn.Boxes) != 2 || drawn.Boxes[drawn.Root].Name != "Album" {
		t.Fatalf("the diagram reports the boxes %+v and the root %d", drawn.Boxes, drawn.Root)
	}
	for _, box := range drawn.Boxes {
		top := []rune(drawn.Lines[box.Y])
		if top[box.X] != '╭' || top[box.X+box.Width-1] != '╮' {
			t.Errorf("the box %+v does not start at its corner:\n%s", box, string(top))
		}
		if bottom := []rune(drawn.Lines[box.Y+box.Height-1]); bottom[box.X] != '╰' {
			t.Errorf("the box %+v does not end at its corner", box)
		}
	}
	wanted := map[DiagramSpanKind][]string{
		DiagramSpanPrimary: {"*", "*"}, DiagramSpanForeign: {">"},
		DiagramSpanType: {"int", "bigint", "int"},
	}
	found := map[DiagramSpanKind][]string{}
	for _, span := range drawn.Spans {
		line := []rune(drawn.Lines[span.Y])
		found[span.Kind] = append(found[span.Kind], string(line[span.X:span.X+span.Width]))
	}
	for kind, texts := range wanted {
		if strings.Join(found[kind], ",") != strings.Join(texts, ",") {
			t.Errorf("the %s spans cover %q, wanted %q", kind, found[kind], texts)
		}
	}
}

// A glyph wider than one cell, or no glyph at all, leaves the mark blank.
func TestRenderErDiagramBlanksAMarkOfAnotherWidth(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		Columns: []DiagramColumn{{Name: "AlbumId", Primary: true}},
	}
	drawn := RenderErDiagram(root, nil, DiagramMarks{Primary: "PK"})
	if !strings.Contains(strings.Join(drawn.Lines, "\n"), "│   AlbumId") {
		t.Errorf("the mark is not blank:\n%s", strings.Join(drawn.Lines, "\n"))
	}
}

// buildDiagramKey returns a foreign key of one column to the id of a table of main.
func buildDiagramKey(column, target string) query.ForeignKey {
	return query.ForeignKey{
		Columns: []string{column}, TargetSchema: "main", TargetTable: target,
		TargetColumns: []string{"id"},
	}
}

// Two keys to one table draw one box, and their lines join into one arrow.
func TestRenderErDiagramDrawsATableReferencedTwiceOnce(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "orders",
		Columns: []DiagramColumn{
			{Name: "id", Primary: true},
			{Name: "billing_id", Foreign: true},
			{Name: "shipping_id", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{
			buildDiagramKey("billing_id", "addresses"), buildDiagramKey("shipping_id", "addresses"),
		},
	}
	related := []DiagramTable{{
		Schema: "main", Name: "addresses", Columns: []DiagramColumn{{Name: "id", Primary: true}},
	}}

	drawn := RenderErDiagram(root, related, DiagramMarks{})
	text := strings.Join(drawn.Lines, "\n")
	if len(drawn.Boxes) != 2 || strings.Count(text, "main.addresses") != 1 {
		t.Errorf("the diagram draws %d boxes:\n%s", len(drawn.Boxes), text)
	}
	if len(drawn.Links) != 2 || strings.Count(text, "▶") != 1 {
		t.Errorf("the diagram draws %d links and %d arrows:\n%s",
			len(drawn.Links), strings.Count(text, "▶"), text)
	}
}

// A key of a table to itself draws a loop on the right of the box, back to the box.
func TestRenderErDiagramDrawsASelfReference(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "employees",
		Columns: []DiagramColumn{
			{Name: "id", Primary: true}, {Name: "name"}, {Name: "manager_id", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{buildDiagramKey("manager_id", "employees")},
	}

	drawn := RenderErDiagram(root, nil, DiagramMarks{})
	text := strings.Join(drawn.Lines, "\n")
	if len(drawn.Boxes) != 1 || len(drawn.Links) != 1 {
		t.Fatalf("the diagram draws %d boxes and %d links", len(drawn.Boxes), len(drawn.Links))
	}
	box := drawn.Boxes[0]
	arrow := []rune(drawn.Lines[box.Y+diagramHeaderLines])[box.X+box.Width]
	if arrow != '◀' {
		t.Errorf("the id row ends in %q, wanted the arrow back into the box:\n%s", arrow, text)
	}
	loop := []rune(drawn.Lines[box.Y+diagramHeaderLines+2])[box.X+box.Width]
	if loop != '─' {
		t.Errorf("the manager_id row ends in %q, wanted the line of the loop:\n%s", loop, text)
	}
}

// A related table shows the column its key joins, even past the columns a box shows.
func TestRenderErDiagramShowsTheJoinedColumnOfAWideTable(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "customers", Columns: []DiagramColumn{{Name: "id", Primary: true}},
	}
	columns := []DiagramColumn{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		columns = append(columns, DiagramColumn{Name: name})
	}
	columns = append(columns, DiagramColumn{Name: "customer_id", Foreign: true})
	related := []DiagramTable{{
		Schema: "main", Name: "orders", Columns: columns,
		ForeignKeys: []query.ForeignKey{buildDiagramKey("customer_id", "customers")},
	}}

	drawn := RenderErDiagram(root, related, DiagramMarks{})
	text := strings.Join(drawn.Lines, "\n")
	for _, wanted := range []string{"customer_id", "… 2 more", "▶"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the diagram has no %q:\n%s", wanted, text)
		}
	}
	if len(drawn.Links) != 1 || len(drawn.Links[0].Cells) == 0 {
		t.Errorf("the key has no line: %+v", drawn.Links)
	}
}

// Keys to two tables run in channels of their own, so no two lines share a cell.
func TestRenderErDiagramKeepsTheLinesApart(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "orders",
		Columns: []DiagramColumn{
			{Name: "id", Primary: true}, {Name: "note"},
			{Name: "customer_id", Foreign: true}, {Name: "shop_id", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{
			buildDiagramKey("customer_id", "customers"), buildDiagramKey("shop_id", "shops"),
		},
	}
	related := []DiagramTable{
		{Schema: "main", Name: "customers", Columns: []DiagramColumn{{Name: "id", Primary: true}}},
		{Schema: "main", Name: "shops", Columns: []DiagramColumn{{Name: "id", Primary: true}}},
	}

	drawn := RenderErDiagram(root, related, DiagramMarks{})
	if len(drawn.Links) != 2 {
		t.Fatalf("the diagram draws %d links", len(drawn.Links))
	}
	for _, cell := range drawn.Links[0].Cells {
		if slices.Contains(drawn.Links[1].Cells, cell) {
			t.Errorf("both lines cross %+v:\n%s", cell, strings.Join(drawn.Lines, "\n"))
		}
	}
}

// A table that refers to the root and is referred to by it stands once, on the right.
func TestRenderErDiagramDrawsATableOfBothSidesOnce(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "customers",
		Columns: []DiagramColumn{
			{Name: "id", Primary: true}, {Name: "last_order_id", Foreign: true},
		},
		ForeignKeys: []query.ForeignKey{buildDiagramKey("last_order_id", "orders")},
	}
	related := []DiagramTable{{
		Schema: "main", Name: "orders",
		Columns:     []DiagramColumn{{Name: "id", Primary: true}, {Name: "customer_id", Foreign: true}},
		ForeignKeys: []query.ForeignKey{buildDiagramKey("customer_id", "customers")},
	}}

	drawn := RenderErDiagram(root, related, DiagramMarks{})
	text := strings.Join(drawn.Lines, "\n")
	if len(drawn.Boxes) != 2 || drawn.Root != 0 {
		t.Fatalf("the diagram draws the boxes %+v with the root %d", drawn.Boxes, drawn.Root)
	}
	if !strings.Contains(text, "▶") || !strings.Contains(text, "◀") {
		t.Errorf("the diagram has no arrow each way:\n%s", text)
	}
}

func TestCollectDiagramNeighbours(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		ForeignKeys: []query.ForeignKey{
			{TargetSchema: "main", TargetTable: "Artist"},
		},
	}
	found := CollectDiagramNeighbours(root, nil)
	if !found["main.Artist"] {
		t.Errorf("the target of a key is not a neighbour: %v", found)
	}
}

// buildBuilderBoxes returns three tables of a builder diagram.
func buildBuilderBoxes() []BuilderBox {
	return []BuilderBox{
		{Title: "shop.customers", Columns: []BuilderColumnBox{
			{Name: "id", Kind: "int", Picked: true},
			{Name: "name", Kind: "text", Picked: true},
		}},
		{Title: "shop.orders", Columns: []BuilderColumnBox{
			{Name: "id", Kind: "int"},
			{Name: "customer_id", Kind: "int"},
		}},
		{Title: "shop.order_items", Columns: []BuilderColumnBox{
			{Name: "order_id", Kind: "int"},
		}},
	}
}

// The diagram draws one box per table and connects the joined columns.
func TestRenderBuilderDiagramDrawsTheBoxesAndTheLink(t *testing.T) {
	drawn := RenderBuilderDiagram(buildBuilderBoxes()[:2], []BuilderLink{
		{From: 1, FromColumn: "customer_id", To: 0, ToColumn: "id"},
	})

	text := strings.Join(drawn.Lines, "\n")
	for _, wanted := range []string{"shop.customers", "shop.orders", "[x] id", "▶"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("the diagram holds no %q:\n%s", wanted, text)
		}
	}
	if len(drawn.Cells) != 4 || len(drawn.Titles) != 2 {
		t.Errorf("the diagram reports %d cells and %d titles",
			len(drawn.Cells), len(drawn.Titles))
	}
}

// A join between two boxes that do not stand side by side runs under the boxes, so no line
// crosses a box.
func TestRenderBuilderDiagramRunsALongLinkUnderTheBoxes(t *testing.T) {
	drawn := RenderBuilderDiagram(buildBuilderBoxes(), []BuilderLink{
		{From: 2, FromColumn: "order_id", To: 0, ToColumn: "id"},
	})

	if len(drawn.Lines) < 6 {
		t.Fatalf("the diagram drew %d lines", len(drawn.Lines))
	}
	lane := drawn.Lines[len(drawn.Lines)-1]
	if !strings.Contains(lane, "╰") || !strings.Contains(lane, "╯") {
		t.Errorf("the last line holds no lane:\n%s", strings.Join(drawn.Lines, "\n"))
	}
	// The middle box keeps both of its borders on every row it draws.
	left := builderBoxWidth + builderBoxGap
	for _, line := range drawn.Lines[:5] {
		held := []rune(line)[left : left+builderBoxWidth]
		if !strings.ContainsAny(string(held[0]), "╭│╰") ||
			!strings.ContainsAny(string(held[len(held)-1]), "╮│╯") {
			t.Errorf("the link crosses the middle box:\n%s", line)
		}
	}
}

// Every cell names where its column was drawn, so a press picks the column it looks like.
func TestRenderBuilderDiagramReportsWhereEveryColumnLanded(t *testing.T) {
	drawn := RenderBuilderDiagram(buildBuilderBoxes()[:1], nil)

	for _, cell := range drawn.Cells {
		line := drawn.Lines[cell.Y]
		if !strings.Contains(line[cell.X:cell.X+cell.Width], "id") &&
			!strings.Contains(line[cell.X:cell.X+cell.Width], "name") {
			t.Errorf("the cell %+v points at %q", cell, line)
		}
	}
}

// A table the server has not answered for draws its reason in place of the columns.
func TestRenderBuilderDiagramDrawsTheReasonOfATableWithoutColumns(t *testing.T) {
	drawn := RenderBuilderDiagram([]BuilderBox{
		{Title: "shop.orders", Reason: "reading…"},
	}, nil)

	if !strings.Contains(strings.Join(drawn.Lines, "\n"), "reading…") {
		t.Errorf("the box holds no reason:\n%s", strings.Join(drawn.Lines, "\n"))
	}
}

// A diagram wider than the pane is windowed, so the box the cursor stands in is drawn.
func TestScrollBuilderDiagramKeepsTheActiveBoxInView(t *testing.T) {
	drawn := RenderBuilderDiagram(buildBuilderBoxes(), nil)
	width := builderBoxWidth + builderBoxGap + 4

	// The first box is drawn from the left, so nothing moves.
	offset := FindBuilderColumnOffset(0, 0, 3, builderBoxWidth, width)
	held := ScrollBuilderDiagram(drawn, offset, width)
	if !strings.Contains(held.Lines[1], "shop.customers") {
		t.Errorf("the first box is not drawn:\n%s", strings.Join(held.Lines, "\n"))
	}

	// The last box is drawn at the right edge of the window.
	offset = FindBuilderColumnOffset(0, 2, 3, builderBoxWidth, width)
	held = ScrollBuilderDiagram(drawn, offset, width)
	text := strings.Join(held.Lines, "\n")
	if !strings.Contains(text, "shop.order_items") {
		t.Errorf("the last box is not drawn:\n%s", text)
	}
	if strings.Contains(text, "shop.customers") {
		t.Errorf("the window still holds the first box:\n%s", text)
	}
	for _, line := range held.Lines {
		if len([]rune(line)) > measureDiagramWidth(drawn.Lines) {
			t.Errorf("a line grew to %d columns", len([]rune(line)))
		}
	}
}

// The cells of a windowed diagram move with the lines, so a press still lands on the column
// it looks like.
func TestScrollBuilderDiagramMovesItsCells(t *testing.T) {
	drawn := RenderBuilderDiagram(buildBuilderBoxes(), nil)
	width := builderBoxWidth + builderBoxGap + 4

	held := ScrollBuilderDiagram(drawn, FindBuilderColumnOffset(0, 2, 3, builderBoxWidth, width), width)
	for _, cell := range held.Cells {
		if cell.X < 0 || cell.X >= width {
			t.Errorf("the cell %+v stands outside the window", cell)
		}
		line := []rune(held.Lines[cell.Y])
		if cell.X+cell.Width > len(line) {
			continue
		}
		if !strings.Contains(string(line[cell.X:cell.X+cell.Width]), "[") {
			t.Errorf("the cell %+v points at %q", cell, string(line))
		}
	}
}

// The wheel moves the diagram along its boxes, and the cursor pulls it back where it moves
// past the box it stands in.
func TestFindBuilderColumnOffsetFollowsTheWheelAndTheCursor(t *testing.T) {
	width := builderBoxWidth + builderBoxGap + 4
	stride := builderBoxWidth + builderBoxGap

	// A diagram the wheel moved follows no cursor: it stands where the wheel left it.
	if held := FindBuilderColumnOffset(6, -1, 3, builderBoxWidth, width); held != 6 {
		t.Errorf("the wheel left the diagram at column %d", held)
	}
	// A cursor to the left of the window pulls it back to that box.
	if held := FindBuilderColumnOffset(stride*2, 0, 3, builderBoxWidth, width); held != 0 {
		t.Errorf("the cursor left the diagram at column %d", held)
	}
	// A cursor to the right of the window pulls it on.
	if held := FindBuilderColumnOffset(0, 2, 3, builderBoxWidth, width); held != stride*2+builderBoxWidth-width {
		t.Errorf("the cursor left the diagram at column %d", held)
	}
	// The wheel stops at the last box, and never before the first.
	if held := FindBuilderColumnOffset(9999, -1, 3, builderBoxWidth, width); held != stride*3-builderBoxGap-width {
		t.Errorf("the wheel ran past the last box to column %d", held)
	}
	if held := FindBuilderColumnOffset(-40, 0, 3, builderBoxWidth, width); held != 0 {
		t.Errorf("the wheel ran before the first box to column %d", held)
	}
}

func TestAnErDiagramBoxHoldsTheLongestType(t *testing.T) {
	root := DiagramTable{Schema: "public", Name: "orders", Columns: []DiagramColumn{
		{Name: "id", Type: "integer", Primary: true},
		{Name: "placed_at", Type: "timestamp with time zone"},
	}}
	drawn := RenderErDiagram(root, nil, DiagramMarks{Primary: "*"})
	if !strings.Contains(strings.Join(drawn.Lines, "\n"), "timestamp with time zone │") {
		t.Errorf("the box cuts the type:\n%s", strings.Join(drawn.Lines, "\n"))
	}
}

func TestABuilderBoxKeepsABlankBeforeItsBorder(t *testing.T) {
	drawn := RenderBuilderDiagram([]BuilderBox{{
		Title: "public.customers c",
		Columns: []BuilderColumnBox{
			{Name: "id", Kind: "integer"}, {Name: "created_at", Kind: "timestamp with time zone"},
		},
	}}, nil)
	written := strings.Join(drawn.Lines, "\n")
	if !strings.Contains(written, "timestamp with time zone │") || !strings.Contains(written, "integer │") {
		t.Errorf("the box cuts a type or touches its border:\n%s", written)
	}
}
