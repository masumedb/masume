package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
)

func TestAChatThatCannotAnswerShowsWhatToSetUp(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	model.ai.DefaultAgent, model.aiAgent = "", ""
	model.ai.Providers = map[cfg.AiProviderID]cfg.AiProviderSettings{}
	connection := model.Active()
	connection.Overlay = app.Overlay{Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0)}

	drawn := stripEscapes(model.render())
	for _, wanted := range []string{"AI chat is not set up", "open AI settings"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the chat does not show %q:\n%s", wanted, drawn)
		}
	}
	if strings.Contains(drawn, chatPlaceholder) {
		t.Error("the chat offers a field it cannot answer")
	}

	model.runChatAction(connection, connection.Active(),
		Match{Scope: cfg.ScopeDialog, Action: ActionSendQuestion})
	if model.screen != ScreenSettings {
		t.Errorf("the key opened %q, wanted the settings", model.screen)
	}
}
