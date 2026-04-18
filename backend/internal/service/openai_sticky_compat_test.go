package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetStickySessionAccountID_UsesCurrentHashOnly(t *testing.T) {
	cache := &stubGatewayCache{
		sessionBindings: map[string]int64{
			"openai:new-hash":    42,
			"openai:legacy-hash": 7,
		},
	}
	svc := &OpenAIGatewayService{cache: cache}

	accountID, err := svc.getStickySessionAccountID(context.Background(), nil, "new-hash")
	require.NoError(t, err)
	require.Equal(t, int64(42), accountID)
}

func TestSetStickySessionAccountID_WritesCurrentHashOnly(t *testing.T) {
	cache := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc := &OpenAIGatewayService{cache: cache}

	err := svc.setStickySessionAccountID(context.Background(), nil, "new-hash", 9, openaiStickySessionTTL)
	require.NoError(t, err)
	require.Equal(t, int64(9), cache.sessionBindings["openai:new-hash"])
	_, exists := cache.sessionBindings["openai:legacy-hash"]
	require.False(t, exists)
}
