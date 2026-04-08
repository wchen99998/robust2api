//go:build unit

package service

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewDebouncedInvalidationHandler_SerializesOverlappingTriggers(t *testing.T) {
	var running atomic.Int32
	var maxRunning atomic.Int32
	var runs atomic.Int32
	started := make(chan struct{}, 4)

	handler := newDebouncedInvalidationHandler(10*time.Millisecond, func() {
		current := running.Add(1)
		for {
			prev := maxRunning.Load()
			if current <= prev || maxRunning.CompareAndSwap(prev, current) {
				break
			}
		}
		started <- struct{}{}
		time.Sleep(40 * time.Millisecond)
		runs.Add(1)
		running.Add(-1)
	})

	handler()
	require.Eventually(t, func() bool {
		return len(started) == 1
	}, time.Second, 10*time.Millisecond)

	handler()
	handler()
	handler()

	require.Eventually(t, func() bool {
		return runs.Load() == 2
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), maxRunning.Load(), "debounced invalidation handler should never overlap runs")
}
