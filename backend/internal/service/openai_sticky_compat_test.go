package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeriveSessionHashFromSeed(t *testing.T) {
	require.NotEmpty(t, DeriveSessionHashFromSeed("session-1"))
	require.Equal(t, "", DeriveSessionHashFromSeed("   "))
}

func TestGetStickySessionAccountID_PrimaryKeyOnly(t *testing.T) {
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:new-hash": 42,
		},
	}
	svc := &OpenAIGatewayService{cache: cache}

	accountID, err := svc.getStickySessionAccountID(context.Background(), nil, "new-hash")
	require.NoError(t, err)
	require.Equal(t, int64(42), accountID)
}

func TestSetStickySessionAccountID_PrimaryKeyOnly(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{cache: cache}

	err := svc.setStickySessionAccountID(context.Background(), nil, "new-hash", 9, openaiStickySessionTTL)
	require.NoError(t, err)
	require.Equal(t, int64(9), cache.sessionBindings["openai:new-hash"])
	require.Len(t, cache.sessionBindings, 1)
}
