package mcp

import (
	"testing"

	"github.com/masumedb/masume/internal/agent"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/query/statement"
)

// buildTestProfile returns one profile with that name and the settings of the test.
func buildTestProfile(name string) cfg.Profile {
	return cfg.Profile{
		Name: name, Engine: core.EngineSqlite, Database: "/tmp/" + name + ".db",
		Environment: cfg.EnvironmentDev, AccessMode: cfg.AccessWrite,
	}
}

func TestResolveProfileAccess(t *testing.T) {
	cases := []struct {
		name    string
		named   []string
		level   cfg.McpAccess
		profile cfg.McpAccess
		mode    cfg.AccessMode
		wanted  cfg.McpAccess
	}{
		{"a profile the file does not name is closed",
			nil, cfg.McpFull, cfg.McpUnset, cfg.AccessWrite, cfg.McpOff},
		{"the level of the file stands where the profile names none",
			[]string{"one"}, cfg.McpReadWrite, cfg.McpUnset, cfg.AccessWrite, cfg.McpReadWrite},
		{"a profile lowers the level of the file",
			[]string{"one"}, cfg.McpFull, cfg.McpReadOnly, cfg.AccessWrite, cfg.McpReadOnly},
		{"a profile cannot raise the level of the file",
			[]string{"one"}, cfg.McpReadOnly, cfg.McpFull, cfg.AccessWrite, cfg.McpReadOnly},
		{"a read-only profile stays read-only",
			[]string{"one"}, cfg.McpFull, cfg.McpUnset, cfg.AccessReadOnly, cfg.McpReadOnly},
		{"a profile that says off is closed",
			[]string{"one"}, cfg.McpFull, cfg.McpOff, cfg.AccessWrite, cfg.McpOff},
	}
	for _, held := range cases {
		profile := buildTestProfile("one")
		profile.McpAccess, profile.AccessMode = held.profile, held.mode
		config := cfg.McpConfig{Profiles: held.named, Access: held.level}
		if answered := ResolveProfileAccess(config, profile); answered != held.wanted {
			t.Errorf("%s: the level reads %q, wanted %q", held.name, answered, held.wanted)
		}
	}
}

func TestFindAccessRefusal(t *testing.T) {
	if refusal := FindAccessRefusal(cfg.McpReadOnly, statement.RiskNone); refusal != "" {
		t.Errorf("a read was refused: %s", refusal)
	}
	if refusal := FindAccessRefusal(cfg.McpFull, statement.RiskEveryRow); refusal != "" {
		t.Errorf("a full connection refused a statement: %s", refusal)
	}
	refusal := FindAccessRefusal(cfg.McpReadOnly, statement.RiskWrite)
	wanted := "MCP access is read-only; the statement writes to the database " +
		"and requires read-write access or higher"
	if refusal != wanted {
		t.Errorf("the refusal reads %q, wanted %q", refusal, wanted)
	}
}

func TestFindClosedReason(t *testing.T) {
	profile := buildTestProfile("one")
	if reason := FindClosedReason(cfg.McpConfig{}, profile); reason == "" {
		t.Error("a profile the file does not name reads as open")
	}

	profile.McpAccess = cfg.McpOff
	reason := FindClosedReason(
		cfg.McpConfig{Profiles: []string{"one"}, Access: cfg.McpFull}, profile)
	if reason != `"one" has mcp = "off"; MCP access requires read-only or higher` {
		t.Errorf("the reason reads %q", reason)
	}

	profile.McpAccess = cfg.McpUnset
	config := cfg.McpConfig{Profiles: []string{"one"}, Access: cfg.McpOff}
	if reason := FindClosedReason(config, profile); reason == "" {
		t.Error("a file whose level is off reads as open")
	}
	config.Access = cfg.McpReadOnly
	if reason := FindClosedReason(config, profile); reason != "" {
		t.Errorf("an open profile reads as closed: %s", reason)
	}
}

func TestDescribeNoOpenProfiles(t *testing.T) {
	cases := []struct {
		config cfg.McpConfig
		wanted string
	}{
		{cfg.McpConfig{},
			"no profiles are enabled for MCP; add a profile under [mcp] profiles in the config file"},
		{cfg.McpConfig{Profiles: []string{"one"}, Access: cfg.McpOff},
			`[mcp] access is "off"; MCP access requires read-only or higher`},
		{cfg.McpConfig{Profiles: []string{"one"}, Access: cfg.McpReadOnly},
			`no listed MCP profiles are available; check profile names and profile mcp settings`},
	}
	for _, held := range cases {
		if said := DescribeNoOpenProfiles(held.config); said != held.wanted {
			t.Errorf("the reason reads %q, wanted %q", said, held.wanted)
		}
	}
}

func TestGetNamedProfile(t *testing.T) {
	deps := AccessDeps{
		Profiles: []cfg.Profile{buildTestProfile("one"), buildTestProfile("two")},
		Config:   cfg.McpConfig{Profiles: []string{"one"}, Access: cfg.McpReadOnly},
	}

	profile, err := GetNamedProfile(deps, "one")
	if err != nil || profile.Name != "one" {
		t.Errorf("the open profile was refused: %v", err)
	}
	if _, err := GetNamedProfile(deps, nil); err == nil ||
		err.Error() != "profile is required; call list_profiles for available profiles" {
		t.Errorf("a call that names no profile gave %v", err)
	}
	if _, err := GetNamedProfile(deps, "three"); err == nil ||
		err.Error() != `no profile named "three"; call list_profiles for available profiles` {
		t.Errorf("a call that names no known profile gave %v", err)
	}
	if _, err := GetNamedProfile(deps, "two"); err == nil {
		t.Error("a profile the file does not open was reached")
	}

	scoped := deps
	scoped.ScopedProfile = "one"
	if profile, err := GetNamedProfile(scoped, nil); err != nil || profile.Name != "one" {
		t.Errorf("the scoped profile was refused: %v", err)
	}
	_, err = GetNamedProfile(scoped, "two")
	wanted := `this server only permits profile "one"; requested "two"`
	if err == nil || err.Error() != wanted {
		t.Errorf("the refusal reads %v, wanted %q", err, wanted)
	}
}

func TestListOpenProfiles(t *testing.T) {
	closed := buildTestProfile("two")
	closed.McpAccess = cfg.McpOff
	deps := AccessDeps{
		Profiles: []cfg.Profile{buildTestProfile("one"), closed, buildTestProfile("three")},
		Config: cfg.McpConfig{
			Profiles: []string{"one", "two", "three"}, Access: cfg.McpReadOnly,
		},
	}
	open := ListOpenProfiles(deps)
	if len(open) != 2 || open[0].Name != "one" || open[1].Name != "three" {
		t.Errorf("the open profiles are %v", open)
	}

	// A server started for one profile reaches that profile alone, so `--check` reports
	// that profile alone.
	scoped := deps
	scoped.ScopedProfile = "three"
	open = ListOpenProfiles(scoped)
	if len(open) != 1 || open[0].Name != "three" {
		t.Errorf("the scoped server opens %v, wanted three alone", open)
	}
}

func TestFindUnreachableReason(t *testing.T) {
	profile := buildTestProfile("one")
	if reason := FindUnreachableReason(profile); reason != "" {
		t.Errorf("a file the server can open reads as unreachable: %s", reason)
	}
	asking := cfg.Profile{
		Name: "two", Engine: core.EnginePostgres, Host: "localhost", Port: 5432,
		User: "someone", Auth: cfg.AuthPassword, Environment: cfg.EnvironmentDev,
	}
	if reason := FindUnreachableReason(asking); reason == "" {
		t.Error("a profile that asks for its password reads as reachable")
	}
}

func TestDescribeCheck(t *testing.T) {
	ok := ProfileCheck{Name: "one", Target: "/tmp/one.db", Access: cfg.McpReadOnly, TableCount: 3}
	if said := DescribeCheck(ok); said != "ok      one (read-only) /tmp/one.db · 3 tables" {
		t.Errorf("the line reads %q", said)
	}
	failed := ProfileCheck{
		Name: "two", Target: "/tmp/two.db", Access: cfg.McpFull, Problem: "it is not there",
	}
	if said := DescribeCheck(failed); said !=
		"FAILED  two (full) /tmp/two.db\n        it is not there" {
		t.Errorf("the line reads %q", said)
	}
}

func TestReadServerArguments(t *testing.T) {
	cases := []struct {
		argv   []string
		wanted string
		check  bool
	}{
		{[]string{"--mcp"}, "", false},
		{[]string{"--mcp", "--profile", "one"}, "one", false},
		{[]string{"--mcp", "--profile=one"}, "one", false},
		{[]string{"--mcp", "--check"}, "", true},
		{[]string{"--mcp", "--profile=one", "--check"}, "one", true},
	}
	for _, held := range cases {
		read, err := ReadServerArguments(held.argv)
		if err != nil {
			t.Errorf("%v was refused: %s", held.argv, err)
			continue
		}
		if read.Profile != held.wanted || read.Check != held.check {
			t.Errorf("%v named %q and %v, wanted %q and %v",
				held.argv, read.Profile, read.Check, held.wanted, held.check)
		}
	}
}

// An unknown argument must give an error. A misspelled `--profile` that is ignored would
// open every profile of the config file.
func TestReadServerArgumentsRefusesWhatItCannotRead(t *testing.T) {
	for _, argv := range [][]string{
		{"--mcp", "--profile"},
		{"--mcp", "--profile="},
		{"--mcp", "--profile", "--check"},
		{"--mcp", "--profil=shop"},
		{"--mcp", "shop"},
	} {
		read, err := ReadServerArguments(argv)
		if err == nil {
			t.Errorf("%v was read, and named %q", argv, read.Profile)
		}
	}
}

func TestDescribeServing(t *testing.T) {
	deps := AccessDeps{
		Profiles: []cfg.Profile{buildTestProfile("one"), buildTestProfile("two")},
		Config:   cfg.McpConfig{Profiles: []string{"one", "two"}, Access: cfg.McpReadOnly},
	}
	if said := describeServing(deps); said != "serving one, two" {
		t.Errorf("the report reads %q", said)
	}

	deps.ScopedProfile = "one"
	if said := describeServing(deps); said != `serving profile "one"` {
		t.Errorf("the report reads %q", said)
	}
	deps.ScopedProfile = "three"
	if said := describeServing(deps); said != `profile "three" is not in the config file` {
		t.Errorf("the report reads %q", said)
	}

	deps = AccessDeps{Config: cfg.McpConfig{}}
	if said := describeServing(deps); said != DescribeNoOpenProfiles(deps.Config) {
		t.Errorf("the report reads %q", said)
	}
}

// One tool catalogue answers both servers. The source of the connection is the only thing
// that differs, so a tool added once appears in both.
func TestBothSourcesServeOneCatalogue(t *testing.T) {
	bound := BuildTools(BindConnection(agent.ToolDeps{}))
	pooled := BuildTools(OpenProfiles(ToolDeps{
		AccessDeps: AccessDeps{Config: cfg.DefaultMcpConfig()},
	}))

	names := map[string]bool{}
	for _, tool := range bound {
		names[tool.Name] = true
	}
	for _, definition := range agent.Definitions() {
		if !names[definition.Name] {
			t.Errorf("the chat serves no %s", definition.Name)
		}
	}

	// The server of the profiles adds the tools that find one.
	if len(pooled) != len(bound)+3 {
		t.Errorf("the servers offer %d and %d tools", len(pooled), len(bound))
	}
	for _, wanted := range []string{"list_profiles", "list_notebooks", "read_notebook"} {
		if names[wanted] {
			t.Errorf("the chat serves %s, which needs several connections", wanted)
		}
	}
}

// A server of several connections asks which one to use, and a server of one does not.
func TestOnlyTheServerOfManyProfilesAsksForOne(t *testing.T) {
	bound := findToolNamed(t, BuildTools(BindConnection(agent.ToolDeps{})), "run_query")
	if holdsSchemaField(bound, "profile") || holdsSchemaField(bound, "plan_token") {
		t.Errorf("the chat asks for a profile or a token: %v", bound.InputSchema)
	}

	pooled := findToolNamed(t, BuildTools(OpenProfiles(ToolDeps{
		AccessDeps: AccessDeps{Config: cfg.DefaultMcpConfig()},
	})), "run_query")
	if !holdsSchemaField(pooled, "profile") || !holdsSchemaField(pooled, "plan_token") {
		t.Errorf("the server asks for neither: %v", pooled.InputSchema)
	}

	// A server scoped to one profile needs no name either.
	scoped := findToolNamed(t, BuildTools(OpenProfiles(ToolDeps{
		AccessDeps: AccessDeps{Config: cfg.DefaultMcpConfig(), ScopedProfile: "shop"},
	})), "run_query")
	if holdsSchemaField(scoped, "profile") {
		t.Errorf("a scoped server asks for a profile: %v", scoped.InputSchema)
	}
}

// findToolNamed returns the tool of that name.
func findToolNamed(t *testing.T, tools []Tool, name string) Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool named %s", name)
	return Tool{}
}

// holdsSchemaField is true where the input schema of a tool has this field.
func holdsSchemaField(tool Tool, name string) bool {
	properties, _ := tool.InputSchema["properties"].(map[string]any)
	_, held := properties[name]
	return held
}
