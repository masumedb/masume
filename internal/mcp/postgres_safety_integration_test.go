//go:build integration

// An integration test of what an agent may send to a real PostgreSQL. The server is named
// through MASUME_TEST_POSTGRES.
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

// buildPostgresTools answers the tools of a server on a schema of four orders, a function
// that removes them, and the access and confirmation asked.
func buildPostgresTools(
	t *testing.T, access cfg.AccessMode, level cfg.McpAccess, confirm cfg.ConfirmWrites,
) []mcp.Tool {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.Postgres)
	profile.Name = "shop"
	profile.WritePlan, profile.UndoRows = cfg.PlanUndo, cfg.DefaultUndoRows
	profile.ConfirmWrites = confirm
	profile.Environment = cfg.EnvironmentProd
	// The server reads the password out of the environment, as MCP has no prompt.
	t.Setenv("MASUME_TEST_PG_PASSWORD", password)
	profile.Auth, profile.PasswordEnv = cfg.AuthPassword, "MASUME_TEST_PG_PASSWORD"

	writing := profile
	writing.AccessMode = cfg.AccessWrite
	session, err := engines.CreateAdapters().Open(context.Background(), writing, password)
	if err != nil {
		t.Fatalf("cannot reach the server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	dbtest.RunStatements(t, session,
		"drop schema if exists mcp_safety cascade",
		"create schema mcp_safety",
		"create table mcp_safety.orders (id serial primary key, status text not null)",
		"insert into mcp_safety.orders (status) values ('open'), ('open'), ('sent'), ('open')",
		`create function mcp_safety.sweep() returns int as $$
		   begin delete from mcp_safety.orders; return 1; end $$ language plpgsql`,
		`create procedure mcp_safety.sweep_all() as $$
		   begin delete from mcp_safety.orders; end $$ language plpgsql`,
	)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop schema if exists mcp_safety cascade", dbtest.ReadEverything, nil)
	})

	profile.AccessMode = access
	return mcp.BuildTools(mcp.OpenProfiles(mcp.ToolDeps{
		AccessDeps: mcp.AccessDeps{
			Profiles: []cfg.Profile{profile},
			Config: cfg.McpConfig{
				Profiles: []string{"shop"}, Access: level, RowLimit: 100,
				Timeout: cfg.DefaultMcpTimeout,
			},
			Sessions: mcp.CreateSessions(engines.CreateAdapters()),
		},
		Asker: mcp.CreateAsker(func(string) {}),
		Plans: mcp.CreatePlanTokens(),
	}))
}

func countSafetyOrders(t *testing.T, tools []mcp.Tool) string {
	t.Helper()
	counted := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "select count(*) as held from mcp_safety.orders",
	})
	rows, is := counted["rows"].([][]any)
	if !is || len(rows) != 1 {
		t.Fatalf("the count answered %v", counted)
	}
	return core.FormatCell(rows[0][0], "")
}

// A statement that removes rows without naming a write: a block of the server, and a call
// of a function that deletes. The connection asks about a write that removes data.
func TestAHiddenWriteRemovesNoRowWithoutAQuestion(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
	}{
		{"a block of the server", "do $$ begin delete from mcp_safety.orders; end $$"},
		{"a call of a function", "select mcp_safety.sweep()"},
		{"a trigger of another write", `
			create trigger sweep_on_insert after insert on mcp_safety.orders
			for each row execute function mcp_safety.sweep()`},
	} {
		t.Run(held.name, func(t *testing.T) {
			tools := buildPostgresTools(t,
				cfg.AccessWrite, cfg.McpReadWrite, cfg.ConfirmDelete)
			answered := runTool(t, tools, "run_query",
				map[string]any{"profile": "shop", "sql": held.sql})
			t.Logf("ran=%v reason=%v error=%v",
				answered["ran"], answered["reason"], answered["error"])
			if rows := countSafetyOrders(t, tools); rows != "4" {
				t.Errorf("%q left %s rows", held.sql, rows)
			}
		})
	}
}

// A read-only connection refuses the same statements, whatever the client asks.
func TestAReadOnlyConnectionRefusesEveryHiddenWrite(t *testing.T) {
	for _, written := range []string{
		"do $$ begin delete from mcp_safety.orders; end $$",
		"select mcp_safety.sweep()",
		"delete from mcp_safety.orders",
		"create table mcp_safety.copied as select * from mcp_safety.orders",
	} {
		tools := buildPostgresTools(t, cfg.AccessReadOnly, cfg.McpFull, cfg.ConfirmOff)
		answered := runTool(t, tools, "run_query",
			map[string]any{"profile": "shop", "sql": written})
		t.Logf("%-55s ran=%v reason=%v error=%v", written,
			answered["ran"], answered["reason"], answered["error"])
		if held := countSafetyOrders(t, tools); held != "4" {
			t.Errorf("%q left %s rows", written, held)
		}
	}
}

// The confirmation level of the connection against the statements that remove rows without
// naming a write. The rows left say which of them ran.
func TestTheConfirmationLevelCoversAHiddenWrite(t *testing.T) {
	for _, confirm := range []cfg.ConfirmWrites{
		cfg.ConfirmOff, cfg.ConfirmDelete, cfg.ConfirmWrite, cfg.ConfirmAgent,
	} {
		for _, held := range []struct {
			name string
			sql  string
		}{
			{"a plain delete", "delete from mcp_safety.orders"},
			{"a block", "do $$ begin delete from mcp_safety.orders; end $$"},
			{"a function call", "select mcp_safety.sweep()"},
			{"a call of a routine", "call mcp_safety.sweep_all()"},
		} {
			tools := buildPostgresTools(t,
				cfg.AccessWrite, cfg.McpReadWrite, confirm)
			answered := runTool(t, tools, "run_query",
				map[string]any{"profile": "shop", "sql": held.sql})
			rows := countSafetyOrders(t, tools)
			t.Logf("confirm=%-6s %-18s ran=%v reason=%v error=%v rows=%s",
				confirm, held.name, answered["ran"], answered["reason"],
				answered["error"], rows)
			if rows != "4" {
				t.Errorf("confirm=%s: %q left %s rows", confirm, held.sql, rows)
			}
		}
	}
}

// A connection an agent may write on still writes what its access and confirmation permit.
func TestFullAccessStillRunsTheWritesItPermits(t *testing.T) {
	for _, written := range []string{
		"delete from mcp_safety.orders where status = 'sent'",
		"select mcp_safety.sweep()",
		"do $$ begin delete from mcp_safety.orders; end $$",
		"call mcp_safety.sweep_all()",
		"insert into mcp_safety.orders (status) values ('new')",
	} {
		tools := buildPostgresTools(t, cfg.AccessWrite, cfg.McpFull, cfg.ConfirmOff)
		answered := runTool(t, tools, "run_query",
			map[string]any{"profile": "shop", "sql": written})
		t.Logf("%-55s ran=%v reason=%v error=%v rows=%s", written,
			answered["ran"], answered["reason"], answered["error"],
			countSafetyOrders(t, tools))
	}
}

// An ordinary read still answers its rows through the unit that refuses a write.
func TestAReadStillAnswersItsRows(t *testing.T) {
	tools := buildPostgresTools(t, cfg.AccessWrite, cfg.McpReadWrite, cfg.ConfirmWrite)
	for _, written := range []string{
		"select count(*) as held from mcp_safety.orders",
		"select id, status from mcp_safety.orders order by id",
		"with c as (select 1 as a) select * from c",
		"select now()",
	} {
		answered := runTool(t, tools, "run_query",
			map[string]any{"profile": "shop", "sql": written})
		if answered["ran"] != true || answered["error"] != nil {
			t.Errorf("%q answered %v", written, answered)
		}
	}
}

// A relation with a trigger runs statements the client cannot read, so a write on it is
// asked about even where the statement alone removes nothing.
func TestAWriteOnATriggeredRelationIsAsked(t *testing.T) {
	tools := buildPostgresTools(t, cfg.AccessWrite, cfg.McpFull, cfg.ConfirmDelete)
	// The trigger stands on the relation before the agent reaches it.
	owner := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, owner,
		`create function mcp_safety.sweep_trigger() returns trigger as $$
		  begin delete from mcp_safety.orders where status = 'open'; return null; end
		$$ language plpgsql`,
		`create trigger sweep_after_insert after insert on mcp_safety.orders
		for each statement execute function mcp_safety.sweep_trigger()`,
	)

	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "insert into mcp_safety.orders (status) values ('new')",
	})
	t.Logf("the insert: ran=%v reason=%v error=%v", answered["ran"],
		answered["reason"], answered["error"])
	if answered["ran"] != false {
		t.Errorf("the insert ran with no question: %v", answered)
	}
	if rows := countSafetyOrders(t, tools); rows != "4" {
		t.Errorf("the relation holds %s rows", rows)
	}
}

// An agent holds no transaction of its own: the statement that opens one is classified as a
// read, and it runs inside the unit that ends with it. A hidden write after it still writes
// nothing.
func TestAnAgentHoldsNoTransactionOfItsOwn(t *testing.T) {
	tools := buildPostgresTools(t, cfg.AccessWrite, cfg.McpReadWrite, cfg.ConfirmOff)
	for _, written := range []string{
		"begin",
		"select mcp_safety.sweep()",
		"commit",
	} {
		answered := runTool(t, tools, "run_query",
			map[string]any{"profile": "shop", "sql": written})
		t.Logf("%-35s ran=%v error=%v", written, answered["ran"], answered["error"])
	}
	if rows := countSafetyOrders(t, tools); rows != "4" {
		t.Errorf("the relation holds %s rows", rows)
	}
}
