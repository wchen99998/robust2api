package provider

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

type Registry struct {
	adapters map[string]core.ProviderAdapter
}

func NewRegistry(adapters ...core.ProviderAdapter) *Registry {
	r := &Registry{adapters: make(map[string]core.ProviderAdapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil || adapter.Provider() == "" {
			continue
		}
		r.adapters[adapter.Provider()] = adapter
	}
	return r
}

func (r *Registry) Get(platform string) (core.ProviderAdapter, error) {
	if r == nil {
		return nil, fmt.Errorf("provider registry is nil")
	}
	adapter, ok := r.adapters[platform]
	if !ok {
		return nil, fmt.Errorf("provider adapter not registered: %s", platform)
	}
	return adapter, nil
}
