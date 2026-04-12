package repository

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestScanOldestPendingAgeScansAllPages(t *testing.T) {
	t.Parallel()

	callStarts := make([]string, 0, 2)
	oldest, err := scanOldestPendingAge(4, 2, func(start string, count int64) ([]redis.XPendingExt, error) {
		callStarts = append(callStarts, start)
		switch start {
		case "-":
			return []redis.XPendingExt{
				{ID: "1-0", Idle: time.Second},
				{ID: "1-1", Idle: 2 * time.Second},
			}, nil
		case "1-2":
			return []redis.XPendingExt{
				{ID: "2-0", Idle: 9 * time.Second},
				{ID: "5-0", Idle: 3 * time.Second},
			}, nil
		default:
			t.Fatalf("unexpected start %q", start)
			return nil, nil
		}
	})
	require.NoError(t, err)
	require.Equal(t, 9*time.Second, oldest)
	require.Equal(t, []string{"-", "1-2"}, callStarts)
}

func TestScanOldestPendingAgeZeroPending(t *testing.T) {
	t.Parallel()

	called := false
	oldest, err := scanOldestPendingAge(0, 10, func(start string, count int64) ([]redis.XPendingExt, error) {
		called = true
		return nil, nil
	})
	require.NoError(t, err)
	require.Zero(t, oldest)
	require.False(t, called)
}

func TestNextPendingScanStart(t *testing.T) {
	t.Parallel()

	require.Equal(t, "123-5", nextPendingScanStart("123-4"))
	require.Equal(t, "124-0", nextPendingScanStart("123-18446744073709551615"))
	require.Empty(t, nextPendingScanStart("bad-id"))
}
