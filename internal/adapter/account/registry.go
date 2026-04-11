package account

import (
	"context"
	"errors"

	"lune/internal/config"
	"lune/internal/execution"
)

var ErrAdapterNotImplemented = errors.New("account adapter is not implemented")

type Adapter interface {
	ID() string
	Prepare(context.Context, execution.Request, execution.Plan, config.Platform, config.Account) (*execution.PreparedExecution, error)
	Execute(context.Context, *execution.PreparedExecution) (*execution.RawResult, error)
	Normalize(context.Context, *execution.PreparedExecution, *execution.RawResult) (*execution.GatewayResponse, error)
	Classify(*execution.RawResult, error) execution.Outcome
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{
		adapters: make(map[string]Adapter, len(adapters)),
	}
	for _, adapter := range adapters {
		registry.Register(adapter)
	}
	return registry
}

func (r *Registry) Register(adapter Adapter) {
	if r == nil || adapter == nil {
		return
	}
	r.adapters[adapter.ID()] = adapter
}

func (r *Registry) ForPlatform(platform config.Platform) (Adapter, bool) {
	if r == nil {
		return nil, false
	}

	adapterID := platform.Adapter
	if adapterID == "" {
		adapterID = platform.Type
	}

	adapter, ok := r.adapters[adapterID]
	return adapter, ok
}
