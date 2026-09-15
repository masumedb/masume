package ai

import (
	"context"
	"fmt"
	"strings"
)

// The chat completions client sends to any server with the OpenAI chat endpoint, such as
// Ollama, LM Studio, llama.cpp or vLLM.

// chatModel is one model of a server with the chat completions endpoint.
type chatModel struct {
	model   string
	apiKey  string
	baseURL string
}

func openChatModel(model, apiKey, baseURL string) Model {
	return &chatModel{model: model, apiKey: apiKey, baseURL: baseURL}
}

func (held *chatModel) Describe() string {
	return "openai_compatible/" + held.model
}

// A tool call of this protocol, in the request and in the stream.
type chatCall struct {
	// Index keys the fragments of one call within one response.
	Index    int              `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function chatCallFunction `json:"function"`
}

type chatCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	// ToolCalls holds the calls an assistant turn requested.
	ToolCalls []chatCall `json:"tool_calls,omitempty"`
	// ToolCallID is the call a tool turn answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model      string        `json:"model"`
	Messages   []chatMessage `json:"messages"`
	Tools      []chatTool    `json:"tools,omitempty"`
	ToolChoice string        `json:"tool_choice,omitempty"`
	Stream     bool          `json:"stream"`
	// StreamOptions asks for the token counts in the last event.
	StreamOptions *chatStreamOptions `json:"stream_options,omitempty"`
}

type chatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// buildChatMessages builds the message list with the system prompt in a system message.
func buildChatMessages(request Request) []chatMessage {
	messages := []chatMessage{}
	if request.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: request.System})
	}

	for _, message := range request.Messages {
		// A tool result is one message per call, and it carries nothing else.
		if len(message.Answers) > 0 {
			for _, answered := range message.Answers {
				messages = append(messages, chatMessage{
					Role: "tool", ToolCallID: answered.CallID, Content: answered.Output,
				})
			}
			continue
		}
		if message.Text == "" && len(message.Calls) == 0 {
			continue
		}

		built := chatMessage{Role: message.Role, Content: message.Text}
		for index, call := range message.Calls {
			built.ToolCalls = append(built.ToolCalls, chatCall{
				Index: index, ID: call.ID, Type: "function",
				Function: chatCallFunction{Name: call.Name, Arguments: call.Arguments},
			})
		}
		messages = append(messages, built)
	}
	return messages
}

func (held *chatModel) buildRequest(request Request) chatRequest {
	tools := make([]chatTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema,
			},
		})
	}
	built := chatRequest{
		Model: held.model, Messages: buildChatMessages(request), Stream: true,
		StreamOptions: &chatStreamOptions{IncludeUsage: true},
	}
	if len(tools) > 0 {
		built.Tools = tools
		built.ToolChoice = "auto"
	}
	return built
}

// The events of the stream, in the form of this protocol.
type chatStreamEvent struct {
	Choices []struct {
		Delta struct {
			Content   string     `json:"content"`
			ToolCalls []chatCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// chatDone is the last line of the stream, which carries no event.
const chatDone = "[DONE]"

func (held *chatModel) Stream(
	ctx context.Context, request Request, onEvent func(Event),
) (Answer, error) {
	headers := map[string]string{}
	if held.apiKey != "" {
		headers["authorization"] = "Bearer " + held.apiKey
	}
	body, err := sendJSON(ctx, held.baseURL+"/chat/completions", headers,
		held.buildRequest(request))
	if err != nil {
		return Answer{}, err
	}
	defer func() { _ = body.Close() }()

	answer := Answer{FinishReason: FinishUnknown}
	text := strings.Builder{}
	// The fragments of each call arrive by index, in the order the model requested them.
	building := map[int]*openBlock{}
	order := []int{}
	// A block of text that follows a tool call starts a new block of the reply.
	started := false
	finish := ""

	err = readServerEvents(body, func(held serverEvent) error {
		if strings.TrimSpace(held.data) == chatDone {
			return nil
		}
		event := chatStreamEvent{}
		if problem := readJSONInto(held.data, &event); problem != nil {
			LogEvent("! cannot parse chat event: " + problem.Error())
			return nil
		}
		if event.Error != nil && event.Error.Message != "" {
			return fmt.Errorf("%s", event.Error.Message)
		}
		answer.Usage = readChatUsage(event, answer.Usage)

		for _, choice := range event.Choices {
			if choice.FinishReason != "" {
				finish = choice.FinishReason
			}
			if delta := choice.Delta.Content; delta != "" {
				if !started {
					started = true
					onEvent(Event{Kind: EventTextStart})
				}
				text.WriteString(delta)
				onEvent(Event{Kind: EventTextDelta, Text: delta})
			}
			for _, call := range choice.Delta.ToolCalls {
				open, waiting := building[call.Index]
				if !waiting {
					open = &openBlock{}
					building[call.Index] = open
					order = append(order, call.Index)
					// Text after a call belongs to a block of its own.
					started = false
				}
				if call.ID != "" {
					open.callID = call.ID
				}
				if call.Function.Name != "" {
					open.name = call.Function.Name
				}
				open.arguments.WriteString(call.Function.Arguments)
			}
		}
		return nil
	})
	if err != nil {
		return answer, err
	}

	for _, index := range order {
		open := building[index]
		answer.Calls = append(answer.Calls,
			readToolCall(open.callID, open.name, open.arguments.String()))
	}
	answer.Text = text.String()
	answer.FinishReason = readChatStopReason(finish, len(answer.Calls) > 0)
	return answer, nil
}

// readChatUsage returns the token counts of the request.
func readChatUsage(event chatStreamEvent, kept Usage) Usage {
	if event.Usage == nil {
		return kept
	}
	kept.InputTokens = event.Usage.PromptTokens
	kept.OutputTokens = event.Usage.CompletionTokens
	if event.Usage.PromptTokensDetails != nil {
		kept.CachedInputTokens = event.Usage.PromptTokensDetails.CachedTokens
	}
	return kept
}

// readChatStopReason maps the finish reason of this protocol to a shared finish reason.
func readChatStopReason(written string, asked bool) string {
	switch written {
	case "stop":
		// A server that requests calls without a reason of its own still requested them.
		if asked {
			return FinishToolCalls
		}
		return FinishStop
	case "tool_calls", "function_call":
		return FinishToolCalls
	case "length":
		return FinishLength
	case "content_filter":
		return FinishContentFilter
	case "":
		if asked {
			return FinishToolCalls
		}
		return FinishUnknown
	}
	return FinishOther
}
