package mcp

import (
	"context"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// A connection source gives one tool call the connection it runs on. The server serves the
// same tools whichever source it has; only the connection behind them differs.

// ConnectionSource resolves the connection of one tool call.
type ConnectionSource interface {
	// NamesProfile is true for a source that serves several connections, so every tool
	// takes the name of the one to use.
	NamesProfile() bool
	// DiscoveryTools returns the tools a client uses to find a connection and the
	// notebooks of the team. A source that serves one connection returns none.
	DiscoveryTools() []Tool
	// Resolve returns what the tools of this call reach.
	Resolve(ctx context.Context, input map[string]any) (agent.ToolDeps, error)
	// TakesPlanToken is true where a write is approved by a token rather than by a dialog
	// of the client.
	TakesPlanToken(input map[string]any) bool
}

// boundSource is the connection the AI chat of the client is open on. There is one, the
// reader opened it, and the chat itself asks before a write runs.
type boundSource struct {
	deps agent.ToolDeps
}

// BindConnection returns a source that serves one open connection.
func BindConnection(deps agent.ToolDeps) ConnectionSource {
	return boundSource{deps: deps}
}

func (held boundSource) NamesProfile() bool                 { return false }
func (held boundSource) DiscoveryTools() []Tool             { return nil }
func (held boundSource) TakesPlanToken(map[string]any) bool { return false }

func (held boundSource) Resolve(
	_ context.Context, _ map[string]any,
) (agent.ToolDeps, error) {
	return held.deps, nil
}

// poolSource opens the profiles of the config file that `[mcp]` serves. It is the source of
// the server an agent outside masume connects to.
type poolSource struct {
	deps ToolDeps
}

// OpenProfiles returns a source that serves the profiles of `[mcp]`.
func OpenProfiles(deps ToolDeps) ConnectionSource {
	return poolSource{deps: deps}
}

// NamesProfile is false for a server of one profile, which needs no name.
func (held poolSource) NamesProfile() bool { return held.deps.ScopedProfile == "" }

// DiscoveryTools returns the tools that find a connection and the notebooks of the team.
func (held poolSource) DiscoveryTools() []Tool {
	return []Tool{
		buildListProfilesTool(held.deps),
		buildListNotebooksTool(held.deps),
		buildReadNotebookTool(held.deps),
	}
}

// TakesPlanToken is true where the client cannot ask its user, so a token stands for the
// answer.
func (held poolSource) TakesPlanToken(map[string]any) bool { return true }

func (held poolSource) Resolve(
	ctx context.Context, input map[string]any,
) (agent.ToolDeps, error) {
	token, _ := input["plan_token"].(string)
	profile, err := GetNamedProfile(held.deps.AccessDeps, input["profile"])
	if err != nil {
		return agent.ToolDeps{}, err
	}
	// Allow concurrent calls during database operations.
	releaseReader(ctx)
	connection, err := OpenNamedConnection(ctx, held.deps.AccessDeps, profile)
	if err != nil {
		return agent.ToolDeps{}, err
	}
	return agent.ToolDeps{
		Session:      connection.Session,
		Tables:       connection.Tables,
		Runner:       buildRunner(held.deps, profile, connection, token),
		WritePlanOff: profile.WritePlan == cfg.PlanOff,
	}, nil
}
