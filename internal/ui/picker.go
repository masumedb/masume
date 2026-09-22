package ui

import (
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
	// True where the typed password tests the connection form instead of opening a
	// connection.
	testsForm bool
	// Filter field. The card always draws it, and filtering is true while it has the
	// focus.
	filter    *app.EditorBuffer
	filtering bool
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
	return profile.Name + " " + string(profile.Environment) + " " +
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

func (picker *pickerState) pick(profiles []cfg.Profile) (cfg.Profile, bool) {
	if picker.cursor < 0 || picker.cursor >= len(profiles) {
		return cfg.Profile{}, false
	}
	return profiles[picker.cursor], true
}

// askPassword opens the field for a profile. A profile that already reads the keyring keeps
// the box ticked, because the keyring is where its password belongs.
func (picker *pickerState) askPassword(profile cfg.Profile) {
	picker.pending, picker.password = profile, app.NewEditorBuffer("", 0)
	picker.keepInKeyring = profile.Auth == cfg.AuthKeyring && secret.IsAvailable()
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
