package service

import "strings"

// mapAntigravityModel resolves the public model name through Antigravity
// account mappings. An empty result means the account does not support the
// requested model.
func mapAntigravityModel(account *Account, requestedModel string) string {
	if account == nil {
		return ""
	}

	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return ""
	}

	mapped := account.GetMappedModel(requestedModel)
	if mapped != requestedModel {
		return mapped
	}

	if account.IsModelSupported(requestedModel) {
		return requestedModel
	}

	return ""
}

func antigravityModelSupported(requestedModel string) bool {
	return strings.HasPrefix(requestedModel, "claude-") ||
		strings.HasPrefix(requestedModel, "gemini-")
}

// applyThinkingModelSuffix adjusts mapped Claude model names for requests that
// explicitly enable thinking.
func applyThinkingModelSuffix(mappedModel string, thinkingEnabled bool) string {
	if !thinkingEnabled {
		return mappedModel
	}
	if mappedModel == "claude-sonnet-4-5" {
		return "claude-sonnet-4-5-thinking"
	}
	return mappedModel
}
