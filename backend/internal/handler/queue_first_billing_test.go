package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBufferedResponseCaptureCommitSpillToDisk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginBufferedResponseCapture(c, true)
	require.NotNil(t, capture)

	payload := bytes.Repeat([]byte("a"), queueFirstBufferedResponseMemoryLimit+1024)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusAccepted)
	n, err := c.Writer.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	require.NotNil(t, capture.buffered.spill)
	spillPath := capture.buffered.spillPath
	require.NotEmpty(t, spillPath)
	_, err = os.Stat(spillPath)
	require.NoError(t, err)

	require.NoError(t, capture.Commit(c))
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, payload, recorder.Body.Bytes())
	_, err = os.Stat(spillPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestBufferedResponseCaptureCommitPreservesStatusForEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginBufferedResponseCapture(c, true)
	require.NotNil(t, capture)

	// Header-only response: status set explicitly, no body.
	c.Writer.Header().Set("X-Empty", "1")
	c.Writer.WriteHeader(http.StatusNoContent)

	require.NoError(t, capture.Commit(c))
	require.Equal(t, http.StatusNoContent, recorder.Code, "captured status must be replayed even without body")
	require.Equal(t, "1", recorder.Header().Get("X-Empty"))
	require.Empty(t, recorder.Body.Bytes())
}

func TestBufferedResponseCaptureRejectsOversize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginBufferedResponseCapture(c, true)
	require.NotNil(t, capture)
	defer capture.Discard(c)

	// Write up to the cap in chunks; the write that exceeds the cap must
	// return ErrBufferedResponseTooLarge so callers can surface a 503.
	chunk := bytes.Repeat([]byte("x"), 4<<20) // 4 MiB per write
	writes := (queueFirstBufferedResponseMaxTotal / len(chunk)) + 1
	var lastErr error
	for i := 0; i < writes; i++ {
		if _, err := c.Writer.Write(chunk); err != nil {
			lastErr = err
			break
		}
	}
	require.ErrorIs(t, lastErr, ErrBufferedResponseTooLarge)
}

func TestBufferedResponseCaptureCommitDoesNotReplayPartialBodyAfterOversize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginBufferedResponseCapture(c, true)
	require.NotNil(t, capture)

	payload := bytes.Repeat([]byte("x"), queueFirstBufferedResponseMaxTotal)
	_, err := c.Writer.Write(payload)
	require.NoError(t, err)

	_, err = c.Writer.Write([]byte("overflow"))
	require.ErrorIs(t, err, ErrBufferedResponseTooLarge)
	require.ErrorIs(t, capture.Commit(c), ErrBufferedResponseTooLarge)
	require.Empty(t, recorder.Body.Bytes(), "overflowed queue-first buffer must not replay a partial upstream response")
}

func TestBufferedResponseCaptureDiscardCleansSpillFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	capture := beginBufferedResponseCapture(c, true)
	require.NotNil(t, capture)

	payload := bytes.Repeat([]byte("b"), queueFirstBufferedResponseMemoryLimit+2048)
	_, err := c.Writer.Write(payload)
	require.NoError(t, err)
	spillPath := capture.buffered.spillPath
	require.NotEmpty(t, spillPath)
	_, err = os.Stat(spillPath)
	require.NoError(t, err)

	capture.Discard(c)
	_, err = os.Stat(spillPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
	require.Empty(t, recorder.Body.Bytes())
}
