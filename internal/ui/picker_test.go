package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
)

func TestThePickerStepsRoundTheEndsOfTheList(t *testing.T) {
	picker := pickerState{cursor: 2}
	picker.step(1, 3)
	if picker.cursor != 0 {
		t.Errorf("a step past the last row left the cursor on %d", picker.cursor)
	}
	picker.step(-1, 3)
	if picker.cursor != 2 {
		t.Errorf("a step before the first row left the cursor on %d", picker.cursor)
	}
}

func TestThePickerPagesStopAtTheEnds(t *testing.T) {
	picker := pickerState{cursor: 1}
	picker.page(10, 3)
	if picker.cursor != 2 {
		t.Errorf("a page past the last row left the cursor on %d", picker.cursor)
	}
	picker.page(-10, 3)
	if picker.cursor != 0 {
		t.Errorf("a page before the first row left the cursor on %d", picker.cursor)
	}
}

func TestThePickerStaysPutOnAnEmptyList(t *testing.T) {
	picker := pickerState{cursor: 4}
	picker.step(1, 0)
	if picker.cursor != 0 {
		t.Errorf("a step on no rows left the cursor on %d", picker.cursor)
	}
	picker.page(10, 0)
	if picker.cursor != 0 {
		t.Errorf("a page on no rows left the cursor on %d", picker.cursor)
	}
	picker.focus(3, 0)
	if picker.cursor != 0 {
		t.Errorf("a focus on no rows left the cursor on %d", picker.cursor)
	}
	if _, found := picker.pick(nil); found {
		t.Error("an empty list picked a profile")
	}
}

func TestThePickerPicksTheProfileTheCursorStandsOn(t *testing.T) {
	profiles := []cfg.Profile{{Name: "alpha"}, {Name: "beta"}}
	picker := pickerState{cursor: 1}
	held, found := picker.pick(profiles)
	if !found || held.Name != "beta" {
		t.Errorf("the cursor on 1 picked %+v, found=%v", held, found)
	}
	picker.cursor = 2
	if _, found := picker.pick(profiles); found {
		t.Error("a cursor off the list picked a profile")
	}
}

func TestThePickerAsksForAPasswordOnAFreshField(t *testing.T) {
	picker := pickerState{}
	picker.askPassword(cfg.Profile{Name: "first"})
	picker.password.SetText("secret")
	picker.askPassword(cfg.Profile{Name: "second"})
	if picker.pending.Name != "second" {
		t.Errorf("the pending profile is %q", picker.pending.Name)
	}
	if picker.password == nil || picker.password.Text != "" {
		t.Error("the password of the first profile was left in the field")
	}
	if !picker.waitsFor("second") || picker.waitsFor("first") {
		t.Error("the picker waits for a profile it is not opening")
	}
}

func TestThePickerKeysMoveTheCursor(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}

	model.runPickerAction(Match{Action: ActionCursorDown})
	if model.picker.cursor != 1 {
		t.Errorf("down left the cursor on %d", model.picker.cursor)
	}
	model.runPickerAction(Match{Action: ActionCursorLastRow})
	if model.picker.cursor != 2 {
		t.Errorf("end left the cursor on %d", model.picker.cursor)
	}
	model.runPickerAction(Match{Action: ActionCursorFirstRow})
	if model.picker.cursor != 0 {
		t.Errorf("home left the cursor on %d", model.picker.cursor)
	}
	model.runPickerAction(Match{Action: ActionCursorPageDown})
	if model.picker.cursor != 2 {
		t.Errorf("page down left the cursor on %d", model.picker.cursor)
	}
}

func TestChooseProfileAsksForAPasswordTheClientCannotFind(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	profile := cfg.Profile{
		Name: "shop", Engine: "postgres", User: "ada", Auth: cfg.AuthPrompt,
	}

	model.chooseProfile(profile)
	if model.screen != ScreenPromptingPassword {
		t.Errorf("the screen is %q, wanted the password prompt", model.screen)
	}
	if !model.picker.waitsFor("shop") {
		t.Error("the picker does not wait for the profile it asked a password for")
	}
	if model.picker.password == nil || model.picker.password.Text != "" {
		t.Error("the password field was not opened empty")
	}
}

// The list names the engine of every connection, so two profiles of one server are told
// apart before either one is opened.
func TestThePickerNamesTheEngineOfEveryConnection(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{
		{Name: "shop", Engine: core.EngineMysql, Host: "127.0.0.1", Port: 3306, User: "root"},
		{Name: "notes", Engine: core.EngineSqlite, Database: "/tmp/notes.db"},
	}

	drawn := stripEscapes(model.renderPicker())
	for _, wanted := range []string{"mysql", "sqlite"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the list does not name %q:\n%s", wanted, drawn)
		}
	}
}

// A card too narrow for the engine name beside the target drops the engine column, so the
// row still fits the width of the card.
func TestThePickerRowFitsTheNarrowCard(t *testing.T) {
	model := buildOfflineModel(t, 60, 30)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{
		{Name: "shop", Engine: core.EngineAuroraMysql, Host: "127.0.0.1", Port: 3306, User: "root"},
	}

	for _, line := range strings.Split(stripEscapes(model.renderPicker()), "\n") {
		if present.MeasureText(line) > 60 {
			t.Errorf("a row of %d columns does not fit the screen: %q",
				present.MeasureText(line), line)
		}
	}
}

// The filter keeps the matching profiles, and the letters reach the field instead of the
// actions of the card.
func TestThePickerFilterKeepsTheMatchingConnections(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{
		{Name: "alpha", Engine: core.EngineMysql},
		{Name: "beta", Engine: core.EnginePostgres},
		{Name: "tenant", Engine: core.EnginePostgres},
	}

	pressKey(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})
	if !model.picker.filtersList() {
		t.Fatal("the slash did not open the filter")
	}
	for _, key := range []tea.KeyPressMsg{
		{Code: 'n', Text: "n"}, {Code: 'a', Text: "a"},
	} {
		pressKey(t, model, key)
	}
	if model.screen != ScreenPickingProfile {
		t.Errorf("a typed letter left the screen on %q", model.screen)
	}
	if model.picker.filter.Text != "na" {
		t.Errorf("the field holds %q", model.picker.filter.Text)
	}

	shown := model.shownProfiles()
	if len(shown) != 1 || shown[0].Name != "tenant" {
		t.Errorf("the filter kept %+v", shown)
	}
	if profile, found := model.pickedProfile(); !found || profile.Name != "tenant" {
		t.Errorf("the cursor stands on %+v, found=%v", profile, found)
	}
}

// Escape unfocuses the field, keeps the filter and leaves the picker open. The letter keys
// run the actions again.
func TestEscapeStopsThePickerFilter(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{{Name: "alpha"}, {Name: "beta"}}

	pressKey(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})
	pressKey(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	if model.picker.filtersList() {
		t.Error("escape left the keyboard in the field")
	}
	if model.screen != ScreenPickingProfile {
		t.Errorf("escape left the screen on %q", model.screen)
	}
	if model.picker.readFilterTerm() != "b" {
		t.Errorf("escape left the term on %q", model.picker.readFilterTerm())
	}
	if len(model.shownProfiles()) != 1 {
		t.Errorf("the list holds %d rows", len(model.shownProfiles()))
	}

	pressKey(t, model, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if model.screen != ScreenEditingConnection {
		t.Errorf("the new connection key left the screen on %q", model.screen)
	}
}

// The card always draws the field, and the unfocused placeholder names the filter key.
func TestThePickerDrawsTheFilterFieldAtAllTimes(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{{Name: "shop", Engine: core.EngineMysql}}

	drawn := stripEscapes(model.renderPicker())
	if !strings.Contains(drawn, pickerFilterHint) {
		t.Errorf("the card does not name the filter key:\n%s", drawn)
	}
	if !strings.Contains(drawn, "shop") {
		t.Errorf("the card drops a row of the list:\n%s", drawn)
	}
}

// List keys still move the cursor while the field has the focus.
func TestThePickerFilterKeepsTheListKeys(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{
		{Name: "alpha"}, {Name: "beta"}, {Name: "banana"},
	}

	pressKey(t, model, tea.KeyPressMsg{Code: '/', Text: "/"})
	pressKey(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})

	if model.picker.cursor != 1 {
		t.Errorf("down left the cursor on %d", model.picker.cursor)
	}
	if profile, found := model.pickedProfile(); !found || profile.Name != "banana" {
		t.Errorf("the cursor stands on %+v, found=%v", profile, found)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if model.picker.filter.Text != "" || model.picker.cursor != 0 {
		t.Errorf("backspace left %q and the cursor on %d",
			model.picker.filter.Text, model.picker.cursor)
	}
}

// The card draws the filter text and reports an empty result.
func TestThePickerDrawsTheFilterField(t *testing.T) {
	model := buildOfflineModel(t, 120, 30)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{{Name: "shop", Engine: core.EngineMysql}}

	model.picker.filter.SetText("none")
	drawn := stripEscapes(model.renderPicker())
	if !strings.Contains(drawn, "none") || !strings.Contains(drawn, "no match") {
		t.Errorf("the card does not draw the filter:\n%s", drawn)
	}
	if strings.Contains(drawn, "shop") {
		t.Errorf("the card draws a row the term dropped:\n%s", drawn)
	}
}
