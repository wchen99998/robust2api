package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const antigravityConnectionTestAction = "streamGenerateContent"

type AntigravityConnectionTestResult struct {
	Text        string
	MappedModel string
}

type AntigravityConnectionTester struct {
	tokenProvider *AntigravityTokenProvider
	httpUpstream  HTTPUpstream
}

func NewAntigravityConnectionTester(tokenProvider *AntigravityTokenProvider, httpUpstream HTTPUpstream) *AntigravityConnectionTester {
	return &AntigravityConnectionTester{
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
	}
}

func (t *AntigravityConnectionTester) TestConnection(ctx context.Context, account *Account, modelID string) (*AntigravityConnectionTestResult, error) {
	if t == nil {
		return nil, errors.New("antigravity connection tester not configured")
	}
	if t.tokenProvider == nil {
		return nil, errors.New("antigravity token provider not configured")
	}
	if t.httpUpstream == nil {
		return nil, errors.New("http upstream not configured")
	}

	accessToken, err := t.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
	}

	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	mappedModel := mapAntigravityModel(account, modelID)
	if mappedModel == "" {
		return nil, fmt.Errorf("model %s not in whitelist", modelID)
	}

	body, err := t.buildTestRequest(projectID, modelID, mappedModel)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := t.doWithRetry(ctx, account, accessToken, body, proxyURL)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("upstream returned empty response")
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("API 返回 %d: %s", resp.StatusCode, string(respBody))
	}

	return &AntigravityConnectionTestResult{
		Text:        extractAntigravitySSEText(respBody),
		MappedModel: mappedModel,
	}, nil
}

func (t *AntigravityConnectionTester) doWithRetry(ctx context.Context, account *Account, accessToken string, body []byte, proxyURL string) (*http.Response, error) {
	baseURL := resolveAntigravityForwardBaseURL()
	if baseURL == "" {
		return nil, errors.New("no antigravity forward base url configured")
	}

	var lastErr error
	for attempt := 1; attempt <= antigravityMaxRetries; attempt++ {
		req, err := antigravity.NewAPIRequestWithURL(ctx, baseURL, antigravityConnectionTestAction, accessToken, body)
		if err != nil {
			return nil, err
		}

		resp, err := t.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
		if err == nil && resp == nil {
			err = errors.New("upstream returned nil response")
		}
		if err != nil {
			lastErr = err
			if attempt < antigravityMaxRetries {
				if !sleepAntigravityBackoffWithContext(ctx, attempt) {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, fmt.Errorf("upstream request failed after retries: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests || shouldRetryAntigravityError(resp.StatusCode) {
			if attempt < antigravityMaxRetries {
				_ = drainAndClose(resp.Body)
				if !sleepAntigravityBackoffWithContext(ctx, attempt) {
					return nil, ctx.Err()
				}
				continue
			}
		}
		return resp, nil
	}
	return nil, lastErr
}

func (t *AntigravityConnectionTester) buildTestRequest(projectID, requestedModel, mappedModel string) ([]byte, error) {
	if strings.HasPrefix(requestedModel, "gemini-") {
		return buildAntigravityGeminiTestRequest(projectID, mappedModel)
	}
	return buildAntigravityClaudeTestRequest(projectID, mappedModel)
}

func buildAntigravityGeminiTestRequest(projectID, model string) ([]byte, error) {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "."},
				},
			},
		},
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": antigravity.GetDefaultIdentityPatch()},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 1,
		},
	}
	payloadBytes, _ := json.Marshal(payload)
	return wrapAntigravityV1InternalRequest(projectID, model, payloadBytes)
}

func buildAntigravityClaudeTestRequest(projectID, mappedModel string) ([]byte, error) {
	claudeReq := &antigravity.ClaudeRequest{
		Model: mappedModel,
		Messages: []antigravity.ClaudeMessage{
			{
				Role:    "user",
				Content: json.RawMessage(`"."`),
			},
		},
		MaxTokens: 1,
		Stream:    false,
	}
	return antigravity.TransformClaudeToGemini(claudeReq, projectID, mappedModel)
}

func wrapAntigravityV1InternalRequest(projectID, model string, originalBody []byte) ([]byte, error) {
	var request any
	if err := json.Unmarshal(originalBody, &request); err != nil {
		return nil, fmt.Errorf("解析请求体失败: %w", err)
	}

	return json.Marshal(map[string]any{
		"project":     projectID,
		"requestId":   "agent-" + uuid.New().String(),
		"userAgent":   "antigravity",
		"requestType": "agent",
		"model":       model,
		"request":     request,
	})
}

func extractAntigravitySSEText(respBody []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(respBody))
	var parts []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if text := extractAntigravityTextFromJSON([]byte(data)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

func extractAntigravityTextFromJSON(body []byte) string {
	paths := []string{
		"response.candidates.0.content.parts.0.text",
		"candidates.0.content.parts.0.text",
		"response.text",
		"text",
	}
	for _, path := range paths {
		if value := gjson.GetBytes(body, path); value.Exists() {
			return value.String()
		}
	}
	return ""
}

func drainAndClose(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64<<10))
	return body.Close()
}
