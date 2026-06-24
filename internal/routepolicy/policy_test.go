package routepolicy

import (
	"testing"

	"github.com/Floodnut/ergo-loom/internal/core"
)

func TestManualPolicyFallsBackWhenActiveRouteIsNotCandidate(t *testing.T) {
	routeID, modelID, err := ManualPolicy{}.Select(core.RouteSelectionContext{
		Session: core.Session{
			ActiveRouteID: "failed-route",
			ActiveModelID: "failed-model",
		},
		Candidates: []core.RouteCandidate{
			{RouteID: "fallback-route", ModelID: "fallback-model", Priority: 10},
		},
	})
	if err != nil {
		t.Fatalf("select route: %v", err)
	}
	if routeID != "fallback-route" || modelID != "fallback-model" {
		t.Fatalf("expected fallback candidate, got route=%q model=%q", routeID, modelID)
	}
}

func TestFailoverPolicyReturnsCandidateModel(t *testing.T) {
	routeID, modelID, err := FailoverPolicy{}.Select(core.RouteSelectionContext{
		Session: core.Session{
			ActiveRouteID: "failed-route",
			ActiveModelID: "failed-model",
		},
		Candidates: []core.RouteCandidate{
			{RouteID: "fallback-route", ModelID: "fallback-model", Priority: 10},
		},
	})
	if err != nil {
		t.Fatalf("select route: %v", err)
	}
	if routeID != "fallback-route" || modelID != "fallback-model" {
		t.Fatalf("expected fallback candidate, got route=%q model=%q", routeID, modelID)
	}
}
