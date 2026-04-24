package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
)

// DeriveSessionHashFromSeed computes the current-format sticky-session hash
// from an arbitrary seed string.
func DeriveSessionHashFromSeed(seed string) string {
	normalized := strings.TrimSpace(seed)
	if normalized == "" {
		return ""
	}
	return fmt.Sprintf("%016x", xxhash.Sum64String(normalized))
}

func (s *OpenAIGatewayService) openAISessionCacheKey(sessionHash string) string {
	normalized := strings.TrimSpace(sessionHash)
	if normalized == "" {
		return ""
	}
	return "openai:" + normalized
}

func (s *OpenAIGatewayService) getStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, nil
	}
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" {
		return 0, nil
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

func (s *OpenAIGatewayService) setStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if s == nil || s.cache == nil || accountID <= 0 {
		return nil
	}
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" {
		return nil
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, ttl)
}

func (s *OpenAIGatewayService) refreshStickySessionTTL(ctx context.Context, groupID *int64, sessionHash string, ttl time.Duration) error {
	if s == nil || s.cache == nil {
		return nil
	}
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" {
		return nil
	}
	return s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), cacheKey, ttl)
}

func (s *OpenAIGatewayService) deleteStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) error {
	if s == nil || s.cache == nil {
		return nil
	}
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" {
		return nil
	}
	return s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}
