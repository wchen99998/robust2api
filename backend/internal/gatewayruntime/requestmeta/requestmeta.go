package requestmeta

import (
	"context"
)

type requestMetadataContextKey struct{}

var requestMetadataKey = requestMetadataContextKey{}

type RequestMetadata struct {
	IsMaxTokensOneHaikuRequest *bool
	ThinkingEnabled            *bool
	PrefetchedStickyAccountID  *int64
	PrefetchedStickyGroupID    *int64
	SingleAccountRetry         *bool
	AccountSwitchCount         *int
}

func metadataFromContext(ctx context.Context) *RequestMetadata {
	if ctx == nil {
		return nil
	}
	md, _ := ctx.Value(requestMetadataKey).(*RequestMetadata)
	return md
}

func updateRequestMetadata(ctx context.Context, update func(md *RequestMetadata)) context.Context {
	if ctx == nil {
		return nil
	}
	current := metadataFromContext(ctx)
	next := &RequestMetadata{}
	if current != nil {
		*next = *current
	}
	update(next)
	return context.WithValue(ctx, requestMetadataKey, next)
}

func WithIsMaxTokensOneHaikuRequest(ctx context.Context, value bool) context.Context {
	return updateRequestMetadata(ctx, func(md *RequestMetadata) {
		v := value
		md.IsMaxTokensOneHaikuRequest = &v
	})
}

func WithThinkingEnabled(ctx context.Context, value bool) context.Context {
	return updateRequestMetadata(ctx, func(md *RequestMetadata) {
		v := value
		md.ThinkingEnabled = &v
	})
}

func WithPrefetchedStickySession(ctx context.Context, accountID, groupID int64) context.Context {
	return updateRequestMetadata(ctx, func(md *RequestMetadata) {
		account := accountID
		group := groupID
		md.PrefetchedStickyAccountID = &account
		md.PrefetchedStickyGroupID = &group
	})
}

func WithSingleAccountRetry(ctx context.Context, value bool) context.Context {
	return updateRequestMetadata(ctx, func(md *RequestMetadata) {
		v := value
		md.SingleAccountRetry = &v
	})
}

func WithAccountSwitchCount(ctx context.Context, value int) context.Context {
	return updateRequestMetadata(ctx, func(md *RequestMetadata) {
		v := value
		md.AccountSwitchCount = &v
	})
}

func IsMaxTokensOneHaikuRequestFromContext(ctx context.Context) (bool, bool) {
	if md := metadataFromContext(ctx); md != nil && md.IsMaxTokensOneHaikuRequest != nil {
		return *md.IsMaxTokensOneHaikuRequest, true
	}
	return false, false
}

func ThinkingEnabledFromContext(ctx context.Context) (bool, bool) {
	if md := metadataFromContext(ctx); md != nil && md.ThinkingEnabled != nil {
		return *md.ThinkingEnabled, true
	}
	return false, false
}

func PrefetchedStickyGroupIDFromContext(ctx context.Context) (int64, bool) {
	if md := metadataFromContext(ctx); md != nil && md.PrefetchedStickyGroupID != nil {
		return *md.PrefetchedStickyGroupID, true
	}
	return 0, false
}

func PrefetchedStickyAccountIDFromContext(ctx context.Context) (int64, bool) {
	if md := metadataFromContext(ctx); md != nil && md.PrefetchedStickyAccountID != nil {
		return *md.PrefetchedStickyAccountID, true
	}
	return 0, false
}

func SingleAccountRetryFromContext(ctx context.Context) (bool, bool) {
	if md := metadataFromContext(ctx); md != nil && md.SingleAccountRetry != nil {
		return *md.SingleAccountRetry, true
	}
	return false, false
}

func AccountSwitchCountFromContext(ctx context.Context) (int, bool) {
	if md := metadataFromContext(ctx); md != nil && md.AccountSwitchCount != nil {
		return *md.AccountSwitchCount, true
	}
	return 0, false
}
