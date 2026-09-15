package ai_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/ai"
	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestFindAPIKeyPrefersTheWrittenKey(t *testing.T) {
	key, found := ai.FindAPIKey(cfg.AiProviderSettings{APIKey: "sk-written", APIKeyEnv: "MISSING"})
	if !found || key != "sk-written" {
		t.Errorf("the key reads %q, found=%v", key, found)
	}
}

func TestFindAPIKeyReadsTheEnvironment(t *testing.T) {
	t.Setenv("MASUME_TEST_AI_KEY", "sk-env")
	key, found := ai.FindAPIKey(cfg.AiProviderSettings{APIKeyEnv: "MASUME_TEST_AI_KEY"})
	if !found || key != "sk-env" {
		t.Errorf("the key reads %q, found=%v", key, found)
	}
}

func TestIsProviderReadyFollowsTheProvider(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenai: {APIKey: "sk-openai"},
	}}
	if !ai.IsProviderReady(config, cfg.ProviderOpenai) {
		t.Error("a provider with a key reports none")
	}
	if ai.IsProviderReady(config, cfg.ProviderAnthropic) {
		t.Error("a provider with no key reports one")
	}
}

// The compatible provider reaches a server of the user, which often has no key at all. Its
// address and its model are the settings it cannot run without.
func TestIsProviderReadyReadsTheAddressOfTheCompatibleProvider(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenaiCompatible: {Model: "qwen3", BaseURL: "http://localhost:11434"},
	}}
	if !ai.IsProviderReady(config, cfg.ProviderOpenaiCompatible) {
		t.Error("a server with an address and a model reports a missing setting")
	}

	config.Providers[cfg.ProviderOpenaiCompatible] = cfg.AiProviderSettings{Model: "qwen3"}
	written := ai.DescribeMissingSetting(config, cfg.ProviderOpenaiCompatible)
	if !strings.Contains(written, "base_url") {
		t.Errorf("the message does not name base_url: %q", written)
	}

	config.Providers[cfg.ProviderOpenaiCompatible] = cfg.AiProviderSettings{
		BaseURL: "http://localhost:11434",
	}
	held := ai.DescribeMissingSetting(config, cfg.ProviderOpenaiCompatible)
	if !strings.Contains(held, "model") {
		t.Errorf("the message does not name model: %q", held)
	}
}

func TestDescribeMissingSettingNamesTheTableAndTheVariable(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenai:    {},
		cfg.ProviderAnthropic: {APIKeyEnv: "ANTHROPIC_API_KEY"},
	}}
	written := ai.DescribeMissingSetting(config, cfg.ProviderOpenai)
	if !strings.Contains(written, "[ai.providers.openai]") {
		t.Errorf("the message does not name the table: %q", written)
	}
	held := ai.DescribeMissingSetting(config, cfg.ProviderAnthropic)
	if !strings.Contains(held, "ANTHROPIC_API_KEY") {
		t.Errorf("the message does not name the variable: %q", held)
	}
}

// A provider without a key must give an error here, with a message that says what to
// configure. A model opened with an empty key sends the request and gets a 401.
func TestOpenModelRefusesAProviderWithNoKey(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenai: {Model: "gpt-5"},
	}}
	held, err := ai.OpenModel(config, cfg.ProviderOpenai, "masume/shop")
	if held != nil {
		t.Fatal("a provider with no key opened a model")
	}
	if err == nil || !strings.Contains(err.Error(), "[ai.providers.openai]") {
		t.Errorf("the failure reads %v, wanted what DescribeMissingSetting writes", err)
	}
}

func TestOpenModelOpensTheProviderWithAKey(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderAnthropic: {Model: "claude-opus-5", APIKey: "sk-written"},
	}}
	held, err := ai.OpenModel(config, cfg.ProviderAnthropic, "masume/shop")
	if err != nil {
		t.Fatalf("a provider with a key failed to open: %v", err)
	}
	if held.Describe() != "anthropic/claude-opus-5" {
		t.Errorf("the model describes itself as %q", held.Describe())
	}
}
