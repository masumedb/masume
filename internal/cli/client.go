package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/acp"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/mcp"
	"github.com/turanmahmudov/masume/internal/ui"
)

// Dispatch of the process arguments. internal/ui draws the client the arguments select.

// usage is the --help text.
const usage = `masume - a database client for the terminal

usage:
  masume                                  open the client
  masume TARGET                           open a connection, postgres://you@host/shop
  masume --profile NAME                   open a user or project profile
  masume --detect                         open the picker with detected container databases
  masume run [TARGET | -p NAME] STATEMENT run statements
  masume nb run [TARGET | -p NAME] FILE   run a notebook
  masume dump [TARGET | -p NAME] FILE     dump schema and data
  masume restore [TARGET | -p NAME] FILE  restore a dump
  masume FILE.masume.md                   open a notebook file
  masume --mcp                            serve allowed profiles over JSON-RPC on stdio
  masume --mcp --profile=NAME             serve one allowed profile
  masume --mcp --check                    check enabled MCP profiles
  masume --version                        print the version
  masume --help                           print this help

Run masume run --help for headless options, and masume nb --help for notebooks.
Run masume dump --help and masume restore --help for the SQL file commands.

A command-line connection remains temporary until saved.
Press Ctrl+N, then e, then Ctrl+S to save the selected profile.
With no target or profile, masume opens $DATABASE_URL.

The config file is $XDG_CONFIG_HOME/masume/config.toml.
The history file is $XDG_STATE_HOME/masume/history.sqlite.
The nearest .masume.toml in or above the working directory supplies project profiles and queries.`

// Run reads the arguments of the process and returns the exit code.
func Run(argv []string) int {
	// An ACP agent requires the version of the client that opens the session.
	acp.ClientVersion = ResolveVersion()

	// Parse subcommand arguments before client flags.
	if len(argv) > 0 && argv[0] == "run" {
		return runHeadless(argv[1:])
	}
	if len(argv) > 0 && argv[0] == "nb" {
		return runNotebookCommand(argv[1:])
	}
	if len(argv) > 0 && argv[0] == "dump" {
		return runDumpCommand(argv[1:])
	}
	if len(argv) > 0 && argv[0] == "restore" {
		return runRestoreCommand(argv[1:])
	}
	if slices.Contains(argv, "--help") || slices.Contains(argv, "-h") {
		fmt.Println(usage)
		return 0
	}
	if slices.Contains(argv, "--version") || slices.Contains(argv, "-v") {
		fmt.Println("masume " + ResolveVersion())
		return 0
	}
	if slices.Contains(argv, "--mcp") {
		return mcp.RunServer(argv, ResolveVersion())
	}
	held, err := parseArguments(argv)
	if err == nil {
		err = runApp(held)
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "masume: "+err.Error())
	if _, isArgument := errors.AsType[argumentError](err); isArgument {
		fmt.Fprintln(os.Stderr, "run masume --help for usage")
		return 2
	}
	return 1
}

func runApp(held invocation) error {
	configPath := cfg.ResolveConfigPath()

	// Keep startup problems for display after the renderer enters the alternate screen.
	problems := []string{}

	// A config creation failure does not stop startup.
	if _, err := cfg.EnsureConfigFile(configPath); err != nil {
		problems = append(problems, "config: "+err.Error())
	}
	// A file written by an earlier version gains what this one brings, before it is read.
	if _, err := cfg.MigrateConfigFile(configPath); err != nil {
		problems = append(problems, "config: "+err.Error())
	}

	loaded := cfg.LoadConfigForWorkingDirectory(configPath)
	problems = append(problems, loaded.Project.Problems...)

	// Read before anything is opened, while the terminal still shows the shell.
	listed, start, err := resolveStartProfile(held, loaded.Profiles, os.Getenv)
	if err != nil {
		return err
	}
	loaded.Profiles = listed

	for _, problem := range loaded.Problems {
		problems = append(problems, problem.Describe())
	}
	for _, warning := range loaded.Warnings {
		problems = append(problems, warning.DescribeWarning())
	}
	for _, problem := range loaded.ThemeProblems {
		problems = append(problems, "theme: "+problem)
	}

	// A history failure does not stop startup.
	historyStore, historyErr := hist.Open(hist.DefaultPath())
	if historyErr != nil {
		problems = append(problems, "history: "+historyErr.Error())
	}
	defer func() { _ = historyStore.Close() }()

	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}

	model := ui.NewModel(loaded, engines.CreateAdapters(), historyStore, problems)
	if start != nil {
		model.OpenAtStart(*start)
	}
	if held.notebookPath != "" {
		model.OpenNotebookAtStart(held.notebookPath)
	}
	_, runErr := tea.NewProgram(model).Run()
	return runErr
}
