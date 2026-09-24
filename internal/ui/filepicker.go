package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// fileRow is one row of a file picker: the parent directory, a directory, or a file.
type fileRow struct {
	name      string
	directory bool
	parent    bool
	size      int64
}

// filePicker lists one directory: the parent, the directories, and the files of the allowed
// extensions. Hidden entries are left out.
type filePicker struct {
	id int64
	// Directory is the directory the picker lists.
	Directory string
	// Extensions are the file extensions offered. Empty offers every file.
	Extensions []string
	rows       []fileRow
	cursor     int
	offset     int
	problem    string
	// The name the cursor goes to when the listing arrives.
	lands string
}

// fileListMsg is the listing of one directory, read away from the loop.
type fileListMsg struct {
	id        int64
	directory string
	rows      []fileRow
	problem   string
}

var lastFilePickerID atomic.Int64

// newFilePicker returns a picker of that directory.
func newFilePicker(directory string, extensions []string) filePicker {
	return filePicker{
		id: lastFilePickerID.Add(1), Directory: directory, Extensions: extensions,
	}
}

// Init returns the command that reads the directory.
func (picker filePicker) Init() tea.Cmd {
	return picker.readDirectory()
}

// readDirectory returns the command that lists the directory of the picker.
func (picker filePicker) readDirectory() tea.Cmd {
	id, directory, extensions := picker.id, picker.Directory, picker.Extensions
	return func() tea.Msg {
		rows, err := listFileRows(directory, extensions)
		listed := fileListMsg{id: id, directory: directory, rows: rows}
		if err != nil {
			listed.problem = err.Error()
		}
		return listed
	}
}

// listFileRows returns the rows of a directory: the parent first, then the directories, then
// the files, each group by name.
func listFileRows(directory string, extensions []string) ([]fileRow, error) {
	entries, err := os.ReadDir(directory)
	rows := []fileRow{}
	if filepath.Dir(directory) != directory {
		rows = append(rows, fileRow{name: "..", directory: true, parent: true})
	}
	if err != nil {
		return rows, err
	}
	directories, files := []fileRow{}, []fileRow{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		// A link is listed as what it points at.
		info, statErr := os.Stat(filepath.Join(directory, name))
		if statErr != nil {
			continue
		}
		if info.IsDir() {
			directories = append(directories, fileRow{name: name, directory: true})
			continue
		}
		if matchesFileExtension(name, extensions) {
			files = append(files, fileRow{name: name, size: info.Size()})
		}
	}
	return append(append(rows, directories...), files...), nil
}

// matchesFileExtension is true for a name with one of the extensions, and for every name
// where there are none.
func matchesFileExtension(name string, extensions []string) bool {
	if len(extensions) == 0 {
		return true
	}
	lowered := strings.ToLower(name)
	return slices.ContainsFunc(extensions, func(extension string) bool {
		return strings.HasSuffix(lowered, strings.ToLower(extension))
	})
}

// Update applies a listing or a key press. It returns the path of a file the press chose,
// and an empty string where it chose none.
func (picker filePicker) Update(message tea.Msg) (filePicker, tea.Cmd, string) {
	switch held := message.(type) {
	case fileListMsg:
		if held.id != picker.id || held.directory != picker.Directory {
			return picker, nil, ""
		}
		picker.rows, picker.problem = held.rows, held.problem
		picker.cursor, picker.offset = 0, 0
		for at, row := range picker.rows {
			if row.name == picker.lands && !row.parent {
				picker.cursor = at
			}
		}
		picker.lands = ""
		picker.scrollToCursor()
		return picker, nil, ""
	case tea.KeyPressMsg:
		return picker.readKey(tea.Key(held))
	}
	return picker, nil, ""
}

// readKey moves the cursor, opens a directory, or chooses a file.
func (picker filePicker) readKey(key tea.Key) (filePicker, tea.Cmd, string) {
	last := len(picker.rows) - 1
	switch {
	case key.Code == tea.KeyUp || key.Text == "k" || isCtrlKey(key, 'p'):
		picker.cursor--
	case key.Code == tea.KeyDown || key.Text == "j" || isCtrlKey(key, 'n'):
		picker.cursor++
	case key.Code == tea.KeyPgUp || key.Text == "K":
		picker.cursor -= pickerRows
	case key.Code == tea.KeyPgDown || key.Text == "J":
		picker.cursor += pickerRows
	case key.Code == tea.KeyHome || key.Text == "g":
		picker.cursor = 0
	case key.Code == tea.KeyEnd || key.Text == "G":
		picker.cursor = last
	case key.Code == tea.KeyLeft || key.Code == tea.KeyBackspace || key.Text == "h":
		return picker.leaveDirectory()
	case key.Code == tea.KeyRight || key.Text == "l":
		if row, found := picker.findCursorRow(); found && row.directory {
			return picker.openRow(row)
		}
	case key.Code == tea.KeyEnter:
		if row, found := picker.findCursorRow(); found {
			return picker.openRow(row)
		}
	}
	picker.cursor = max(min(picker.cursor, last), 0)
	picker.scrollToCursor()
	return picker, nil, ""
}

// isCtrlKey is true for the press of that letter with Ctrl alone.
func isCtrlKey(key tea.Key, letter rune) bool {
	return key.Code == letter && key.Mod == uv.ModCtrl
}

// findCursorRow returns the row under the cursor.
func (picker filePicker) findCursorRow() (fileRow, bool) {
	if picker.cursor < 0 || picker.cursor >= len(picker.rows) {
		return fileRow{}, false
	}
	return picker.rows[picker.cursor], true
}

// openRow enters a directory, or returns the path of a file.
func (picker filePicker) openRow(row fileRow) (filePicker, tea.Cmd, string) {
	if row.parent {
		return picker.leaveDirectory()
	}
	path := filepath.Join(picker.Directory, row.name)
	if !row.directory {
		return picker, nil, path
	}
	picker.Directory = path
	return picker, picker.readDirectory(), ""
}

// leaveDirectory lists the parent directory, with the cursor on the directory it left.
func (picker filePicker) leaveDirectory() (filePicker, tea.Cmd, string) {
	parent := filepath.Dir(picker.Directory)
	if parent == picker.Directory {
		return picker, nil, ""
	}
	picker.lands = filepath.Base(picker.Directory)
	picker.Directory = parent
	return picker, picker.readDirectory(), ""
}

// scrollToCursor keeps the cursor inside the rows the picker draws.
func (picker *filePicker) scrollToCursor() {
	if picker.cursor < picker.offset {
		picker.offset = picker.cursor
	}
	if picker.cursor >= picker.offset+pickerRows {
		picker.offset = picker.cursor - pickerRows + 1
	}
	picker.offset = max(picker.offset, 0)
}
