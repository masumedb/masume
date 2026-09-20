//go:build integration

// An integration test of what an agent may send to a real MySQL. The server is named
// through MASUME_TEST_MYSQL.
package mcp_test

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/mcp"
)

// A MySQL function writes where the server permits it, so a read that calls one runs in a
// unit the server refuses a write in.
func TestAReadCannotWriteThroughAMysqlFunction(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	profile.Name = "shop"
	profile.WritePlan, profile.UndoRows = cfg.PlanUndo, cfg.DefaultUndoRows
	profile.ConfirmWrites, profile.Environment = cfg.ConfirmOff, cfg.EnvironmentProd
	t.Setenv("MASUME_TEST_MYSQL_PASSWORD", password)
	profile.Auth, profile.PasswordEnv = cfg.AuthPassword, "MASUME_TEST_MYSQL_PASSWORD"

	writing := profile
	writing.AccessMode = cfg.AccessWrite
	session, err := engines.CreateAdapters().Open(context.Background(), writing, password)
	if err != nil {
		t.Fatalf("cannot reach the server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	dbtest.RunStatements(t, session,
		"drop table if exists mcp_safety_orders",
		"create table mcp_safety_orders (id int primary key auto_increment, status varchar(20))",
		"insert into mcp_safety_orders (status) values ('open'), ('open'), ('sent'), ('open')",
		"drop function if exists mcp_safety_sweep",
		"set global log_bin_trust_function_creators = 1",
		`create function mcp_safety_sweep() returns int modifies sql data deterministic
		   begin delete from mcp_safety_orders; return 1; end`,
	)
	t.Cleanup(func() {
		for _, written := range []string{
			"drop function if exists mcp_safety_sweep",
			"drop table if exists mcp_safety_orders",
		} {
			_, _ = session.RunQuery(context.Background(), written, dbtest.ReadEverything, nil)
		}
	})

	profile.AccessMode = cfg.AccessWrite
	tools := mcp.BuildTools(mcp.OpenProfiles(mcp.ToolDeps{
		AccessDeps: mcp.AccessDeps{
			Profiles: []cfg.Profile{profile},
			Config: cfg.McpConfig{
				Profiles: []string{"shop"}, Access: cfg.McpReadWrite, RowLimit: 100,
				Timeout: cfg.DefaultMcpTimeout,
			},
			Sessions: mcp.CreateSessions(engines.CreateAdapters()),
		},
		Asker: mcp.CreateAsker(func(string) {}),
		Plans: mcp.CreatePlanTokens(),
	}))

	answered := runTool(t, tools, "run_query",
		map[string]any{"profile": "shop", "sql": "select mcp_safety_sweep() as held"})
	t.Logf("the function call: ran=%v reason=%v error=%v",
		answered["ran"], answered["reason"], answered["error"])

	counted := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "select count(*) as held from mcp_safety_orders",
	})
	rows, is := counted["rows"].([][]any)
	if !is || len(rows) != 1 {
		t.Fatalf("the count answered %v", counted)
	}
	if held := core.FormatCell(rows[0][0], ""); held != "4" {
		t.Errorf("the relation holds %s rows", held)
	}
}
