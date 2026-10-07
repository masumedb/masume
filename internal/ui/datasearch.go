package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/datasearch"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// dataMatchPrefix starts the menu ID of one table a data search matched.
const dataMatchPrefix = "data-match:"

// dataSearchRows is the most rows a search reads of one table.
const dataSearchRows = 100

// dataSearchTimeout is the time limit of a whole search.
const dataSearchTimeout = 5 * time.Minute

// dataSearchState is the schema a search runs on, and the tables it matched.
type dataSearchState struct {
	schema  string
	text    string
	matches []datasearch.Match
}

// dataSearchMsg carries what a search found, or why it stopped.
type dataSearchMsg struct {
	ConnectionID int
	Schema       string
	Text         string
	Report       datasearch.Report
	Problem      string
}

func (model *Model) openDataSearch(connection *app.Connection, schema string) (tea.Model, tea.Cmd) {
	model.search.schema = schema
	connection.Open(app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptDataSearch, Title: "search data in " + schema,
		Hint:  "every column of every table, as text, in any case; % and _ are wildcards",
		Draft: app.NewEditorBuffer(model.search.text, len(model.search.text)),
	})
	return model, nil
}

func (model *Model) searchData(connection *app.Connection, text string) (tea.Model, tea.Cmd) {
	if text == "" {
		return model, nil
	}
	schema := model.search.schema
	model.search.text = text
	session := connection.Session
	id := model.connections.idOf(connection)
	connection.Show("searching " + schema + " for " + text + "…")
	return model, func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), dataSearchTimeout)
		defer stop()
		report, err := datasearch.Search(ctx, session, schema, text, dataSearchRows)
		answered := dataSearchMsg{ConnectionID: id, Schema: schema, Text: text, Report: report}
		if err != nil {
			answered.Problem = db.DescribeError(err)
		}
		return answered
	}
}

// readDataSearch lists the matched tables in a menu. Enter opens one, filtered to the rows
// that matched.
func (model *Model) readDataSearch(answered dataSearchMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	report := answered.Report
	verb := " hold "
	if len(report.Matches) == 1 {
		verb = " holds "
	}
	summary := present.FormatCountOf(int64(len(report.Matches)), "table", "tables") + " of " +
		strconv.Itoa(report.Searched) + verb + answered.Text
	if len(report.Failed) > 0 {
		summary += " · not searched: " + strings.Join(report.Failed, ", ")
	}
	connection.Show(summary)
	if len(report.Matches) == 0 {
		return model, nil
	}

	model.search.matches = report.Matches
	actions := make([]app.MenuAction, 0, len(report.Matches))
	for at, match := range report.Matches {
		rows := present.FormatCountOf(int64(match.Rows), "row", "rows")
		if match.Capped {
			rows = strconv.Itoa(match.Rows) + "+ rows"
		}
		detail := rows
		if len(match.Columns) > 0 {
			detail += " · " + strings.Join(match.Columns, ", ")
		}
		actions = append(actions, app.MenuAction{
			ID: dataMatchPrefix + strconv.Itoa(at), Label: match.Table.Name, Detail: detail,
			Icon: present.TableIcons[match.Table.Kind],
		})
	}
	connection.Open(app.Overlay{
		Kind: app.OverlayActionMenu, Title: " " + answered.Text + " in " + answered.Schema + " ",
		Draft: app.NewEditorBuffer("", 0), Actions: actions,
	})
	return model, nil
}

// openDataMatch opens the table of one match, filtered to the rows that hold the text.
func (model *Model) openDataMatch(connection *app.Connection, chosen string) (tea.Model, tea.Cmd) {
	at, err := strconv.Atoi(strings.TrimPrefix(chosen, dataMatchPrefix))
	if err != nil || at < 0 || at >= len(model.search.matches) {
		return model, nil
	}
	match := model.search.matches[at]
	step, _ := core.BuildRawFilter(match.Filter)
	preview := connection.Session.Composer().ComposeRelationRead(
		match.Table, core.ReadRewrite{Filter: []core.FilterStep{step}}).Display
	tab := connection.OpenTableInNewTab(match.Table, preview)
	tab.Filter = []core.FilterStep{step}
	tab.Focus = app.PaneResult
	return model.runTabRead(connection, tab)
}
