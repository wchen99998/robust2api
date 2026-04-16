package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/gin-gonic/gin"
)

type usageRecordErrTask func(context.Context) error

func usageRecordTaskTimeout(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Billing.Stream.PublishTimeoutSeconds > 0 {
		return time.Duration(cfg.Billing.Stream.PublishTimeoutSeconds) * time.Second
	}
	return 10 * time.Second
}

func queueFirstNonStreamEnabled(cfg *config.Config, reqStream bool) bool {
	if reqStream || cfg == nil {
		return false
	}
	if cfg.RunMode == config.RunModeSimple {
		return false
	}
	return cfg.Billing.QueueFirstNonStreamEnabled
}

func recordLegacyStreamingBilling(endpoint string) {
	appelotel.M().RecordLegacyStreamingBilling(context.Background(), endpoint)
}

type bufferedResponseCapture struct {
	original gin.ResponseWriter
	buffered *bufferedResponseWriter
}

const queueFirstBufferedResponseMemoryLimit = 1 << 20

func beginBufferedResponseCapture(c *gin.Context, enabled bool) *bufferedResponseCapture {
	if !enabled || c == nil || c.Writer == nil {
		return nil
	}
	capture := &bufferedResponseCapture{
		original: c.Writer,
		buffered: newBufferedResponseWriter(c.Writer),
	}
	c.Writer = capture.buffered
	return capture
}

func (c *bufferedResponseCapture) Discard(ctx *gin.Context) {
	if c == nil || ctx == nil {
		return
	}
	ctx.Writer = c.original
	if c.buffered != nil {
		c.buffered.cleanup()
	}
}

func (c *bufferedResponseCapture) Commit(ctx *gin.Context) error {
	if c == nil || ctx == nil {
		return nil
	}
	ctx.Writer = c.original
	return c.buffered.CommitTo(c.original)
}

type bufferedResponseWriter struct {
	original  gin.ResponseWriter
	header    http.Header
	body      bytes.Buffer
	spill     *os.File
	spillPath string
	status    int
	size      int
}

func newBufferedResponseWriter(original gin.ResponseWriter) *bufferedResponseWriter {
	clonedHeader := make(http.Header)
	if original != nil {
		for key, values := range original.Header() {
			clonedHeader[key] = append([]string(nil), values...)
		}
	}
	return &bufferedResponseWriter{
		original: original,
		header:   clonedHeader,
		status:   http.StatusOK,
		size:     -1,
	}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(code int) {
	if code > 0 {
		w.status = code
	}
}

func (w *bufferedResponseWriter) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}

func (w *bufferedResponseWriter) Write(data []byte) (int, error) {
	if !w.Written() {
		w.WriteHeaderNow()
	}
	n, err := w.appendBody(data)
	if n > 0 {
		w.size += n
	}
	return n, err
}

func (w *bufferedResponseWriter) WriteString(s string) (int, error) {
	if !w.Written() {
		w.WriteHeaderNow()
	}
	n, err := w.appendBody([]byte(s))
	if n > 0 {
		w.size += n
	}
	return n, err
}

func (w *bufferedResponseWriter) Status() int {
	return w.status
}

func (w *bufferedResponseWriter) Size() int {
	return w.size
}

func (w *bufferedResponseWriter) Written() bool {
	return w.size >= 0
}

func (w *bufferedResponseWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *bufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := any(w.original).(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("buffered response writer does not support hijack")
	}
	return hijacker.Hijack()
}

func (w *bufferedResponseWriter) CloseNotify() <-chan bool {
	if notifier, ok := any(w.original).(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}
	ch := make(chan bool, 1)
	ch <- true
	return ch
}

func (w *bufferedResponseWriter) Pusher() http.Pusher {
	if pusher, ok := any(w.original).(http.Pusher); ok {
		return pusher
	}
	return nil
}

func (w *bufferedResponseWriter) CommitTo(target gin.ResponseWriter) error {
	defer w.cleanup()
	if target == nil {
		return nil
	}
	dstHeader := target.Header()
	for key := range dstHeader {
		delete(dstHeader, key)
	}
	for key, values := range w.header {
		dstHeader[key] = append([]string(nil), values...)
	}
	if !w.Written() {
		return nil
	}
	target.WriteHeader(w.status)
	if w.spill == nil && w.body.Len() == 0 {
		return nil
	}
	if w.spill != nil {
		if _, err := w.spill.Seek(0, io.SeekStart); err != nil {
			return err
		}
		_, err := io.Copy(target, w.spill)
		return err
	}
	_, err := target.Write(w.body.Bytes())
	return err
}

func (w *bufferedResponseWriter) appendBody(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if err := w.ensureSpill(len(data)); err != nil {
		return 0, err
	}
	if w.spill != nil {
		return w.spill.Write(data)
	}
	return w.body.Write(data)
}

func (w *bufferedResponseWriter) ensureSpill(extra int) error {
	if w.spill != nil {
		return nil
	}
	if w.body.Len()+extra <= queueFirstBufferedResponseMemoryLimit {
		return nil
	}
	tmp, err := os.CreateTemp("", "sub2api-queue-first-*")
	if err != nil {
		return err
	}
	if w.body.Len() > 0 {
		if _, err := tmp.Write(w.body.Bytes()); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return err
		}
		w.body.Reset()
	}
	w.spill = tmp
	w.spillPath = tmp.Name()
	return nil
}

func (w *bufferedResponseWriter) cleanup() {
	if w == nil {
		return
	}
	w.body.Reset()
	if w.spill != nil {
		_ = w.spill.Close()
		if w.spillPath != "" {
			_ = os.Remove(w.spillPath)
		}
		w.spill = nil
		w.spillPath = ""
	}
}
