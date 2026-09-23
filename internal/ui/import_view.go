package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/load"
	"github.com/masumedb/masume/internal/present"
)

// The import dialog contains file options, column mappings, and a review before writing.

// importLabelWidth is the width of the label of one row.
const importLabelWidth = 30

// importedProblemRows is how many refused rows the review lists. The rest are counted.
const importedProblemRows = 5

// renderImport draws the card of an import: the picker, the form, or the review.
func (model *Model) renderImport(overlay app.Overlay, width int) string {
	switch overlay.Import.Stage {
	case app.ImportPick:
		return model.renderImportPicker(overlay, width)
	case app.ImportReview:
		return model.renderImportReview(overlay, width)
	}
	return model.renderImportForm(overlay, width)
}

// renderImportPicker draws the directory the file is chosen out of.
func (model *Model) renderImportPicker(overlay app.Overlay, width int) string {
	inner := width - present.CardChrome
	lines := model.renderFilePicker(model.ActiveID(), inner)
	if overlay.Notice != "" {
		lines = append(lines, "", model.styles.Error().Render(
			present.TruncateText(overlay.Notice, inner)))
	}

	choose := model.buildCardButton(cfg.ScopeList, ActionChooseRow, "choose")
	choose.primary = true
	return model.renderImportCard(overlay, width, lines, []cardButton{
		choose, model.buildCardButton(cfg.ScopeDialog, ActionClose, "cancel"),
	})
}

// renderImportCard draws a stage of the import with its buttons under the lines.
func (model *Model) renderImportCard(
	overlay app.Overlay, width int, lines []string, buttons []cardButton,
) string {
	model.recordCardBody()
	lines = append(lines, "")
	lines = append(lines, model.renderButtonRow(buttons, cardBodyRow+len(lines), cardBodyColumn))
	model.rememberCardKeys(model.buildCardKeys(app.OverlayImport, keyScene{overlay: overlay}))
	return model.renderNotedCard(buildImportTitle(overlay.Import),
		model.renderActiveEnvironmentBadge(), width, lines, plainCard)
}

// renderImportForm draws one row per setting and one row per column of the file.
func (model *Model) renderImportForm(overlay app.Overlay, width int) string {
	fields := BuildImportFields(overlay)
	valueWidth := max(width-present.CardChrome-importLabelWidth, 8)

	model.layout.formChoices = nil
	sourceFields := len(fields)
	for at, field := range fields {
		if strings.HasPrefix(field.Key, mappingKeyPrefix) {
			sourceFields = at
			break
		}
	}
	lines := make([]string, 0, len(fields)+5)
	lines = append(lines, model.renderFieldHeading("source"))
	for at, field := range fields {
		if at == sourceFields {
			lines = append(lines, model.renderFieldHeading("columns"))
		}
		focused := at == overlay.Field
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}

		value := field.Value
		written := model.styles.Muted().Render(
			present.TruncateText(describeFieldValue(field), valueWidth))
		switch {
		case len(field.Choices) > 0:
			written = model.renderChoiceField(value, valueWidth, at,
				cardBodyRow+len(lines), cardBodyColumn+importLabelWidth, focused)
		case focused:
			written = model.renderField(
				app.NewEditorBuffer(value, len(value)), valueWidth, FieldLook{
					Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
					Focused: true, Placeholder: field.Label,
				})
		}
		lines = append(lines, labelStyle.Render(marker+
			fitFieldLabel(field.Label, importLabelWidth-present.MeasureText(marker)))+written)
	}

	// The problem line is always counted, so the card keeps its height. A card that is
	// writing draws how far it has come on that line instead.
	text := FindImportProblem(overlay, model.readActiveDialect())
	if text == "" {
		text = overlay.Notice
	}
	line := model.styles.Error().Render(present.TruncateText(text, width-4))
	if overlay.Import.Running && overlay.Import.Progress.IsStarted() {
		line = model.renderProgress(overlay.Import.Progress, width-4)
	}
	lines = append(lines, line)

	count, gap := len(fields), 0
	if sourceFields < len(fields) {
		count, gap = len(fields)+1, sourceFields
	}
	model.layout.formRows = rowsHit{
		top: cardBodyRow + 1, count: count, gap: gap,
		from: cardBodyColumn - 1, to: cardBodyColumn + width - 4,
	}
	advance := model.buildCardButton(cfg.ScopeDialog, ActionSaveForm,
		describeImportAdvance(overlay))
	advance.primary = true
	return model.renderImportCard(overlay, width, lines, []cardButton{
		advance, model.buildCardButton(cfg.ScopeDialog, ActionClose, "cancel"),
	})
}

// describeImportStep describes what Enter does on the row under the cursor. The row that
// holds the path opens the file picker again, whatever stage the form stands at.
func describeImportStep(overlay app.Overlay) string {
	if !overlay.Import.Running && readFieldKey(overlay) == "path" {
		return "choose another file"
	}
	return describeImportAdvance(overlay)
}

// describeImportAdvance describes the next stage of the form.
func describeImportAdvance(overlay app.Overlay) string {
	held := overlay.Import
	if held.Running {
		return "reading…"
	}
	if held.Stage == app.ImportFile {
		return "read the file"
	}
	return "review"
}

// buildImportTitle names the card: the file being read, and the rows it holds once they
// have been counted.
func buildImportTitle(held app.ImportRequest) string {
	if held.Plan.Path == "" {
		return " import "
	}
	title := " import " + present.TruncateText(filepath.Base(held.Plan.Path), 40)
	if held.Plan.CreatesTable {
		title += " · new table"
	}
	if held.Report.Rows > 0 {
		title += " · " + present.FormatRowCount(int64(held.Report.Rows))
	} else if len(held.Plan.Sample.Rows) > 0 && held.Plan.Sample.More {
		title += " · " + strconv.Itoa(len(held.Plan.Sample.Rows)) + "+ rows"
	}
	return title + " "
}

// renderImportReview draws what the import would do: the rows it would write, the rows it
// cannot, and the SQL that would run.
func (model *Model) renderImportReview(overlay app.Overlay, width int) string {
	held := overlay.Import
	inner := width - present.CardChrome

	lines := []string{
		model.styles.Ink().Render(present.TruncateText(DescribeImportSummary(held), inner)),
		model.styles.Muted().Render(present.TruncateText(
			held.Plan.Path+" → "+describeImportTable(held), inner)),
		"",
	}

	if held.Running && held.Progress.IsStarted() {
		lines = append(lines,
			model.renderProgress(held.Progress, inner), "")
	}
	if held.Report.Refused > 0 {
		lines = append(lines, model.styles.Error().Render(present.TruncateText(
			present.FormatRowCount(int64(held.Report.Refused))+
				" will be skipped:", inner)))
		for at, problem := range held.Report.Problems {
			if at >= importedProblemRows {
				break
			}
			lines = append(lines, model.styles.Muted().Render(present.TruncateText(
				"  line "+strconv.Itoa(problem.Line)+"  "+
					describeRowProblem(problem), inner)))
		}
		lines = append(lines, "")
	}

	for _, statement := range held.Statements {
		for line := range strings.SplitSeq(statement, "\n") {
			lines = append(lines, model.styles.Faint().Render(
				present.TruncateText(line, inner)))
		}
		lines = append(lines, "")
	}

	if overlay.Notice != "" {
		lines = append(lines, model.styles.Error().Render(
			present.TruncateText(overlay.Notice, inner)), "")
	}

	write := model.buildCardButton(cfg.ScopeDialog, ActionSaveForm, describeImportRun(held))
	write.primary = true
	return model.renderImportCard(overlay, width, lines[:len(lines)-1], []cardButton{
		write, model.buildCardButton(cfg.ScopeDialog, ActionStepBack, "back to the form"),
	})
}

// describeRowProblem returns one refused row as the review lists it.
func describeRowProblem(problem load.RowProblem) string {
	if problem.Column == "" {
		return problem.Reason
	}
	return problem.Column + ": " + problem.Reason
}

// describeImportRun describes what running the import writes.
func describeImportRun(held app.ImportRequest) string {
	if held.Running {
		return "writing…"
	}
	written := held.Report.Rows - held.Report.Refused
	return "write " + present.FormatRowCount(int64(written))
}
