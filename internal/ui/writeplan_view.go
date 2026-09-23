package ui

import (
	"image/color"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/writeplan"
)

// The write-plan dialog.

const (
	// writePlanLabelWidth is the column the values of the plan start in.
	writePlanLabelWidth = 10
	writePlanMeterWidth = 12
	// writePlanShareOfConcern is the share of a relation above which the count is drawn as
	// a fault rather than as a number.
	writePlanShareOfConcern = 0.25
	// writePlanStatementRows is how many rows of the statement the card draws. The rest is
	// counted, so the answers stay on the card whatever the statement is.
	writePlanStatementRows = 8
)

// renderWritePlan draws the plan and its buttons. A write that a foreign key blocks opens
// with a headline.
func (model *Model) renderWritePlan(overlay app.Overlay, width int) string {
	inner := max(width-present.CardChrome, 1)
	plan := overlay.Plan

	lines := []string{}
	if len(plan.Blockers) > 0 {
		lines = append(lines, model.renderWritePlanHeadline(plan, inner)...)
		lines = append(lines, "")
	}
	lines = append(lines, model.renderWritePlanStatement(plan.SQL, inner)...)
	lines = append(lines, "")
	lines = append(lines, model.renderWritePlanLines(plan, inner)...)
	lines = append(lines, "")

	model.recordCardBody()
	lines = append(lines, model.renderButtonRow(
		model.buildWritePlanButtons(plan), cardBodyRow+len(lines), cardBodyColumn))
	card := model.renderNotedTextCard(overlay.Kind, overlay.Title,
		model.renderActiveEnvironmentBadge(), width, lines, nil, len(lines), destructiveCard)
	model.rememberCardKeys(model.buildCardKeys(app.OverlayWritePlan, keyScene{overlay: overlay}))
	return card
}

// buildWritePlanButtons returns the buttons of the plan. A blocked write leads with the
// button that opens the blocking rows, and run is a secondary button.
func (model *Model) buildWritePlanButtons(plan writeplan.Plan) []cardButton {
	scene := keyScene{overlay: app.Overlay{Plan: plan}}
	run := model.buildCardButton(cfg.ScopeDialog, ActionAnswerYes, describeWritePlanRun(scene))
	cancel := model.buildCardButton(cfg.ScopeDialog, ActionClose, "cancel")
	if len(plan.Blockers) == 0 {
		run.primary, run.destructive = true, true
		return []cardButton{run, cancel}
	}
	if !opensBlockingRows(scene) {
		cancel.primary = true
		return []cardButton{run, cancel}
	}
	show := model.buildCardButton(cfg.ScopeList, ActionChooseRow, "show the blocking rows")
	show.primary = true
	return []cardButton{show, run, cancel}
}

// findOpenableBlocker returns the first blocker whose referencing rows a filter can match.
func findOpenableBlocker(plan writeplan.Plan) (writeplan.Cascade, bool) {
	for _, blocker := range plan.Blockers {
		if blocker.Referencing != "" {
			return blocker, true
		}
	}
	return writeplan.Cascade{}, false
}

// opensBlockingRows is true for a plan with a blocker whose rows a filter can match.
func opensBlockingRows(scene keyScene) bool {
	_, found := findOpenableBlocker(scene.overlay.Plan)
	return found
}

// describeWritePlanRun returns the label of the key that runs the write.
func describeWritePlanRun(scene keyScene) string {
	if len(scene.overlay.Plan.Blockers) > 0 {
		return "run anyway"
	}
	return "run"
}

// renderWritePlanHeadline draws the sentence that the write fails, and one line per table
// that blocks it.
func (model *Model) renderWritePlanHeadline(plan writeplan.Plan, inner int) []string {
	theme := model.styles.Theme
	outcome := " may fail"
	if slices.ContainsFunc(plan.Blockers, func(blocker writeplan.Cascade) bool {
		return blocker.HasRows
	}) {
		outcome = " will fail"
	}
	headline := model.writeProblemSign() + "This " + string(plan.Kind) + outcome
	lines := []string{padStyledOn(paintBoldText(theme.Error, theme.Panel,
		present.TruncateText(headline, inner)), inner, theme.Panel)}
	for _, blocker := range plan.Blockers {
		for _, line := range present.WrapWords(writeplan.DescribeBlockingRows(blocker), inner-2) {
			lines = append(lines, padStyledOn(
				paintText(theme.Text, theme.Panel, "  "+line), inner, theme.Panel))
		}
	}
	return lines
}

// renderWritePlanStatement draws the write, wrapped so the predicate is read in full.
func (model *Model) renderWritePlanStatement(sql string, inner int) []string {
	theme := model.styles.Theme
	wrapped := []string{}
	for line := range strings.SplitSeq(present.SafeLines(sql), "\n") {
		wrapped = append(wrapped, present.WrapWords(line, inner)...)
	}

	// The colours are read off the text as drawn, so a wrapped line keeps them.
	highlights := buildSQLLineHighlights(strings.Join(wrapped, "\n"))
	shown := min(len(wrapped), writePlanStatementRows)

	lines := make([]string, 0, shown+1)
	for at := range shown {
		lines = append(lines, model.renderCodeLineOn(theme.Panel, codeLine{
			text: wrapped[at], spans: highlights[at], width: inner}))
	}
	if len(wrapped) > shown {
		lines = append(lines, padStyledOn(paintText(theme.Muted, theme.Panel,
			"… and "+present.FormatCountOf(int64(len(wrapped)-shown), "more line", "more lines")),
			inner, theme.Panel))
	}
	return lines
}

// writePlanLine is one line of the plan. A line that follows another of the same label
// leaves the label out.
type writePlanLine struct {
	label string
	value string
	ink   color.Color
	// Drawn after the value, already styled.
	trailer string
}

func (model *Model) renderWritePlanLines(plan writeplan.Plan, inner int) []string {
	rows := []writePlanLine{model.buildWritePlanRowsLine(plan)}
	if columns, named := writeplan.DescribeColumns(plan); named {
		rows = append(rows, writePlanLine{
			label: writeplan.LabelColumns, value: columns, ink: model.styles.Theme.Text,
		})
	}
	rows = append(rows, model.buildWritePlanCascadeLines(plan)...)
	rows = append(rows, model.buildWritePlanUndoLine(plan), writePlanLine{
		label: writeplan.LabelCommit, value: writeplan.DescribeCommit(plan),
		ink: model.styles.Theme.Muted,
	})

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, model.renderWritePlanLine(row, inner))
	}
	return lines
}

func (model *Model) buildWritePlanRowsLine(plan writeplan.Plan) writePlanLine {
	theme := model.styles.Theme
	row := writePlanLine{
		label: writeplan.LabelRows, value: writeplan.DescribeRows(plan), ink: theme.Text,
	}
	if !plan.HasRows {
		row.ink = theme.Warning
		return row
	}

	share, held := plan.ReadShare()
	if !held {
		return row
	}
	if plan.NamesEveryRow() || share >= writePlanShareOfConcern {
		row.ink = theme.Error
	}
	row.trailer = paintText(row.ink, theme.Panel,
		"  "+present.BuildMeter(share, 1, writePlanMeterWidth))
	return row
}

func (model *Model) buildWritePlanCascadeLines(plan writeplan.Plan) []writePlanLine {
	rows := make([]writePlanLine, 0, len(plan.Cascades))
	for at, cascade := range plan.Cascades {
		label := writeplan.LabelCascades
		if at > 0 {
			label = ""
		}
		rows = append(rows, writePlanLine{
			label: label, value: writeplan.DescribeCascade(cascade),
			ink: model.styles.Theme.Warning,
		})
	}
	return rows
}

func (model *Model) buildWritePlanUndoLine(plan writeplan.Plan) writePlanLine {
	theme := model.styles.Theme
	row := writePlanLine{
		label: writeplan.LabelUndo, value: writeplan.DescribeUndo(plan.Undo), ink: theme.Text,
	}
	if !plan.Undo.Kept {
		row.ink = theme.Muted
		return row
	}
	row.trailer = paintText(theme.Muted, theme.Panel, "  "+
		model.registry.FormatActionChords(cfg.ScopeGlobal, ActionUndoWrite)+
		" undoes this write after execution")
	return row
}

func (model *Model) renderWritePlanLine(row writePlanLine, inner int) string {
	theme := model.styles.Theme
	label := present.FitText(row.label, writePlanLabelWidth)
	room := max(inner-writePlanLabelWidth-measureStyledWidth(row.trailer), 1)

	written := paintText(theme.Muted, theme.Panel, label)
	written += paintText(row.ink, theme.Panel, present.TruncateText(row.value, room))
	return padStyledOn(written+row.trailer, inner, theme.Panel)
}
