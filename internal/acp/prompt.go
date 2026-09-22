package acp

import (
	"strings"

	"github.com/masumedb/masume/internal/ai"
)

// The prompt of an agent is one block of text. The agent reaches the database through the
// MCP server of masume, so the tools of the request are left out.

// The lines that name each part of the prompt.
const (
	historyHeading  = "## Earlier in this conversation"
	questionHeading = "## The question"
)

// BuildPromptText returns the request as one block of text: the instructions, the earlier
// turns, and the question.
func BuildPromptText(request ai.Request) string {
	parts := []string{}
	if written := strings.TrimSpace(request.System); written != "" {
		parts = append(parts, written)
	}

	earlier, question := splitLastQuestion(request.Messages)
	if len(earlier) > 0 {
		parts = append(parts, historyHeading+"\n\n"+strings.Join(earlier, "\n\n"))
	}
	if question != "" {
		parts = append(parts, questionHeading+"\n\n"+question)
	}
	return strings.Join(parts, "\n\n")
}

// splitLastQuestion returns the earlier turns as written lines, and the text of the last
// question on its own.
func splitLastQuestion(messages []ai.Message) ([]string, string) {
	written := []string{}
	question := ""
	for at, message := range messages {
		text := strings.TrimSpace(message.Text)
		if text == "" {
			continue
		}
		if at == len(messages)-1 && message.Role == ai.RoleUser {
			question = text
			continue
		}
		written = append(written, describeRole(message.Role)+": "+text)
	}
	return written, question
}

// describeRole names who wrote one turn.
func describeRole(role string) string {
	if role == ai.RoleAssistant {
		return "Assistant"
	}
	return "User"
}
