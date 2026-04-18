package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestMetadataWriteAndRead(t *testing.T) {
	ctx := context.Background()
	ctx = WithIsMaxTokensOneHaikuRequest(ctx, true)
	ctx = WithThinkingEnabled(ctx, true)
	ctx = WithPrefetchedStickySession(ctx, 123, 456)
	ctx = WithSingleAccountRetry(ctx, true)
	ctx = WithAccountSwitchCount(ctx, 2)

	isHaiku, ok := IsMaxTokensOneHaikuRequestFromContext(ctx)
	require.True(t, ok)
	require.True(t, isHaiku)

	thinking, ok := ThinkingEnabledFromContext(ctx)
	require.True(t, ok)
	require.True(t, thinking)

	accountID, ok := PrefetchedStickyAccountIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, int64(123), accountID)

	groupID, ok := PrefetchedStickyGroupIDFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, int64(456), groupID)

	singleRetry, ok := SingleAccountRetryFromContext(ctx)
	require.True(t, ok)
	require.True(t, singleRetry)

	switchCount, ok := AccountSwitchCountFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, 2, switchCount)
}

func TestRequestMetadataReadPreferLatestValue(t *testing.T) {
	ctx := context.Background()
	ctx = WithThinkingEnabled(ctx, false)
	ctx = WithThinkingEnabled(ctx, true)

	thinking, ok := ThinkingEnabledFromContext(ctx)
	require.True(t, ok)
	require.True(t, thinking)
}

func TestRequestMetadataReadMissingValues(t *testing.T) {
	ctx := context.Background()

	_, ok := IsMaxTokensOneHaikuRequestFromContext(ctx)
	require.False(t, ok)

	_, ok = ThinkingEnabledFromContext(ctx)
	require.False(t, ok)

	_, ok = PrefetchedStickyAccountIDFromContext(ctx)
	require.False(t, ok)

	_, ok = PrefetchedStickyGroupIDFromContext(ctx)
	require.False(t, ok)

	_, ok = SingleAccountRetryFromContext(ctx)
	require.False(t, ok)

	_, ok = AccountSwitchCountFromContext(ctx)
	require.False(t, ok)
}
