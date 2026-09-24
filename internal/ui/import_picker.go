package ui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/present"
)

// The import file picker uses the current directory and theme.

// fileSizeWidth is the room the size of a file takes, which is enough for `1.1GB`.
const fileSizeWidth = 7

// pickerRows is how many rows of files the picker draws.
const pickerRows = 12

// buildFilePicker returns a picker opened in the directory the client was started in,
// offering the files of those extensions.
func (model *Model) buildFilePicker(extensions []string) filePicker {
	directory, err := os.Getwd()
	if err != nil {
		directory = "."
	}
	return newFilePicker(directory, extensions)
}

// openFilePicker gives this connection a picker of those files, and returns the command that
// reads the directory it opens in.
func (model *Model) openFilePicker(connectionID int, extensions []string) tea.Cmd {
	if model.filePickers == nil {
		model.filePickers = map[int]*filePicker{}
	}
	picker := model.buildFilePicker(extensions)
	model.filePickers[connectionID] = &picker
	return picker.Init()
}

// findFilePicker returns the picker open on this connection, and nothing where no card is
// picking a file.
func (model *Model) findFilePicker(connectionID int) *filePicker {
	return model.filePickers[connectionID]
}

// picksFile is true while the card of this connection is picking a file.
func picksFile(overlay app.Overlay) bool {
	return (overlay.Kind == app.OverlayImport && overlay.Import.Stage == app.ImportPick) ||
		(overlay.Kind == app.OverlayDump && overlay.Dump.Stage == app.DumpPick)
}

// readPickerMessage hands a message to the picker of the card that is picking a file, which
// reads a directory with a command of its own.
func (model *Model) readPickerMessage(message tea.Msg) (tea.Model, tea.Cmd, bool) {
	connection, id := model.Active(), model.ActiveID()
	if connection == nil || !picksFile(connection.Overlay) {
		return model, nil, false
	}
	picker := model.findFilePicker(id)
	if picker == nil {
		return model, nil, false
	}

	held, command, path := picker.Update(message)
	*picker = held
	if path == "" {
		return model, command, true
	}
	if connection.Overlay.Kind == app.OverlayDump {
		model.readRestoreFile(connection, path)
		return model, nil, true
	}
	return model.readPickedFile(connection, id, path)
}

// readPickedFile takes the file the user picked into the form, and reads it at once.
func (model *Model) readPickedFile(
	connection *app.Connection, connectionID int, path string,
) (tea.Model, tea.Cmd, bool) {
	overlay := &connection.Overlay
	overlay.Import.Plan.Path = path
	overlay.Import.Stage = app.ImportFile
	overlay.Notice = ""
	ApplyImportPath(overlay)
	// The row is written to directly, because the step of a cursor writes what the row it
	// leaves held and this row was never typed into.
	overlay.Field = importFileField
	overlay.Draft = app.NewEditorBuffer(path, len(path))

	overlay.Import.Running = true
	return model, readImportFile(connectionID, connection.Session, overlay.Import.Plan), true
}

// renderFilePicker draws the picker of the import, or the reason there is none to draw.
func (model *Model) renderFilePicker(connectionID int, width int) []string {
	picker := model.findFilePicker(connectionID)
	if picker == nil {
		return []string{model.styles.Muted().Render("the picker is not open")}
	}

	return model.buildPickerLines(picker, width)
}

// buildPickerLines draws the directory a picker stands in and its rows.
func (model *Model) buildPickerLines(picker *filePicker, width int) []string {
	theme := model.styles.Theme
	lines := []string{
		model.styles.Muted().Render(present.TruncatePath(picker.Directory, width)),
		"",
	}
	cursor := model.icons.Icon(cfg.IconField)
	blank := strings.Repeat(" ", present.MeasureText(cursor))
	for at := picker.offset; at < len(picker.rows) && at < picker.offset+pickerRows; at++ {
		row := picker.rows[at]
		size, name, ink := "", row.name, theme.Text
		if row.directory {
			name, ink = name+"/", theme.Accent
		} else {
			size = present.FormatFileSize(row.size)
		}
		text := present.FitTextRight(size, fileSizeWidth) + " " + name
		if at == picker.cursor {
			lines = append(lines, truncateStyled(paintText(theme.Accent, nil, cursor)+
				paintText(theme.OnAccent, theme.Accent, text), width))
			continue
		}
		lines = append(lines, truncateStyled(blank+
			paintText(theme.Muted, nil, present.FitTextRight(size, fileSizeWidth))+
			paintText(ink, nil, " "+name), width))
	}
	if picker.problem != "" {
		lines = append(lines, model.styles.Error().Render(present.TruncateText(picker.problem, width)))
	} else if len(picker.rows) == 0 || (len(picker.rows) == 1 && picker.rows[0].parent) {
		lines = append(lines, model.styles.Muted().Render("no file of that kind here"))
	}
	for len(lines) < pickerRows+2 {
		lines = append(lines, "")
	}
	return lines
}
