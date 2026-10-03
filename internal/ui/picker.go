package ui

import (
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/secret"
)

type pickerState struct {
	// The row of the picker the cursor is on, and why the last attempt failed.
	cursor  int
	problem string
	// The profile a password is being typed for, or a connection is being opened on.
	pending cfg.Profile
	// The field a password is typed into.
	password *app.EditorBuffer
	// True where the user asked to keep the typed password in the keyring of the
	// operating system.
	keepInKeyring bool
	// True while the keyring box has the focus.
	keyringFocused bool
	// True where the typed password tests the connection form instead of opening a
	// connection.
	testsForm bool
	// Filter field. The card always draws it, and filtering is true while it has the
	// focus.
	filter    *app.EditorBuffer
	filtering bool
	// Group paths folded in the list.
	folded map[string]bool
}

// pickerRow is one row of the picker: a group header, or a profile.
type pickerRow struct {
	profile cfg.Profile
	// Header path, or empty for a profile row.
	group string
	depth int
	// Profiles in the group and in its subgroups.
	count int
}

func (row pickerRow) isGroup() bool { return row.group != "" }

// buildPickerRows returns the tree of the profiles. Each level lists its profiles first, in
// their order, then its subgroups by name.
func buildPickerRows(profiles []cfg.Profile, folded map[string]bool) []pickerRow {
	return appendPickerLevel(nil, profiles, "", 0, folded)
}

func appendPickerLevel(
	rows []pickerRow, profiles []cfg.Profile, path string, depth int, folded map[string]bool,
) []pickerRow {
	children := []string{}
	for _, profile := range profiles {
		if profile.Group == path {
			rows = append(rows, pickerRow{profile: profile, depth: depth})
			continue
		}
		if child, inside := findChildGroup(profile.Group, path); inside &&
			!slices.Contains(children, child) {
			children = append(children, child)
		}
	}
	slices.SortFunc(children, compareGroupNames)
	for _, child := range children {
		count := 0
		for _, profile := range profiles {
			if isInGroup(profile.Group, child) {
				count++
			}
		}
		rows = append(rows, pickerRow{group: child, depth: depth, count: count})
		if !folded[child] {
			rows = appendPickerLevel(rows, profiles, child, depth+1, folded)
		}
	}
	return rows
}

func compareGroupNames(left, right string) int {
	if order := strings.Compare(strings.ToLower(left), strings.ToLower(right)); order != 0 {
		return order
	}
	return strings.Compare(left, right)
}

// findChildGroup returns the path one level below parent on the way to group.
func findChildGroup(group, parent string) (string, bool) {
	rest := group
	if parent != "" {
		var below bool
		if rest, below = strings.CutPrefix(group, parent+"/"); !below {
			return "", false
		}
	}
	if rest == "" {
		return "", false
	}
	level, _, _ := strings.Cut(rest, "/")
	if parent == "" {
		return level, true
	}
	return parent + "/" + level, true
}

// isInGroup is true for a group path equal to path or below it.
func isInGroup(group, path string) bool {
	return group == path || strings.HasPrefix(group, path+"/")
}

// findParentGroup returns the path one level up, or empty for a top-level group.
func findParentGroup(group string) string {
	at := strings.LastIndex(group, "/")
	if at < 0 {
		return ""
	}
	return group[:at]
}

func (picker *pickerState) setFolded(group string, folded bool) {
	if picker.folded == nil {
		picker.folded = map[string]bool{}
	}
	if folded {
		picker.folded[group] = true
		return
	}
	delete(picker.folded, group)
}

// unfoldPath unfolds every group from the top level down to group.
func (picker *pickerState) unfoldPath(group string) {
	for path := group; path != ""; path = findParentGroup(path) {
		picker.setFolded(path, false)
	}
}

// filtersList is true while the filter field has the focus.
func (picker *pickerState) filtersList() bool { return picker.filtering }

// startFilter focuses the field and selects the first row.
func (picker *pickerState) startFilter() {
	picker.filtering, picker.cursor = true, 0
}

// stopFilter unfocuses the field and keeps the filter.
func (picker *pickerState) stopFilter() {
	picker.filtering = false
}

// clearFilter empties the field and unfocuses it.
func (picker *pickerState) clearFilter() {
	picker.filtering, picker.cursor = false, 0
	if picker.filter != nil {
		picker.filter.SetText("")
	}
}

// readFilterTerm returns the filter text.
func (picker *pickerState) readFilterTerm() string {
	if picker.filter == nil {
		return ""
	}
	return strings.TrimSpace(picker.filter.Text)
}

// keepFilteredProfiles returns the profiles that match the filter. An empty filter keeps
// every profile.
func (picker *pickerState) keepFilteredProfiles(profiles []cfg.Profile) []cfg.Profile {
	return keepMatchingRows(profiles, picker.readFilterTerm(), describeProfileRow)
}

// describeProfileRow returns the row text the filter matches.
func describeProfileRow(profile cfg.Profile) string {
	return profile.Name + " " + profile.Group + " " + string(profile.Environment) + " " +
		string(profile.Engine) + " " + cfg.DescribeProfileTarget(profile)
}

func (picker *pickerState) step(by, count int) {
	picker.cursor = wrap(picker.cursor+by, count)
}

func (picker *pickerState) page(by, count int) {
	picker.cursor = clamp(picker.cursor+by, count)
}

func (picker *pickerState) focus(index, count int) {
	picker.cursor = clamp(index, count)
}

// askPassword opens the field for a profile. A profile that already reads the keyring keeps
// the box ticked, because the keyring is where its password belongs.
func (picker *pickerState) askPassword(profile cfg.Profile) {
	picker.pending, picker.password = profile, app.NewEditorBuffer("", 0)
	picker.keepInKeyring = profile.Auth == cfg.AuthKeyring && secret.IsAvailable()
	picker.keyringFocused = false
	picker.testsForm = false
}

// askPasswordForFormTest opens the field for the profile the form describes. A test uses
// the typed password and stores nothing.
func (picker *pickerState) askPasswordForFormTest(profile cfg.Profile) {
	picker.askPassword(profile)
	picker.testsForm = true
}

// offersKeyring is true where the card draws the box that keeps the password. A test of the
// form keeps no password.
func (picker *pickerState) offersKeyring() bool {
	return secret.IsAvailable() && !picker.testsForm
}

func (picker *pickerState) waitsFor(name string) bool {
	return picker.pending.Name == name
}
