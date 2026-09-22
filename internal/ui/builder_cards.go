package ui

import (
	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query/statement"
)

// The three cards of the query builder: the tables of the server, the join of one table,
// and the aggregate, the name and the sort of one picked column.

// The widths of the cards.
const (
	builderCardLabelWidth = 12
	builderTableWidth     = 48
)

// filterBuilderTables returns the tables the term of the picker keeps.
func (model *Model) filterBuilderTables(overlay app.Overlay) []app.PaletteAction {
	return keepMatchingRows(overlay.Palette, model.readOverlayTerm(overlay),
		func(action app.PaletteAction) string { return action.Label })
}

// renderBuilderTables draws the tables a builder tab can add.
func (model *Model) renderBuilderTables(overlay app.Overlay, width int) string {
	tables := model.filterBuilderTables(overlay)
	rows := make([]string, 0, len(tables))
	for at, table := range tables {
		rows = append(rows, model.renderListRow(ListRowSpec{
			Label: table.Label, LabelWidth: builderTableWidth,
			Selected: at == overlay.List.Cursor, Width: width,
		}))
	}
	return model.renderListCard(ListCard{
		Kind: app.OverlayBuilderTables, Title: overlay.Title,
		Filter: model.renderFilterFieldOf(overlay, width, "table", len(tables)),
		Rows:   rows, Cursor: overlay.List.Cursor, Offset: overlay.List.Offset,
		Rolled: overlay.List.Rolled, Width: width, ReportsNoMatch: true,
		Keys:        model.buildCardKeys(app.OverlayBuilderTables, keyScene{overlay: overlay}),
		ContentRows: len(tables) + 1,
	})
}

// renderBuilderJoin draws the card of one join: the kind, and the condition.
func (model *Model) renderBuilderJoin(
	tab *app.Tab, overlay app.Overlay, width int,
) string {
	if !tab.BuildsQuery() {
		return ""
	}
	kind := statement.JoinInner
	if join, found := tab.Builder.FindJoin(overlay.Field); found {
		kind = join.Kind
	}

	model.layout.formChoices = nil
	// The card opens on the kind, and a typed character moves the marker to the condition.
	lines := []string{
		model.styles.Muted().Render(present.TruncateText(overlay.Body, width-4)),
		"",
		model.renderCardRow(cardRow{
			label: "join", value: string(kind), width: width,
			field: builderJoinKindRow, row: cardBodyRow + builderJoinRowsAbove,
			focused: overlay.List.Cursor == builderJoinKindRow,
		}),
		model.renderCardRow(cardRow{
			label: "on", width: width,
			field: builderJoinOnRow, row: cardBodyRow + builderJoinRowsAbove + 1,
			focused: overlay.List.Cursor == builderJoinOnRow, draft: overlay.Draft,
		}),
	}
	model.recordCardRows(builderJoinRows, builderJoinRowsAbove, width)
	keys := model.buildCardKeys(app.OverlayBuilderJoin, keyScene{overlay: overlay})
	return model.renderTextCard(app.OverlayBuilderJoin, overlay.Title, width, lines,
		keys, len(lines), plainCard)
}

// The rows of the join card. The kind steps with the arrows and the condition is typed.
const (
	builderJoinKindRow = 0
	builderJoinOnRow   = 1
	builderJoinRows    = 2
	// builderJoinRowsAbove are the lines the card draws over its rows: what proposed the
	// join, and the blank row under it.
	builderJoinRowsAbove = 2
)

// renderBuilderField draws the card of one picked column: the aggregate, the name of the
// result column, and the sort.
func (model *Model) renderBuilderField(
	tab *app.Tab, overlay app.Overlay, width int,
) string {
	if !tab.BuildsQuery() {
		return ""
	}
	column, found := tab.Builder.ActiveColumn()
	if !found {
		return ""
	}

	model.layout.formChoices = nil
	lines := []string{
		model.renderCardRow(cardRow{
			label: "aggregate", value: describeAggregateName(column.Aggregate),
			width: width, field: builderFieldAggregate,
			row:     cardBodyRow + builderFieldAggregate,
			focused: overlay.Field == builderFieldAggregate,
		}),
		model.renderCardRow(cardRow{
			label: "name", value: column.As, width: width, field: builderFieldAlias,
			row:     cardBodyRow + builderFieldAlias,
			focused: overlay.Field == builderFieldAlias, draft: overlay.Draft,
		}),
		model.renderCardRow(cardRow{
			label: "sort", value: describeSortName(column.Sort), width: width,
			field: builderFieldSort, row: cardBodyRow + builderFieldSort,
			focused: overlay.Field == builderFieldSort,
		}),
	}
	model.recordCardRows(len(lines), 0, width)
	keys := model.buildCardKeys(app.OverlayBuilderField, keyScene{overlay: overlay})
	return model.renderTextCard(app.OverlayBuilderField, overlay.Title, width, lines,
		keys, len(lines), plainCard)
}

// recordCardRows keeps where the rows of a card were drawn, so a press marks the row it
// looks like. The rows of a card start under its top border and its blank row.
func (model *Model) recordCardRows(rows, above, width int) {
	model.layout.formRows = rowsHit{
		top: cardBodyRow + above, count: rows,
		from: cardBodyColumn - 1, to: cardBodyColumn + width - 4,
	}
}

// describeAggregateName returns the aggregate as the card writes it.
func describeAggregateName(held statement.Aggregate) string {
	if held == statement.AggregateNone {
		return "none"
	}
	return string(held)
}

// describeSortName returns the direction as the card writes it.
func describeSortName(held core.SortDirection) string {
	if held == "" {
		return "none"
	}
	return string(held)
}

// cardRow is one row of a builder card: its label, what it holds, and where it was drawn.
type cardRow struct {
	label string
	value string
	width int
	// field is the row of the card the keys step, and row is where it was drawn in the
	// card, which the marks of a choice are recorded at.
	field int
	row   int
	// focused is true for the row the marker stands on.
	focused bool
	// draft is the field of a row that takes typed text.
	draft *app.EditorBuffer
}

// renderCardRow draws one row of a builder card: the label, and either a value the arrows
// step or a field that takes typed text. The focused row carries the marker of the form.
func (model *Model) renderCardRow(held cardRow) string {
	theme := model.styles.Theme
	valueWidth := max(held.width-present.CardChrome-builderCardLabelWidth, 8)

	marker := "  "
	labelStyle := model.styles.Muted()
	if held.focused {
		marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
		labelStyle = model.styles.Accent()
	}

	written := model.styles.Muted().Render(present.TruncateText(held.value, valueWidth))
	switch {
	case held.draft != nil && held.focused:
		written = model.renderField(held.draft, valueWidth, FieldLook{
			Ground: theme.Header, Ink: theme.Text, Focused: true,
			Placeholder: "the column as it stands",
		})
	case held.draft != nil:
		written = model.styles.Muted().Render(present.TruncateText(
			describeDraftValue(held.draft), valueWidth))
	case held.focused:
		written = model.renderChoiceField(held.value, valueWidth, held.field,
			held.row, cardBodyColumn+builderCardLabelWidth, held.focused)
	}
	return labelStyle.Render(marker+present.FitText(
		held.label, builderCardLabelWidth-fieldMarkerWidth)) + written
}

// describeDraftValue returns what a field that is not focused shows.
func describeDraftValue(draft *app.EditorBuffer) string {
	if draft == nil || draft.Text == "" {
		return "the column as it stands"
	}
	return draft.Text
}
