package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
)

func TestTheKeyOfAPromptNamesWhatThePromptDoes(t *testing.T) {
	for kind, wanted := range map[app.PromptKind]string{
		app.PromptAiNotebook: "build", app.PromptTabName: "rename", app.PromptSaveName: "save",
	} {
		scene := keyScene{overlay: app.Overlay{Kind: app.OverlayPrompt, Prompt: kind}}
		if label := describePromptAnswer(scene); label != wanted {
			t.Errorf("the prompt %q labels its key %q, wanted %q", kind, label, wanted)
		}
	}
}
