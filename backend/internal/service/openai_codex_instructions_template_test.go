package service

import "testing"

func TestRenderForcedCodexInstructionsTemplate(t *testing.T) {
	out, err := renderForcedCodexInstructionsTemplate(
		"prefix\n{{ .ExistingInstructions }}\n{{ .UpstreamModel }}",
		forcedCodexInstructionsTemplateData{
			ExistingInstructions: "keep this",
			UpstreamModel:        "gpt-5.3-codex",
		},
	)
	if err != nil {
		t.Fatalf("renderForcedCodexInstructionsTemplate() error = %v", err)
	}
	if out != "prefix\nkeep this\ngpt-5.3-codex" {
		t.Fatalf("unexpected rendered template: %q", out)
	}
}

func TestApplyForcedCodexInstructionsTemplate(t *testing.T) {
	body := map[string]any{
		"instructions": "original",
	}
	changed, err := applyForcedCodexInstructionsTemplate(body, "{{ .ExistingInstructions }}\npatched", forcedCodexInstructionsTemplateData{
		ExistingInstructions: "original",
	})
	if err != nil {
		t.Fatalf("applyForcedCodexInstructionsTemplate() error = %v", err)
	}
	if !changed {
		t.Fatalf("expected instructions to change")
	}
	if got := body["instructions"]; got != "original\npatched" {
		t.Fatalf("unexpected instructions: %v", got)
	}
}
