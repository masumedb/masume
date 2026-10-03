package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query/editor"
	"github.com/masumedb/masume/internal/query/statement"
)

// completionRelation is one relation of the statement at the caret, with its catalog detail.
type completionRelation struct {
	reference statement.TableReference
	detail    db.TableDetail
}

// qualifier returns the name the columns of the relation take: its alias, or its name.
func (relation completionRelation) qualifier() string {
	if relation.reference.HasAlias {
		return relation.reference.Alias
	}
	return relation.reference.Name
}

// findCompletionRelations returns the relations that text reads whose detail the catalog holds.
func (model *Model) findCompletionRelations(
	connection *app.Connection, text string,
) []completionRelation {
	relations := []completionRelation{}
	flavour := connection.Session.Dialect().Syntax
	for _, reference := range statement.FindTableReferences(text, flavour) {
		table, found := model.findTableByName(connection, reference.SelectSource)
		if !found {
			continue
		}
		state, read := connection.Catalog.Details[present.BuildTableID(table)]
		if !read || state.Kind != present.DetailReady {
			continue
		}
		relations = append(relations, completionRelation{reference: reference, detail: state.Detail})
	}
	return relations
}

// buildCompletionColumns returns the columns of the relations, keyed in lower case under the
// name of each relation and under any alias it takes, and the relations in order.
func buildCompletionColumns(
	relations []completionRelation,
) (map[string][]editor.CompletionColumn, []editor.CompletionReference) {
	byQualifier := map[string][]editor.CompletionColumn{}
	references := []editor.CompletionReference{}
	for _, relation := range relations {
		reference, state := relation.reference, relation
		columns := make([]editor.CompletionColumn, 0, len(state.detail.Columns))
		for _, column := range state.detail.Columns {
			// A key column says so, because the name alone does not.
			detail := column.DataType
			if column.IsPrimaryKey {
				detail += " pk"
			}
			columns = append(columns, editor.CompletionColumn{
				Name: column.Name, Detail: detail,
			})
		}
		byQualifier[strings.ToLower(reference.Name)] = columns
		if reference.HasAlias {
			byQualifier[strings.ToLower(reference.Alias)] = columns
		}
		references = append(references, editor.CompletionReference{
			Qualifier: relation.qualifier(), Columns: columns,
		})
	}
	return byQualifier, references
}

// buildJoinConditions returns the conditions the foreign keys between the joined relation and
// the relations before it offer, such as "o.customer_id = c.id".
func buildJoinConditions(relations []completionRelation, name, alias string) []string {
	joined := -1
	for at, relation := range relations {
		reference := relation.reference
		if (alias != "" && strings.EqualFold(reference.Alias, alias)) ||
			(alias == "" && !reference.HasAlias && strings.EqualFold(
				lastNamePart(name), reference.Name)) {
			joined = at
		}
	}
	if joined < 0 {
		return nil
	}
	target := relations[joined]
	conditions := []string{}
	for at, other := range relations {
		if at == joined {
			continue
		}
		conditions = append(conditions, buildForeignKeyConditions(target, other)...)
		conditions = append(conditions, buildForeignKeyConditions(other, target)...)
	}
	return conditions
}

// buildForeignKeyConditions returns one condition per foreign key of from that points at to.
func buildForeignKeyConditions(from, to completionRelation) []string {
	conditions := []string{}
	for _, key := range from.detail.ForeignKeys {
		if !strings.EqualFold(key.TargetTable, to.detail.Table.Name) ||
			(key.TargetSchema != "" && to.detail.Table.Schema != "" &&
				!strings.EqualFold(key.TargetSchema, to.detail.Table.Schema)) ||
			len(key.Columns) != len(key.TargetColumns) {
			continue
		}
		pairs := make([]string, 0, len(key.Columns))
		for at, column := range key.Columns {
			pairs = append(pairs, from.qualifier()+"."+column+" = "+
				to.qualifier()+"."+key.TargetColumns[at])
		}
		conditions = append(conditions, strings.Join(pairs, " and "))
	}
	return conditions
}

// lastNamePart returns the name after the last dot of a qualified name, without quotes.
func lastNamePart(name string) string {
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	return strings.Trim(name, "\"`[]")
}

// buildCompletionSources returns everything the catalog and the result offer the caret.
func (model *Model) buildCompletionSources(
	connection *app.Connection, tab *app.Tab,
) editor.CompletionSources {
	schemas, tables, functions := connection.CompletionSources()

	columns := []editor.CompletionColumn{}
	if held := tab.Results.Active(); held != nil && held.State.Kind == app.QuerySucceeded {
		for _, column := range held.State.Result.Columns {
			columns = append(columns, editor.CompletionColumn{
				Name: column.Name, Detail: column.DescribeType(),
			})
		}
	}

	// The relations of the statement at the caret, not of every statement in the buffer,
	// so a name of another statement is never offered here.
	relations := model.findCompletionRelations(
		connection, tab.Editor.ReadStatementAtCaret(connection.Session.Language()))
	byQualifier, references := buildCompletionColumns(relations)
	sources := editor.CompletionSources{
		Schemas: schemas, Tables: tables, Functions: functions, Columns: columns,
		ColumnsByQualifier: byQualifier, References: references,
	}
	offset := tab.Editor.Caret - len(editor.ReadPrefix(tab.Editor.Text, tab.Editor.Caret))
	if name, alias, joins := editor.ReadJoinTarget(tab.Editor.Text, offset); joins {
		sources.JoinConditions = buildJoinConditions(relations, name, alias)
	}
	return sources
}

// refreshCompletion builds the list for the caret.
func (model *Model) refreshCompletion(connection *app.Connection, tab *app.Tab) {
	list := &tab.Completion
	if !tab.EditsStatements() {
		list.Close()
		return
	}
	if tab.Focus != app.PaneEditor || connection.Overlay.IsOpen() || list.Dismissed {
		list.Close()
		return
	}

	text := tab.Editor.Text
	offset := tab.Editor.Caret
	prefix := editor.ReadPrefix(text, offset)
	found := connection.Session.Language().BuildCompletions(
		prefix, model.buildCompletionSources(connection, tab),
		editor.CompletionContext{
			AllowQualified: !editor.IsUpdateSetTarget(text, offset),
			// Read from the start of the word, because the text before it decides
			// what may follow.
			NamePosition:  editor.ResolveNamePosition(text, offset-len(prefix)),
			JoinCondition: hasJoinTarget(text, offset-len(prefix)),
		})

	if len(found) == 0 {
		list.Close()
		return
	}
	list.Candidates = found
	list.Selected = 0
}

// hasJoinTarget is true where the text before the offset ends at the ON of a join.
func hasJoinTarget(text string, offset int) bool {
	_, _, joins := editor.ReadJoinTarget(text, offset)
	return joins
}

// acceptCompletion writes the marked candidate in place of the word under the caret.
func (model *Model) acceptCompletion(connection *app.Connection, tab *app.Tab) {
	chosen, found := tab.Completion.Chosen()
	if !found {
		return
	}
	written, caret := editor.ApplyCompletion(
		tab.Editor.Text, tab.Editor.Caret, chosen, connection.Session.Dialect())
	tab.Editor.SetTextWithCaret(written, caret)
	tab.Completion.Close()
}

// completionRows is how many suggestions the popup shows at once.
const completionRows = 6

// completionChrome is the border and the padding of the popup.
const completionChrome = 2

// renderCompletionPopup draws the suggestions over whatever stands under the editor.
// The pane cannot paint over its neighbour, so the workspace places it on the frame.
func (model *Model) renderCompletionPopup(tab *app.Tab, height int) (string, int, int) {
	list := &tab.Completion
	model.layout.completionRows = rowsHit{}
	if !list.IsListing() {
		return "", 0, 0
	}
	theme := model.styles.Theme

	widest := 0
	for _, candidate := range list.Candidates {
		if measured := measureCompletionRow(candidate); measured > widest {
			widest = measured
		}
	}
	width := min(widest+completionChrome, model.width)

	shownRows := min(len(list.Candidates), completionRows)
	popupHeight := shownRows + completionChrome

	// Below the caret where there is room, and above it where there is not. A popup
	// past the bottom of the screen would show one row only. The fault row under the
	// statement stays in view. Above the caret, the popup stays inside the editor pane.
	limit := height
	if model.faultRow > 0 {
		limit = model.faultRow
	}
	roomBelow := limit - model.caretRow - 1
	roomAbove := model.caretRow - model.paneTop
	if !list.Placed {
		list.Placed = true
		list.Above = popupHeight > roomBelow &&
			(popupHeight <= roomAbove || roomAbove > roomBelow)
	}
	room := roomBelow
	if list.Above && roomAbove > completionChrome {
		room = roomAbove
	} else {
		list.Above = false
	}
	if popupHeight > room && room > completionChrome {
		shownRows = room - completionChrome
		popupHeight = room
	}
	top := model.caretRow + 1
	if list.Above {
		top = model.caretRow - popupHeight
	}

	// The window follows the marked row, because the list is longer than the popup.
	start := 0
	if len(list.Candidates) > shownRows {
		start = core.ClampWithin(list.Selected-shownRows/2, len(list.Candidates)-shownRows)
	}

	lines := make([]string, 0, shownRows)
	for at := start; at < start+shownRows && at < len(list.Candidates); at++ {
		lines = append(lines, model.renderCompletionRow(
			list.Candidates[at], at == list.Selected, width-completionChrome))
	}

	left := core.ClampWithin(model.caretColumn, model.width-width)

	// The rows of the popup, so a press takes the candidate it lands on. The box is placed
	// on the frame of the workspace and the title bar is put over it afterwards, so a row of
	// the screen is one more than a row of the frame. The border of the box takes its first
	// row and its first column.
	model.layout.completionRows = rowsHit{
		top: top + 1 + titleBarRows, count: shownRows, offset: start,
		from: left + 1, to: left + width - 2,
	}

	count := " " + strconv.Itoa(list.Selected+1) + "/" + strconv.Itoa(len(list.Candidates)) + " "
	return model.styles.RenderBox(BoxOptions{
		Width: width, Height: popupHeight, Lines: lines, Ground: theme.Header,
		BottomNote: model.styles.Muted().Background(theme.Header).Render(count),
	}), left, top
}

// measureCompletionRow returns the width of one row: the name, the kind and the detail.
func measureCompletionRow(candidate editor.Completion) int {
	detail := ""
	if candidate.Detail != "" {
		detail = " · " + candidate.Detail
	}
	return present.MeasureText(candidate.Text) + present.MeasureText(string(candidate.Kind)) +
		present.MeasureText(detail) + rowPaddingLeft + 3
}

// renderCompletionRow draws one suggestion: the name at the left, and what it is at the
// right. The kind tells a column from a table of the same name.
func (model *Model) renderCompletionRow(
	candidate editor.Completion, marked bool, width int,
) string {
	theme := model.styles.Theme
	ground := theme.Header
	if marked {
		ground = theme.Accent
	}

	detail := ""
	if candidate.Detail != "" {
		detail = " · " + candidate.Detail
	}
	kindInk := model.styles.CompletionKindColor(candidate.Kind)
	nameInk, detailInk := theme.Text, theme.Faint
	if marked {
		nameInk, kindInk, detailInk = theme.OnAccent, theme.OnAccent, theme.OnAccent
	}

	right := lipgloss.NewStyle().Foreground(kindInk).Background(ground).
		Render(string(candidate.Kind)) +
		lipgloss.NewStyle().Foreground(detailInk).Background(ground).Render(detail)
	inner := width - rowPaddingLeft - 1
	room := inner - present.MeasureText(string(candidate.Kind)) -
		present.MeasureText(detail) - 1
	name := lipgloss.NewStyle().Foreground(nameInk).Background(ground).
		Render(present.TruncateText(candidate.Text, room))

	gap := max(inner-present.MeasureText(present.TruncateText(candidate.Text, room))-
		present.MeasureText(string(candidate.Kind))-present.MeasureText(detail), 1)
	pad := lipgloss.NewStyle().Background(ground).Render(" ")
	gutter := lipgloss.NewStyle().Foreground(nameInk).Background(ground).
		Render(model.buildRowGutter(marked))
	return gutter + name +
		lipgloss.NewStyle().Background(ground).Render(strings.Repeat(" ", gap)) + right + pad
}
