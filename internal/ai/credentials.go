package ai

import "github.com/masumedb/masume/internal/cfg"

// Provider credential lookup and diagnostics.

// NeedsAPIKey is false for a provider whose server needs no key.
func NeedsAPIKey(id cfg.AiProviderID) bool {
	return id != cfg.ProviderOpenaiCompatible
}

// FindAPIKey returns the key of this provider, and whether the config has one.
func FindAPIKey(settings cfg.AiProviderSettings) (string, bool) {
	key := cfg.FindConfiguredValue(settings.APIKey, settings.APIKeyEnv)
	return key, key != ""
}

// FindBaseURL returns the provider address, and whether the config has one.
func FindBaseURL(settings cfg.AiProviderSettings) (string, bool) {
	address := cfg.FindConfiguredValue(settings.BaseURL, settings.BaseURLEnv)
	return address, address != ""
}

// IsProviderReady is true if the config has every setting this provider needs.
func IsProviderReady(config cfg.AiConfig, id cfg.AiProviderID) bool {
	return DescribeMissingSetting(config, id) == ""
}

// DescribeMissingSetting returns the instructions for a provider the chat cannot open, and
// an empty string for a provider it can.
func DescribeMissingSetting(config cfg.AiConfig, id cfg.AiProviderID) string {
	settings := config.Providers[id]
	table := "[ai.providers." + string(id) + "]"

	if !NeedsAPIKey(id) {
		if _, held := FindBaseURL(settings); !held {
			return "no server address: set base_url under " + table + " in the config file, " +
				"or set base_url_env to the environment variable containing the address. " +
				"Example: base_url = \"http://localhost:11434\""
		}
	} else if _, held := FindAPIKey(settings); !held {
		if settings.APIKeyEnv != "" {
			return "no API key: " + settings.APIKeyEnv + " is empty or unset. Set the " +
				"variable, or set api_key under " + table + " in the config file."
		}
		return "no API key: set api_key under " + table + " in the config file, or set " +
			"api_key_env to the environment variable containing the key."
	}

	if settings.Model == "" {
		return "no model: set model under " + table + " in the config file to a model " +
			"the server serves"
	}
	return ""
}
