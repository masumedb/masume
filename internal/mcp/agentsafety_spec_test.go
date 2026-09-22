package mcp_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db/engines"
	"github.com/masumedb/masume/internal/mcp"
)

// What an agent may send to a connection, and what the connection does with it. A SQLite
// file needs no server, so these run in the ordinary suite.

// buildGuardedTools opens a server on a file of four orders, under the access and the
// confirmation asked. The asker is one of a client that reported no elicitation, which is a
// client that cannot ask anyone.
func buildGuardedTools(
	t *testing.T, access cfg.AccessMode, level cfg.McpAccess, confirm cfg.ConfirmWrites,
) ([]mcp.Tool, *mcp.Sessions, cfg.Profile) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	profile := cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		Environment: cfg.EnvironmentProd, AccessMode: cfg.AccessWrite,
		ConfirmWrites: confirm, WritePlan: cfg.PlanUndo, UndoRows: cfg.DefaultUndoRows,
		PageSize: cfg.DefaultPageSize,
	}
	made, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = made.Close()

	session, err := engines.CreateAdapters().Open(context.Background(), profile, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, written := range []string{
		"create table orders (id integer primary key, status text not null)",
		"insert into orders (status) values ('open'), ('open'), ('sent'), ('open')",
	} {
		if _, err := session.RunQuery(context.Background(), written, 100, nil); err != nil {
			t.Fatal(err)
		}
	}
	_ = session.Close()

	profile.AccessMode = access
	sessions := mcp.CreateSessions(engines.CreateAdapters())
	tools := mcp.BuildTools(mcp.OpenProfiles(mcp.ToolDeps{
		AccessDeps: mcp.AccessDeps{
			Profiles: []cfg.Profile{profile},
			Config: cfg.McpConfig{
				Profiles: []string{"shop"}, Access: level, RowLimit: 100,
				Timeout: cfg.DefaultMcpTimeout,
			},
			Sessions: sessions,
		},
		Asker: mcp.CreateAsker(func(string) {}),
		Plans: mcp.CreatePlanTokens(),
	}))
	return tools, sessions, profile
}

// countAgentOrders returns the rows the relation holds, read through the same server.
func countAgentOrders(t *testing.T, tools []mcp.Tool) string {
	t.Helper()
	counted := runTool(t, tools, "run_query",
		map[string]any{"profile": "shop", "sql": "select count(*) as held from orders"})
	rows, is := counted["rows"].([][]any)
	if !is || len(rows) != 1 {
		t.Fatalf("the count answered %v", counted)
	}
	return core.FormatCell(rows[0][0], "")
}

// A read-only connection, and a connection an agent reaches read-only, refuse every form of
// write. None of them changes a row.
func TestAnAgentWritesNothingOnAReadOnlyConnection(t *testing.T) {
	writes := []string{
		"delete from orders",
		"delete from orders where status = 'open'",
		"update orders set status = 'sent'",
		"insert into orders (status) values ('new')",
		"drop table orders",
		"select 1; delete from orders",
		"create table copied as select * from orders",
		"select * into copied from orders",
		"with c as (delete from orders returning *) select * from c",
		"alter table orders add column note text",
		"create trigger sweep after insert on orders begin delete from orders; end",
	}
	for _, held := range []struct {
		name   string
		access cfg.AccessMode
		level  cfg.McpAccess
	}{
		{"a read-only connection", cfg.AccessReadOnly, cfg.McpFull},
		{"read-only MCP access", cfg.AccessWrite, cfg.McpReadOnly},
	} {
		t.Run(held.name, func(t *testing.T) {
			tools, _, _ := buildGuardedTools(t, held.access, held.level, cfg.ConfirmOff)
			for _, written := range writes {
				answered := runTool(t, tools, "run_query",
					map[string]any{"profile": "shop", "sql": written})
				if answered["ran"] == true && answered["error"] == nil {
					t.Errorf("%q ran: %v", written, answered)
				}
			}
			if rows := countAgentOrders(t, tools); rows != "4" {
				t.Errorf("the relation holds %s rows", rows)
			}
		})
	}
}

// MCP read-only access opens a read-only connection, so the server refuses what the client
// did not read as a write.
func TestReadOnlyAccessOpensAReadOnlyConnection(t *testing.T) {
	_, sessions, profile := buildGuardedTools(t,
		cfg.AccessWrite, cfg.McpReadOnly, cfg.ConfirmOff)
	connection, err := mcp.OpenNamedConnection(context.Background(), mcp.AccessDeps{
		Profiles: []cfg.Profile{profile},
		Config: cfg.McpConfig{
			Profiles: []string{"shop"}, Access: cfg.McpReadOnly,
			RowLimit: 100, Timeout: cfg.DefaultMcpTimeout,
		},
		Sessions: sessions,
	}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if held := connection.Session.Describe().Profile.AccessMode; held != cfg.AccessReadOnly {
		t.Errorf("the connection opened with access %q", held)
	}
}

// A statement that removes data is asked about, and the write runs only on a yes. A client
// that cannot ask receives a refusal.
func TestAWriteThatNeedsAQuestionRunsOnNoAnswer(t *testing.T) {
	for _, confirm := range []cfg.ConfirmWrites{
		cfg.ConfirmDelete, cfg.ConfirmWrite, cfg.ConfirmAgent,
	} {
		t.Run(string(confirm), func(t *testing.T) {
			tools, _, _ := buildGuardedTools(t, cfg.AccessWrite, cfg.McpFull, confirm)
			answered := runTool(t, tools, "run_query",
				map[string]any{"profile": "shop", "sql": "delete from orders"})
			if answered["ran"] != false {
				t.Errorf("the delete ran without a question: %v", answered)
			}
			if rows := countAgentOrders(t, tools); rows != "4" {
				t.Errorf("the relation holds %s rows", rows)
			}
		})
	}
}

// A statement that runs a routine of the server carries what the routine writes, which this
// client cannot read. It needs full access, and a question wherever a write is asked about.
func TestAStatementThatRunsARoutineIsRefusedLikeAWriteThatRemovesData(t *testing.T) {
	for _, held := range []struct {
		name  string
		level cfg.McpAccess
	}{
		{"read-write access", cfg.McpReadWrite},
		{"full access", cfg.McpFull},
	} {
		t.Run(held.name, func(t *testing.T) {
			tools, _, _ := buildGuardedTools(t,
				cfg.AccessWrite, held.level, cfg.ConfirmDelete)
			answered := runTool(t, tools, "run_query", map[string]any{
				"profile": "shop",
				"sql": "create trigger sweep after insert on orders " +
					"begin delete from orders; end",
			})
			if answered["ran"] != false {
				t.Errorf("the trigger was made with no question: %v", answered)
			}
			if rows := countAgentOrders(t, tools); rows != "4" {
				t.Errorf("the relation holds %s rows", rows)
			}
		})
	}
}

// A plan measures a write and runs none of it, whatever the access.
func TestPlanningAWriteChangesNothing(t *testing.T) {
	for _, level := range []cfg.McpAccess{cfg.McpReadOnly, cfg.McpFull} {
		tools, _, _ := buildGuardedTools(t, cfg.AccessWrite, level, cfg.ConfirmWrite)
		runTool(t, tools, "plan_write",
			map[string]any{"profile": "shop", "sql": "delete from orders"})
		if rows := countAgentOrders(t, tools); rows != "4" {
			t.Errorf("%s: the relation holds %s rows", level, rows)
		}
	}
}

// A plan of a statement, and a check of one, run none of it.
func TestPlanningAndCheckingRunNothingOfAWrite(t *testing.T) {
	tools, _, _ := buildGuardedTools(t, cfg.AccessWrite, cfg.McpFull, cfg.ConfirmWrite)
	for _, tool := range []string{"explain_query", "validate_query"} {
		for _, written := range []string{
			"delete from orders", "update orders set status = 'sent'",
			"insert into orders (status) values ('new')",
		} {
			runTool(t, tools, tool,
				map[string]any{"profile": "shop", "sql": written, "analyze": true})
			if rows := countAgentOrders(t, tools); rows != "4" {
				t.Fatalf("%s of %q changed the rows to %s", tool, written, rows)
			}
		}
	}
}

// A token authorizes one run of one statement on one profile, and nothing after it.
func TestAPlanTokenRunsOnceAndOnlyItsOwnStatement(t *testing.T) {
	tools, _, _ := buildGuardedTools(t, cfg.AccessWrite, cfg.McpFull, cfg.ConfirmAgent)
	written := "delete from orders where status = 'sent'"

	measured := runTool(t, tools, "plan_write",
		map[string]any{"profile": "shop", "sql": written})
	token, is := measured["token"].(string)
	if !is || token == "" {
		t.Fatalf("the plan issued no token: %v", measured)
	}

	other := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "delete from orders", "plan_token": token,
	})
	if other["ran"] != false {
		t.Errorf("the token of one statement ran another: %v", other)
	}
	first := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": written, "plan_token": token,
	})
	if first["ran"] != true {
		t.Fatalf("the token did not run its own statement: %v", first)
	}
	again := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": written, "plan_token": token,
	})
	if again["ran"] != false {
		t.Errorf("the token ran a second time: %v", again)
	}
	if rows := countAgentOrders(t, tools); rows != "3" {
		t.Errorf("the relation holds %s rows", rows)
	}
}
