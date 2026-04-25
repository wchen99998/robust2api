package service

import (
	"context"
	"errors"
	"fmt"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const (
	antigravityMaxRetries     = 3
	antigravityRetryBaseDelay = time.Second
	antigravityRetryMaxDelay  = 16 * time.Second

	antigravityForwardBaseURLEnv = "GATEWAY_ANTIGRAVITY_FORWARD_BASE_URL"
)

type AntigravityAccountSwitchError struct {
	OriginalAccountID int64
	RateLimitedModel  string
	IsStickySession   bool
}

func (e *AntigravityAccountSwitchError) Error() string {
	return fmt.Sprintf("account %d model %s rate limited, need switch",
		e.OriginalAccountID, e.RateLimitedModel)
}

func IsAntigravityAccountSwitchError(err error) (*AntigravityAccountSwitchError, bool) {
	var switchErr *AntigravityAccountSwitchError
	if errors.As(err, &switchErr) {
		return switchErr, true
	}
	return nil, false
}

func resolveAntigravityForwardBaseURL() string {
	baseURLs := antigravity.ForwardBaseURLs()
	if len(baseURLs) == 0 {
		return ""
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(antigravityForwardBaseURLEnv)))
	if mode == "prod" && len(baseURLs) > 1 {
		return baseURLs[1]
	}
	return baseURLs[0]
}

func shouldRetryAntigravityError(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
		return true
	default:
		return false
	}
}

func isAntigravityConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

func sleepAntigravityBackoffWithContext(ctx context.Context, attempt int) bool {
	delay := antigravityRetryBaseDelay * time.Duration(1<<uint(max(attempt-1, 0)))
	if delay > antigravityRetryMaxDelay {
		delay = antigravityRetryMaxDelay
	}

	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(float64(delay) * 0.2 * (r.Float64()*2 - 1))
	sleepFor := delay + jitter
	if sleepFor < 0 {
		sleepFor = 0
	}

	timer := time.NewTimer(sleepFor)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
