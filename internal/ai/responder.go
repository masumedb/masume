package ai

import (
	"context"
)

// The chat sends one question to a responder, which is either a provider model with the tool
// loop of masume, or a coding agent with a tool loop of its own.

// Responder answers one question of the chat.
type Responder interface {
	// Describe returns the source of the answer, for display and logging.
	Describe() string
	// Reply answers the request and reports what happens through the hooks.
	Reply(ctx context.Context, request Request, hooks RunHooks) (RunResult, error)
}

// modelResponder is one provider model, with the tool loop of masume around it.
type modelResponder struct {
	model    Model
	maxSteps int
}

// OpenResponder returns a responder that sends to a provider model.
func OpenResponder(model Model, maxSteps int) Responder {
	return &modelResponder{model: model, maxSteps: maxSteps}
}

func (held *modelResponder) Describe() string { return held.model.Describe() }

func (held *modelResponder) Reply(
	ctx context.Context, request Request, hooks RunHooks,
) (RunResult, error) {
	return RunChat(ctx, held.model, request, held.maxSteps, hooks)
}
