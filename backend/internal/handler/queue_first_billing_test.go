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
