package ui

import (
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
)

// programStatusApp is the program name of every Program Status Protocol report.
const programStatusApp = "masume"

// keepRunOutcome keeps the done or error report of a run that ended, until the next key
// press or click.
func (model *Model) keepRunOutcome(connection *app.Connection, tab *app.Tab, failed bool) {
	results := tab.Results.Results()
	elapsed := time.Duration(0)
	if len(results) > 0 {
		elapsed = time.Since(results[0].StartedAt)
	}
	outcome := tea.ProgramStatus{
		State: tea.ProgramStateDone, App: programStatusApp, Title: connection.Profile().Name,
		Message: tab.Label() + " finished in " + present.FormatDuration(elapsed),
	}
	if failed {
		outcome.State = tea.ProgramStateError
		outcome.Message = tab.Label() + " failed after " + present.FormatDuration(elapsed)
	}
	model.outcome = &outcome
}

// keepExportOutcome keeps the done or error report of an export that ended.
func (model *Model) keepExportOutcome(connection *app.Connection, message string, failed bool) {
	outcome := tea.ProgramStatus{
		State: tea.ProgramStateDone, App: programStatusApp, Title: connection.Profile().Name,
		Message: message,
	}
	if failed {
		outcome.State = tea.ProgramStateError
	}
	model.outcome = &outcome
}

// resolveProgramStatus returns the status the terminal shows: blocked while a card waits
// for the user, working while anything runs, then the outcome of the last run, and idle.
// A quitting client reports nothing, which clears the record.
func (model *Model) resolveProgramStatus() *tea.ProgramStatus {
	if model.quitting {
		return nil
	}
	status := tea.ProgramStatus{State: tea.ProgramStateIdle, App: programStatusApp, Title: "masume"}
	if model.screen == ScreenPromptingPassword {
		status.State, status.Kind = tea.ProgramStateBlocked, tea.ProgramStatusKindAuth
		status.Message = "password required"
		return &status
	}
	active := model.Active()
	if active != nil {
		status.Title = active.Profile().Name
		if blocked, found := resolveBlockedStatus(active); found {
			blocked.Title = status.Title
			return &blocked
		}
	}
	for _, connection := range model.connections.all() {
		if working, found := resolveWorkingStatus(connection); found {
			working.Title = connection.Profile().Name
			return &working
		}
	}
	if model.outcome != nil {
		return model.outcome
	}
	return &status
}

// resolveBlockedStatus returns the report of a card that waits for the user.
func resolveBlockedStatus(connection *app.Connection) (tea.ProgramStatus, bool) {
	status := tea.ProgramStatus{State: tea.ProgramStateBlocked, App: programStatusApp}
	overlay := connection.Overlay
	switch {
	case connection.Chat != nil && connection.Chat.Pending != nil:
		status.Kind, status.Message = tea.ProgramStatusKindPermission, "AI chat asks to run a statement"
	case overlay.Kind == app.OverlayConfirm || overlay.Kind == app.OverlayWritePlan:
		status.Kind = tea.ProgramStatusKindPermission
		status.Message = strings.TrimSpace(overlay.Title)
	case overlay.Kind == app.OverlayParameters:
		status.Kind, status.Message = tea.ProgramStatusKindQuestion, "values for the statement parameters"
	default:
		return tea.ProgramStatus{}, false
	}
	return status, true
}

// resolveWorkingStatus returns the report of the work a connection runs: a statement, a
// dump or a restore, an import, an export, or a reply of the AI chat.
func resolveWorkingStatus(connection *app.Connection) (tea.ProgramStatus, bool) {
	status := tea.ProgramStatus{State: tea.ProgramStateWorking, App: programStatusApp}
	overlay := connection.Overlay
	switch {
	case overlay.Kind == app.OverlayDump && overlay.Dump.Running:
		status.Message = string(overlay.Dump.Mode) + " " + overlay.Dump.Target
		applyProgress(&status, overlay.Dump.Progress)
		return status, true
	case overlay.Kind == app.OverlayImport && overlay.Import.Running:
		status.Message = "import"
		applyProgress(&status, overlay.Import.Progress)
		return status, true
	case connection.IsExporting():
		status.Message = "export"
		return status, true
	case connection.Chat != nil && connection.Chat.IsStreaming():
		status.Message = "AI chat answers"
		return status, true
	}
	for _, tab := range connection.Tabs {
		if tab.Results.IsRunning() {
			status.Message = "running " + tab.Label()
			return status, true
		}
	}
	return tea.ProgramStatus{}, false
}

// applyProgress writes the share done of a run whose size is known.
func applyProgress(status *tea.ProgramStatus, progress app.Progress) {
	if progress.HoldsBar() {
		status.Progress = int(progress.Done * 100 / progress.Total)
		status.HasProgress = true
	}
}

// insideTmux is true where tmux drops the report unless it is wrapped for passthrough.
var insideTmux = os.Getenv("TMUX") != ""

// reportProgramStatus returns the command that sends a changed status through tmux. Outside
// tmux the view carries the status, and the renderer sends it.
func (model *Model) reportProgramStatus() tea.Cmd {
	if !insideTmux {
		return nil
	}
	status := model.resolveProgramStatus()
	if programStatusEquals(model.reported, status) {
		return nil
	}
	model.reported = status
	written := ansi.ClearProgramStatus
	if status != nil {
		written = ansi.SetProgramStatus(*status)
	}
	return tea.Raw(ansi.TmuxPassthrough(written))
}

func programStatusEquals(one, other *tea.ProgramStatus) bool {
	if one == nil || other == nil {
		return one == other
	}
	return *one == *other
}
