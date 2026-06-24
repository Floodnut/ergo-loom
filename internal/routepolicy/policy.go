package routepolicy

import (
	"errors"
	"sort"

	"github.com/Floodnut/ergo-loom/internal/core"
)

// Registry maps policy names to RouteSelectionPolicy implementations.
type Registry struct {
	policies map[string]core.RouteSelectionPolicy
}

func NewRegistry() Registry {
	r := Registry{policies: make(map[string]core.RouteSelectionPolicy)}
	r.Register(ManualPolicy{})
	r.Register(FailoverPolicy{})
	return r
}

func (r Registry) Register(p core.RouteSelectionPolicy) {
	r.policies[p.Name()] = p
}

func (r Registry) List() []string {
	names := make([]string, 0, len(r.policies))
	for name := range r.policies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r Registry) GetOrDefault(name string) core.RouteSelectionPolicy {
	if p, ok := r.policies[name]; ok {
		return p
	}
	return r.policies["manual"]
}

// ManualPolicy uses the session's active route. If the request includes an
// explicit per-message routeID, that takes priority (one-shot override).
// Falls back to the highest-priority candidate if no active route is set.
type ManualPolicy struct{}

func (ManualPolicy) Name() string { return "manual" }

func (ManualPolicy) Select(ctx core.RouteSelectionContext) (string, string, error) {
	// Per-message override wins.
	if ctx.RequestedRouteID != "" {
		return ctx.RequestedRouteID, ctx.RequestedModelID, nil
	}
	// Session active route.
	if ctx.Session.ActiveRouteID != "" {
		return ctx.Session.ActiveRouteID, ctx.Session.ActiveModelID, nil
	}
	// Fall back to highest-priority candidate.
	if len(ctx.Candidates) > 0 {
		c := ctx.Candidates[0]
		return c.RouteID, c.ModelID, nil
	}
	return "", "", errors.New("no route available for this session")
}

// FailoverPolicy tries the session's active route first. If it is not among
// the available candidates, it automatically selects the next one in priority
// order. Useful for resilience when a provider goes down.
type FailoverPolicy struct{}

func (FailoverPolicy) Name() string { return "failover" }

func (FailoverPolicy) Select(ctx core.RouteSelectionContext) (string, string, error) {
	// Per-message override always wins.
	if ctx.RequestedRouteID != "" {
		return ctx.RequestedRouteID, ctx.RequestedModelID, nil
	}
	// Try active route first.
	if ctx.Session.ActiveRouteID != "" {
		for _, c := range ctx.Candidates {
			if c.RouteID == ctx.Session.ActiveRouteID {
				return c.RouteID, c.ModelID, nil
			}
		}
		// Active route not in candidates — fall through to next available.
	}
	if len(ctx.Candidates) > 0 {
		c := ctx.Candidates[0]
		return c.RouteID, c.ModelID, nil
	}
	return "", "", errors.New("no route available for failover")
}
