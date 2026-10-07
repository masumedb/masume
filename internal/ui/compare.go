package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/schemadiff"
)

// compareChoiceKeys are the letters of the targets of the compare card.
const compareChoiceKeys = "abcdefghijklmnopqrstuvwxyz"

// openCompareChoice opens the card of compare targets for one schema: the other schemas of
// this connection, and the other open connections.
func (model *Model) openCompareChoice(
	connection *app.Connection, source string,
) (tea.Model, tea.Cmd) {
	targets := model.listCompareTargets(connection, source)
	if len(targets) == 0 {
		connection.Show("no other schema and no other connection to compare with")
		return model, nil
	}
	choices := make([]app.Choice, 0, len(targets))
	for at, target := range targets {
		label, detail := target.TargetSchema, "this connection"
		if target.TargetProfile != "" {
			label, detail = target.TargetProfile, "schema "+target.TargetSchema
		}
		choices = append(choices, app.Choice{
			Key:        compareChoiceKeys[at : at+1],
			MenuAction: app.MenuAction{ID: fmt.Sprint(at), Label: label, Detail: detail},
		})
	}
	connection.Open(app.Overlay{
		Kind: app.OverlayChoice, Title: " compare " + source + " with ",
		Body:    "+ is only in the target, - only in " + source + ", ~ in both and different.",
		Choices: choices,
		Answers: app.OverlayAnswers{ID: func(chosen string) app.AnswerCommand {
			at := slices.IndexFunc(choices, func(choice app.Choice) bool { return choice.ID == chosen })
			if at < 0 {
				return nil
			}
			tab := connection.OpenCompare(targets[at])
			tab.Focus = app.PaneResult
			_, command := model.readCompare(connection, tab)
			return carryAnswer(command)
		}},
	})
	return model, nil
}

// listCompareTargets returns the other schemas of the connection, then each other open
// connection with the schema of the same name, or its default schema.
func (model *Model) listCompareTargets(
	connection *app.Connection, source string,
) []app.SchemaCompare {
	targets := []app.SchemaCompare{}
	for _, schema := range connection.Catalog.Schemas {
		if schema != source {
			targets = append(targets, app.SchemaCompare{SourceSchema: source, TargetSchema: schema})
		}
	}
	for _, other := range model.connections.all() {
		if other == connection {
			continue
		}
		schema := other.Session.Describe().DefaultSchema
		if slices.Contains(other.Catalog.Schemas, source) {
			schema = source
		}
		targets = append(targets, app.SchemaCompare{
			SourceSchema: source, TargetProfile: other.Profile().Name, TargetSchema: schema,
		})
	}
	return targets[:min(len(targets), len(compareChoiceKeys))]
}

// readCompare reads both schemas of a compare tab. The target connection must be open.
func (model *Model) readCompare(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	tab.View = app.ViewDiff
	compare := *tab.Compare
	target := connection.Session
	if compare.TargetProfile != "" {
		other, found := model.findConnectionByProfile(compare.TargetProfile)
		if !found {
			tab.ViewData = app.PaneContent{
				Kind: app.DataIdle, Reason: "connect to " + compare.TargetProfile + " to compare",
			}
			return model, nil
		}
		target = other.Session
	}
	tab.ViewData = app.PaneContent{Kind: app.DataLoading, StartedAt: time.Now()}
	return model, readSchemaCompare(
		model.connections.idOf(connection), tab.ID, connection.Session, target, compare)
}

// findConnectionByProfile returns the open connection of that profile.
func (model *Model) findConnectionByProfile(name string) (*app.Connection, bool) {
	for _, held := range model.connections.all() {
		if held.Profile().Name == name {
			return held, true
		}
	}
	return nil, false
}

// readSchemaCompare reads both schemas and compares them.
func readSchemaCompare(
	connectionID, tabID int, source, target db.CatalogReader, compare app.SchemaCompare,
) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()
		answered := relationViewMsg{ConnectionID: connectionID, TabID: tabID, View: app.ViewDiff}
		from, err := schemadiff.ReadSnapshot(ctx, source, compare.SourceSchema)
		if err == nil {
			var to schemadiff.Snapshot
			if to, err = schemadiff.ReadSnapshot(ctx, target, compare.TargetSchema); err == nil {
				answered.Content = app.PaneContent{
					Kind: app.DataDiff, Differences: schemadiff.Compare(from, to),
				}
				return answered
			}
		}
		answered.Content = app.PaneContent{Kind: app.DataFailed, Message: db.DescribeError(err)}
		return answered
	}
}

// diffLine is one line of the compare view and its ink.
type diffLine struct {
	text   string
	change schemadiff.Change
	header bool
}

// buildDiffLines lays out the differences: a line per table, a line per item, and the source
// and target definitions of a changed item on lines of their own.
func buildDiffLines(found []schemadiff.Difference) []diffLine {
	lines := []diffLine{}
	table := ""
	for _, difference := range found {
		if difference.Table != table {
			if table != "" {
				lines = append(lines, diffLine{})
			}
			table = difference.Table
			lines = append(lines, diffLine{text: table, header: true})
		}
		if difference.Change != schemadiff.ChangeAltered || difference.Part == schemadiff.PartTable {
			lines = append(lines, diffLine{
				text: "  " + schemadiff.DescribeDifference(difference), change: difference.Change,
			})
			continue
		}
		lines = append(lines,
			diffLine{text: "  ~ " + string(difference.Part) + " " + difference.Name,
				change: schemadiff.ChangeAltered},
			diffLine{text: "      - " + difference.Source, change: schemadiff.ChangeRemoved},
			diffLine{text: "      + " + difference.Target, change: schemadiff.ChangeAdded})
	}
	return lines
}

// renderDiff draws the differences of a compare tab.
func (model *Model) renderDiff(
	tab *app.Tab, found []schemadiff.Difference, width, height int,
) []string {
	theme := model.styles.Theme
	if len(found) == 0 {
		return model.renderEmptyState(width, height, "no differences", nil)
	}
	held := buildDiffLines(found)
	tab.DetailOffset = clampOffset(tab.DetailOffset, height, len(held))
	lines := make([]string, 0, height)
	for at := tab.DetailOffset; at < len(held) && len(lines) < height; at++ {
		line := held[at]
		ink := theme.Text
		switch {
		case line.header:
			ink = theme.Accent
		case line.change == schemadiff.ChangeAdded:
			ink = theme.Success
		case line.change == schemadiff.ChangeRemoved:
			ink = theme.Error
		case line.change == schemadiff.ChangeAltered:
			ink = theme.Warning
		}
		lines = append(lines, paintText(ink, theme.Panel,
			present.FitText(" "+present.TruncateText(line.text, max(width-2, 1)), width)))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return model.drawScrollTrack(lines, scrollView{
		offset: tab.DetailOffset, rows: height, total: len(held),
		moveTo: func(offset int) tea.Cmd { tab.DetailOffset = offset; return nil },
	}, model.layout.detailTop, model.editorLeft+1, width, theme.Panel)
}
