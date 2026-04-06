package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/?start_date=2024-01-01&end_date=2024-01-02&timezone=UTC", nil)
	c.Request = req

	start, end := parseTimeRange(c)
	require.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), start)
	require.Equal(t, time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), end)

	req = httptest.NewRequest(http.MethodGet, "/?start_date=bad&timezone=UTC", nil)
	c.Request = req
	start, end = parseTimeRange(c)
	require.False(t, start.IsZero())
	require.False(t, end.IsZero())
}

func TestParseOpsViewParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?view=excluded", nil)
	require.Equal(t, opsListViewExcluded, parseOpsViewParam(c))

	c2, _ := gin.CreateTestContext(w)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?view=all", nil)
	require.Equal(t, opsListViewAll, parseOpsViewParam(c2))

	c3, _ := gin.CreateTestContext(w)
	c3.Request = httptest.NewRequest(http.MethodGet, "/?view=unknown", nil)
	require.Equal(t, opsListViewErrors, parseOpsViewParam(c3))

	require.Equal(t, "", parseOpsViewParam(nil))
}

func TestParseOpsDuration(t *testing.T) {
	dur, ok := parseOpsDuration("1h")
	require.True(t, ok)
	require.Equal(t, time.Hour, dur)

	_, ok = parseOpsDuration("invalid")
	require.False(t, ok)
}

func TestParseOpsTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	now := time.Now().UTC()
	startStr := now.Add(-time.Hour).Format(time.RFC3339)
	endStr := now.Format(time.RFC3339)
	c.Request = httptest.NewRequest(http.MethodGet, "/?start_time="+startStr+"&end_time="+endStr, nil)
	start, end, err := parseOpsTimeRange(c, "1h")
	require.NoError(t, err)
	require.True(t, start.Before(end))

	c2, _ := gin.CreateTestContext(w)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?start_time=bad", nil)
	_, _, err = parseOpsTimeRange(c2, "1h")
	require.Error(t, err)
}
