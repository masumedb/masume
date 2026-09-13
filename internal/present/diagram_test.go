package present

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query"
)

func TestRenderErDiagram(t *testing.T) {
	root := DiagramTable{
		Schema: "main", Name: "Album",
		Columns: []DiagramColumn{
			{Name: "AlbumId", Primary: true},
			{Name: "Title"},
			{Name: "ArtistId", Foreign: true},
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

	lines := RenderErDiagram(root, related)
	drawn := strings.Join(lines, "\n")
	for _, wanted := range []string{"main.Album", "main.Artist", "PK", "FK", "▶"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the diagram has no %q:\n%s", wanted, drawn)
		}
	}
	width := MeasureText(lines[0])
	for at, line := range lines {
		if MeasureText(line) != width {
			t.Errorf("row %d is %d wide, wanted %d", at, MeasureText(line), width)
		}
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
	offset := FindBuilderColumnOffset(0, 0, 3, width)
	held := ScrollBuilderDiagram(drawn, offset, width)
	if !strings.Contains(held.Lines[1], "shop.customers") {
		t.Errorf("the first box is not drawn:\n%s", strings.Join(held.Lines, "\n"))
	}

	// The last box is drawn at the right edge of the window.
	offset = FindBuilderColumnOffset(0, 2, 3, width)
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

	held := ScrollBuilderDiagram(drawn, FindBuilderColumnOffset(0, 2, 3, width), width)
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
	if held := FindBuilderColumnOffset(6, -1, 3, width); held != 6 {
		t.Errorf("the wheel left the diagram at column %d", held)
	}
	// A cursor to the left of the window pulls it back to that box.
	if held := FindBuilderColumnOffset(stride*2, 0, 3, width); held != 0 {
		t.Errorf("the cursor left the diagram at column %d", held)
	}
	// A cursor to the right of the window pulls it on.
	if held := FindBuilderColumnOffset(0, 2, 3, width); held != stride*2+builderBoxWidth-width {
		t.Errorf("the cursor left the diagram at column %d", held)
	}
	// The wheel stops at the last box, and never before the first.
	if held := FindBuilderColumnOffset(9999, -1, 3, width); held != stride*3-builderBoxGap-width {
		t.Errorf("the wheel ran past the last box to column %d", held)
	}
	if held := FindBuilderColumnOffset(-40, 0, 3, width); held != 0 {
		t.Errorf("the wheel ran before the first box to column %d", held)
	}
}
