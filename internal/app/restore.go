package app

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Workspace persistence restores tab state. Table and object tabs load data on first display.

// RestoreTabs restores saved tabs or preserves the current tabs when the saved list is empty.
func (connection *Connection) RestoreTabs(saved hist.SavedWorkspace, buildPreview PreviewBuilder) {
	if len(saved.Tabs) == 0 {
		return
	}

	tabs := make([]*Tab, 0, len(saved.Tabs))
	unread := map[int]bool{}
	for _, held := range saved.Tabs {
		connection.nextTabID++
		tab := buildRestoredTab(connection.nextTabID, held, buildPreview)
		if tab.Kind != TabQuery && tab.Kind != TabNotebook {
			unread[tab.ID] = true
		}
		tabs = append(tabs, tab)
	}

	connection.Tabs = tabs
	connection.Unread = unread
	connection.ActiveIndex = core.ClampIndex(saved.ActiveIndex, len(tabs))
}

// PreviewBuilder returns the read of a table, which a restored table tab starts with.
type PreviewBuilder func(table db.TableRef) string

// buildRestoredTab returns the tab of one stored tab.
func buildRestoredTab(id int, saved hist.SavedTab, buildPreview PreviewBuilder) *Tab {
	switch saved.Kind {
	case "object":
		tab := NewObjectTab(id, db.SchemaObject{
			Schema: saved.Schema, Name: saved.Name,
			Kind: db.SchemaObjectKind(saved.ObjectKind),
			// Only the tree shows a detail next to a name.
			Identity: saved.Identity,
		})
		applySavedState(tab, saved.State)
		return tab
	case "notebook":
		// The text of the notebook is stored, so a notebook that was never saved comes
		// back as well, and no cell is run to restore it.
		tab := NewNotebookTab(id, notebook.Parse(saved.SQL), saved.Identity,
			notebook.Origin(saved.ObjectKind))
		applySavedState(tab, saved.State)
		applySavedCells(tab, saved.State)
		return tab
	case "builder":
		tab := NewBuilderTab(id)
		tab.Builder = buildRestoredBuilder(saved.State.Builder)
		applySavedState(tab, saved.State)
		return tab
	case "table":
		table := db.TableRef{
			Schema: saved.Schema, Name: saved.Name,
			Kind: db.RelationKind(saved.TableKind),
		}
		tab := NewTableTab(id, table, buildPreview(table))
		applySavedState(tab, saved.State)
		return tab
	}
	tab := NewQueryTab(id, saved.SQL)
	applySavedState(tab, saved.State)
	return tab
}

// applySavedCells puts the list of a restored notebook back on the cell it stood on, with
// the cells that were folded away still folded.
func applySavedCells(tab *Tab, state hist.SavedTabState) {
	folded := map[string]bool{}
	for _, id := range state.Folded {
		folded[id] = true
	}
	for _, cell := range tab.Notebook.Cells {
		cell.Folded = folded[cell.ID]
	}
	tab.Notebook.FocusCell(state.Cell)
	tab.SettleFocusedCell()
	// The sort, the filter and the caret of the stored tab belong to the cell it stood on.
	tab.Sort, tab.Filter = state.Sort, state.Filter
	if state.Caret > 0 && state.Caret <= len(tab.Editor.Text) {
		tab.Editor.Caret, tab.Editor.Anchor = state.Caret, state.Caret
	}
	tab.KeepFocusedCell()
}

// applySavedState applies the sort, the filter and the caret of a stored tab.
func applySavedState(tab *Tab, state hist.SavedTabState) {
	tab.Sort = state.Sort
	tab.Filter = state.Filter
	tab.PaneHeight = state.PaneHeight
	if state.Caret > 0 && state.Caret <= len(tab.Editor.Text) {
		tab.Editor.Caret = state.Caret
		tab.Editor.Anchor = state.Caret
	}
}

// TakeUnread clears and returns the pending first-read flag for a tab.
func (connection *Connection) TakeUnread(tab *Tab) bool {
	if tab == nil || !connection.Unread[tab.ID] {
		return false
	}
	delete(connection.Unread, tab.ID)
	return true
}

// BuildWorkspaceSnapshot returns the tabs in the form the history file stores.
func (connection *Connection) BuildWorkspaceSnapshot() hist.SavedWorkspace {
	tabs := make([]hist.SavedTab, 0, len(connection.Tabs))
	for _, tab := range connection.Tabs {
		tabs = append(tabs, buildSavedTab(tab))
	}
	connection.workspaceChange++
	return hist.SavedWorkspace{
		Tabs: tabs, ActiveIndex: connection.ActiveIndex,
		Change: connection.workspaceChange,
	}
}

// buildSavedTab returns one tab in the form the history file stores.
func buildSavedTab(tab *Tab) hist.SavedTab {
	state := hist.SavedTabState{
		Caret: tab.Editor.Caret, Sort: tab.Sort, Filter: tab.Filter,
		PaneHeight: tab.PaneHeight,
	}
	switch tab.Kind {
	case TabTable:
		return hist.SavedTab{
			Kind: "table", Schema: tab.Table.Schema, Name: tab.Table.Name,
			TableKind: string(tab.Table.Kind), State: state,
		}
	case TabObject:
		return hist.SavedTab{
			Kind: "object", Schema: tab.Object.Schema, Name: tab.Object.Name,
			ObjectKind: string(tab.Object.Kind), Identity: tab.Object.Identity, State: state,
		}
	}
	if tab.Kind == TabBuilder && tab.Builder != nil {
		state.Builder = buildSavedBuilder(tab.Builder)
		return hist.SavedTab{Kind: "builder", State: state}
	}
	if tab.Kind == TabNotebook && tab.Notebook != nil {
		state.Cell = tab.Notebook.Focused
		state.Folded = tab.Notebook.ListFoldedCells()
		return hist.SavedTab{
			Kind: "notebook", SQL: notebook.Write(tab.Notebook.BuildDocument()),
			Identity: tab.Notebook.Path, ObjectKind: string(tab.Notebook.Origin),
			State: state,
		}
	}
	return hist.SavedTab{Kind: "query", SQL: tab.Editor.Text, State: state}
}

// describeBuilderSignature returns the text signature of a builder, so a change to its
// tables, its joins or its filters is stored.
func describeBuilderSignature(builder *Builder) string {
	var written strings.Builder
	for _, table := range builder.Tables {
		written.WriteString("\x00" + table.Ref.Schema + "." + table.Ref.Name + "\x00" + table.Alias)
		for _, column := range table.Columns {
			if !column.Picked {
				continue
			}
			written.WriteString("\x00" + column.Name + "\x00" + string(column.Aggregate) +
				"\x00" + column.As + "\x00" + string(column.Sort))
		}
	}
	for _, join := range builder.Joins {
		written.WriteString("\x00" + string(join.Kind) + "\x00" + strconv.Itoa(join.Table) +
			"\x00" + strconv.Itoa(join.Base) + "\x00" + strings.Join(join.Columns, ",") +
			"\x00" + strings.Join(join.BaseColumns, ",") + "\x00" + join.On)
	}
	written.WriteString("\x00" + strings.Join(builder.Filters, "\x00"))
	return written.String()
}

// buildSavedBuilder returns the builder in the form the history file stores. The columns of
// the server are read again at the next connect, so only the picked ones are stored.
func buildSavedBuilder(builder *Builder) *hist.SavedBuilder {
	saved := &hist.SavedBuilder{Filters: builder.Filters, Limit: builder.Limit}
	for _, table := range builder.Tables {
		held := hist.SavedBuilderTable{
			Schema: table.Ref.Schema, Name: table.Ref.Name, Alias: table.Alias,
		}
		for _, column := range table.Columns {
			if !column.Picked {
				continue
			}
			held.Columns = append(held.Columns, hist.SavedBuilderColumn{
				Name: column.Name, Aggregate: string(column.Aggregate),
				As: column.As, Sort: string(column.Sort),
			})
		}
		saved.Tables = append(saved.Tables, held)
	}
	for _, join := range builder.Joins {
		saved.Joins = append(saved.Joins, hist.SavedBuilderJoin{
			Kind: string(join.Kind), Table: join.Table, Base: join.Base,
			Columns: join.Columns, BaseColumns: join.BaseColumns, On: join.On,
		})
	}
	return saved
}

// buildRestoredBuilder returns the builder of a stored tab. Every table waits for its
// columns, which the connection reads again.
func buildRestoredBuilder(saved *hist.SavedBuilder) *Builder {
	builder := NewBuilder()
	if saved == nil {
		return builder
	}
	builder.Filters, builder.Limit = saved.Filters, saved.Limit
	for _, table := range saved.Tables {
		held := BuilderTable{
			Ref:   db.TableRef{Schema: table.Schema, Name: table.Name},
			Alias: table.Alias, Reading: true,
		}
		for _, column := range table.Columns {
			held.Columns = append(held.Columns, BuilderColumn{
				Name: column.Name, Picked: true,
				Aggregate: statement.Aggregate(column.Aggregate), As: column.As,
				Sort: core.SortDirection(column.Sort),
			})
		}
		builder.Tables = append(builder.Tables, held)
	}
	for _, join := range saved.Joins {
		builder.Joins = append(builder.Joins, BuilderJoin{
			Kind: statement.JoinKind(join.Kind), Table: join.Table, Base: join.Base,
			Columns:     readSavedJoinColumns(join.Columns, join.Column),
			BaseColumns: readSavedJoinColumns(join.BaseColumns, join.BaseColumn),
			On:          join.On,
		})
	}
	return builder
}

// readSavedJoinColumns returns the stored columns of one side of a join, and the one column
// a client stored before a key of several columns was kept.
func readSavedJoinColumns(columns []string, single string) []string {
	if len(columns) > 0 || single == "" {
		return columns
	}
	return []string{single}
}

// DescribeTabs returns a text signature of the active index, tab identities, and editor contents.
func (connection *Connection) DescribeTabs() string {
	var written strings.Builder
	written.WriteString(strconv.Itoa(connection.ActiveIndex))
	for _, tab := range connection.Tabs {
		written.WriteString("\x00" + strconv.Itoa(tab.ID) + "\x00" + string(tab.Kind) + "\x00" +
			tab.Table.Schema + "." + tab.Table.Name + "\x00" +
			tab.Object.Schema + "." + tab.Object.Name + "\x00" + tab.Editor.Text)
		if tab.Builder != nil {
			written.WriteString(describeBuilderSignature(tab.Builder))
		}
		if tab.Notebook == nil {
			continue
		}
		written.WriteString("\x00" + tab.Notebook.Path + "\x00" +
			strconv.Itoa(tab.Notebook.Focused))
		for _, cell := range tab.Notebook.Cells {
			written.WriteString("\x00" + cell.ID + "\x00" + string(cell.Kind) +
				"\x00" + strconv.FormatBool(cell.Folded) + "\x00" + cell.Editor.Text)
		}
	}
	return written.String()
}
