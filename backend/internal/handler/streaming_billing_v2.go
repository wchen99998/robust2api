package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

func streamingV2Enabled(cfg *config.Config, reqStream bool) bool {
	if !reqStream || cfg == nil {
		return false
	}
	if cfg.RunMode == config.RunModeSimple {
		return false
	}
	return cfg.Billing.StreamingV2Enabled
}

func streamingBillingRequestID(ctx context.Context) string {
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
		if requestID, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			return "local:" + strings.TrimSpace(requestID)
		}
	}
	return "generated:" + uuid.NewString()
}

const terminalCaptureMaxPendingBytes = 4 << 20 // 4 MiB

type streamingTerminalMode string

const (
	streamingTerminalModeResponses       streamingTerminalMode = "responses"
	streamingTerminalModeAnthropic       streamingTerminalMode = "anthropic"
	streamingTerminalModeChatCompletions streamingTerminalMode = "chat_completions"
	streamingTerminalModeGeminiRaw       streamingTerminalMode = "gemini_raw"
)

type streamingTerminalCapture struct {
	original gin.ResponseWriter
	buffered *terminalBufferedResponseWriter
}

func beginStreamingTerminalCapture(c *gin.Context, enabled bool, mode streamingTerminalMode) *streamingTerminalCapture {
	if !enabled || c == nil || c.Writer == nil {
		return nil
	}
	capture := &streamingTerminalCapture{
		original: c.Writer,
		buffered: newTerminalBufferedResponseWriter(c.Writer, mode),
	}
	c.Writer = capture.buffered
	return capture
}

func (c *streamingTerminalCapture) CommitTerminal(ctx *gin.Context) error {
	if c == nil || ctx == nil {
		return nil
	}
	ctx.Writer = c.original
	return c.buffered.CommitTerminal()
}

func (c *streamingTerminalCapture) DiscardTerminal(ctx *gin.Context) {
	if c == nil || ctx == nil {
		return
	}
	ctx.Writer = c.original
	c.buffered.DiscardTerminal()
}

type terminalBufferedResponseWriter struct {
	original        gin.ResponseWriter
	mode            streamingTerminalMode
	pending         []byte
	terminal        bytes.Buffer
	terminalStarted bool
}

func newTerminalBufferedResponseWriter(original gin.ResponseWriter, mode streamingTerminalMode) *terminalBufferedResponseWriter {
	return &terminalBufferedResponseWriter{original: original, mode: mode}
}

func (w *terminalBufferedResponseWriter) Header() http.Header {
	return w.original.Header()
}

func (w *terminalBufferedResponseWriter) WriteHeader(code int) {
	w.original.WriteHeader(code)
}

func (w *terminalBufferedResponseWriter) WriteHeaderNow() {
	w.original.WriteHeaderNow()
}

func (w *terminalBufferedResponseWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if w.terminalStarted {
		return w.terminal.Write(data)
	}
	w.pending = append(w.pending, data...)
	if len(w.pending) > terminalCaptureMaxPendingBytes {
		if err := w.flushAllToOriginal(); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	if err := w.processPending(false); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (w *terminalBufferedResponseWriter) flushAllToOriginal() error {
	if len(w.pending) > 0 {
		if _, err := w.original.Write(w.pending); err != nil {
			return err
		}
		w.pending = nil
	}
	if w.terminal.Len() > 0 {
		if _, err := w.original.Write(w.terminal.Bytes()); err != nil {
			return err
		}
		w.terminal.Reset()
	}
	w.terminalStarted = false
	return nil
}

func (w *terminalBufferedResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *terminalBufferedResponseWriter) Status() int {
	return w.original.Status()
}

func (w *terminalBufferedResponseWriter) Size() int {
	return w.original.Size()
}

func (w *terminalBufferedResponseWriter) Written() bool {
	return w.original.Written()
}

func (w *terminalBufferedResponseWriter) Flush() {
	_ = w.processPending(false)
	w.original.Flush()
}

func (w *terminalBufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := any(w.original).(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("terminal buffered writer does not support hijack")
	}
	return hijacker.Hijack()
}

func (w *terminalBufferedResponseWriter) CloseNotify() <-chan bool {
	if notifier, ok := any(w.original).(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}
	ch := make(chan bool, 1)
	ch <- true
	return ch
}

func (w *terminalBufferedResponseWriter) Pusher() http.Pusher {
	if pusher, ok := any(w.original).(http.Pusher); ok {
		return pusher
	}
	return nil
}

func (w *terminalBufferedResponseWriter) CommitTerminal() error {
	if err := w.processPending(true); err != nil {
		return err
	}
	if w.terminal.Len() == 0 {
		w.terminalStarted = false
		return nil
	}
	if _, err := w.original.Write(w.terminal.Bytes()); err != nil {
		return err
	}
	w.terminal.Reset()
	w.terminalStarted = false
	w.original.Flush()
	return nil
}

func (w *terminalBufferedResponseWriter) DiscardTerminal() {
	w.pending = nil
	w.terminal.Reset()
	// Reset terminalStarted so any stray Write after discard is forwarded
	// to the original writer instead of silently buffering into terminal.
	w.terminalStarted = false
}

func (w *terminalBufferedResponseWriter) processPending(final bool) error {
	for {
		frame, rest, ok := nextSSEFrame(w.pending, final)
		if !ok {
			return nil
		}
		w.pending = rest
		if w.terminalStarted || sseFrameStartsTerminal(w.mode, frame) {
			w.terminalStarted = true
			if _, err := w.terminal.Write(frame); err != nil {
				return err
			}
			continue
		}
		if _, err := w.original.Write(frame); err != nil {
			return err
		}
	}
}

func nextSSEFrame(data []byte, final bool) ([]byte, []byte, bool) {
	if len(data) == 0 {
		return nil, nil, false
	}
	for i := 0; i < len(data)-1; i++ {
		// \r\n\r\n (4 bytes)
		if i+3 < len(data) && data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\r' && data[i+3] == '\n' {
			end := i + 4
			return append([]byte(nil), data[:end]...), append([]byte(nil), data[end:]...), true
		}
		// \r\n\n or \n\r\n (3 bytes)
		if i+2 < len(data) {
			if (data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\n') ||
				(data[i] == '\n' && data[i+1] == '\r' && data[i+2] == '\n') {
				end := i + 3
				return append([]byte(nil), data[:end]...), append([]byte(nil), data[end:]...), true
			}
		}
		// \n\n (2 bytes)
		if data[i] == '\n' && data[i+1] == '\n' {
			end := i + 2
			return append([]byte(nil), data[:end]...), append([]byte(nil), data[end:]...), true
		}
	}
	if !final {
		return nil, nil, false
	}
	return append([]byte(nil), data...), nil, true
}

func sseFrameStartsTerminal(mode streamingTerminalMode, frame []byte) bool {
	eventName, data := parseSSEFrame(frame)
	switch mode {
	case streamingTerminalModeResponses:
		return responsesTerminalFrame(data)
	case streamingTerminalModeAnthropic:
		return anthropicTerminalFrame(eventName, data)
	case streamingTerminalModeChatCompletions:
		return chatCompletionsTerminalFrame(data)
	case streamingTerminalModeGeminiRaw:
		return geminiRawTerminalFrame(data)
	default:
		return false
	}
}

func parseSSEFrame(frame []byte) (string, string) {
	if len(frame) == 0 {
		return "", ""
	}
	lines := strings.Split(strings.ReplaceAll(string(frame), "\r\n", "\n"), "\n")
	eventName := ""
	dataLines := make([]string, 0, 2)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		case strings.HasPrefix(trimmed, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
	}
	return eventName, strings.Join(dataLines, "\n")
}

func responsesTerminalFrame(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return true
	}
	switch gjson.Get(trimmed, "type").String() {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

func anthropicTerminalFrame(eventName, data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "[DONE]" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(eventName), "message_stop") {
		return true
	}
	if gjson.Get(trimmed, "type").String() == "message_stop" {
		return true
	}
	eventType := gjson.Get(trimmed, "type").String()
	if strings.EqualFold(strings.TrimSpace(eventName), "message_delta") || eventType == "message_delta" {
		stopReason := gjson.Get(trimmed, "delta.stop_reason")
		return stopReason.Exists() && stopReason.Type != gjson.Null && strings.TrimSpace(stopReason.String()) != ""
	}
	return false
}

func chatCompletionsTerminalFrame(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return true
	}
	finishReason := gjson.Get(trimmed, "choices.0.finish_reason")
	return finishReason.Exists() && finishReason.Type != gjson.Null
}

func geminiRawTerminalFrame(data string) bool {
	return strings.TrimSpace(data) == "[DONE]"
}
