package handler

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNextSSEFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		data      string
		final     bool
		wantFrame string
		wantRest  string
		wantOK    bool
	}{
		{
			name:      "lf_lf_terminator",
			data:      "data: foo\n\ndata: bar",
			wantFrame: "data: foo\n\n",
			wantRest:  "data: bar",
			wantOK:    true,
		},
		{
			name:      "crlf_crlf_terminator",
			data:      "data: foo\r\n\r\ndata: bar",
			wantFrame: "data: foo\r\n\r\n",
			wantRest:  "data: bar",
			wantOK:    true,
		},
		{
			name:      "mixed_crlf_lf_terminator",
			data:      "data: foo\r\n\ndata: bar",
			wantFrame: "data: foo\r\n\n",
			wantRest:  "data: bar",
			wantOK:    true,
		},
		{
			name:      "mixed_lf_crlf_terminator",
			data:      "data: foo\n\r\ndata: bar",
			wantFrame: "data: foo\n\r\n",
			wantRest:  "data: bar",
			wantOK:    true,
		},
		{
			name:   "incomplete_not_final",
			data:   "data: foo",
			final:  false,
			wantOK: false,
		},
		{
			name:      "incomplete_final_returns_rest",
			data:      "data: foo",
			final:     true,
			wantFrame: "data: foo",
			wantOK:    true,
		},
		{
			name:   "empty",
			data:   "",
			final:  false,
			wantOK: false,
		},
		{
			name:      "multi_line_data",
			data:      "event: message_delta\ndata: {\"type\":\"message_delta\"}\n\nnext",
			wantFrame: "event: message_delta\ndata: {\"type\":\"message_delta\"}\n\n",
			wantRest:  "next",
			wantOK:    true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			frame, rest, ok := nextSSEFrame([]byte(tc.data), tc.final)
			require.Equal(t, tc.wantOK, ok, "ok flag")
			if !ok {
				return
			}
			require.Equal(t, tc.wantFrame, string(frame), "frame")
			require.Equal(t, tc.wantRest, string(rest), "rest")
		})
	}
}

func TestResponsesTerminalFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		data string
		want bool
	}{
		{`[DONE]`, true},
		{`{"type":"response.completed","response":{}}`, true},
		{`{"type":"response.done"}`, true},
		{`{"type":"response.failed"}`, true},
		{`{"type":"response.incomplete"}`, true},
		{`{"type":"response.cancelled"}`, true},
		{`{"type":"response.canceled"}`, true},
		{`{"type":"response.output_text.delta"}`, false},
		{``, false},
	}
	for _, tc := range cases {
		if got := responsesTerminalFrame(tc.data); got != tc.want {
			t.Errorf("responsesTerminalFrame(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestAnthropicTerminalFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		event string
		data  string
		want  bool
	}{
		{"done_marker", "", "[DONE]", true},
		{"event_message_stop", "message_stop", `{}`, true},
		{"type_message_stop", "", `{"type":"message_stop"}`, true},
		{"message_delta_with_stop", "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, true},
		{"message_delta_without_stop", "message_delta", `{"type":"message_delta","delta":{}}`, false},
		{"message_delta_null_stop", "message_delta", `{"type":"message_delta","delta":{"stop_reason":null}}`, false},
		{"content_block_delta", "content_block_delta", `{"type":"content_block_delta"}`, false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := anthropicTerminalFrame(tc.event, tc.data)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestChatCompletionsTerminalFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		data string
		want bool
	}{
		{`[DONE]`, true},
		{`{"choices":[{"finish_reason":"stop"}]}`, true},
		{`{"choices":[{"finish_reason":"length"}]}`, true},
		{`{"choices":[{"finish_reason":null,"delta":{}}]}`, false},
		{`{"choices":[{"delta":{"content":"x"}}]}`, false},
		{``, false},
	}
	for _, tc := range cases {
		if got := chatCompletionsTerminalFrame(tc.data); got != tc.want {
			t.Errorf("chatCompletionsTerminalFrame(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestGeminiRawTerminalFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		data string
		want bool
	}{
		{`[DONE]`, true},
		{`  [DONE]  `, true},
		{`{"candidates":[]}`, false},
		{``, false},
	}
	for _, tc := range cases {
		if got := geminiRawTerminalFrame(tc.data); got != tc.want {
			t.Errorf("geminiRawTerminalFrame(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestTerminalBufferedResponseWriter_DefersTerminalFrame(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginStreamingTerminalCapture(c, true, streamingTerminalModeChatCompletions)
	require.NotNil(t, capture)

	_, err := c.Writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), "hello", "non-terminal frame must flush")

	terminalFrame := "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	_, err = c.Writer.Write([]byte(terminalFrame))
	require.NoError(t, err)
	require.NotContains(t, recorder.Body.String(), "finish_reason", "terminal frame must be held")
	require.NotContains(t, recorder.Body.String(), "[DONE]", "DONE marker must be held")

	require.NoError(t, capture.CommitTerminal(c))
	require.Contains(t, recorder.Body.String(), "finish_reason", "terminal frame flushed after commit")
	require.Contains(t, recorder.Body.String(), "[DONE]", "DONE marker flushed after commit")
}

func TestTerminalBufferedResponseWriter_DiscardTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginStreamingTerminalCapture(c, true, streamingTerminalModeAnthropic)
	require.NotNil(t, capture)

	_, err := c.Writer.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), "message_start")

	_, err = c.Writer.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	require.NoError(t, err)
	require.NotContains(t, recorder.Body.String(), "message_stop", "terminal frame buffered")

	capture.DiscardTerminal(c)
	require.NotContains(t, recorder.Body.String(), "message_stop", "discard should not flush the buffered terminal frame")
}

func TestTerminalBufferedResponseWriter_PendingCapFlushes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginStreamingTerminalCapture(c, true, streamingTerminalModeResponses)
	require.NotNil(t, capture)

	// A partial frame (no terminator) larger than the pending cap must
	// flush to the original writer rather than grow unbounded.
	blob := bytes.Repeat([]byte("A"), terminalCaptureMaxPendingBytes+1024)
	_, err := c.Writer.Write(blob)
	require.NoError(t, err)
	require.Equal(t, terminalCaptureMaxPendingBytes+1024, strings.Count(recorder.Body.String(), "A"))
}

func TestStreamingV2Enabled(t *testing.T) {
	t.Parallel()
	require.True(t, streamingV2Enabled(nil, true))
	cfg := &config.Config{}
	require.False(t, streamingV2Enabled(cfg, false))
	require.True(t, streamingV2Enabled(cfg, true))
	simpleCfg := &config.Config{RunMode: config.RunModeSimple}
	require.False(t, streamingV2Enabled(simpleCfg, true))
}
