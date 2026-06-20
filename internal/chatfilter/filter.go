package chatfilter

import (
	"context"
	"strings"
)

type Decision string

const (
	DecisionAllow   Decision = "allow"
	DecisionBlock   Decision = "block"
	DecisionRewrite Decision = "rewrite"
)

type Input struct {
	SessionID      string
	Content        string
	RouteID        string
	ModelID        string
	ThinkingEffort string
}

type Result struct {
	Decision Decision
	Content  string
	Reason   string
	Metadata map[string]string
}

type Filter interface {
	Apply(ctx context.Context, input Input) (Result, error)
}

type Chain struct {
	filters []Filter
}

func NewChain(filters ...Filter) Chain {
	return Chain{filters: filters}
}

func (c Chain) Apply(ctx context.Context, input Input) (Result, error) {
	current := input
	if strings.TrimSpace(current.Content) == "" {
		return Result{Decision: DecisionBlock, Reason: "message content is required"}, nil
	}
	for _, filter := range c.filters {
		result, err := filter.Apply(ctx, current)
		if err != nil {
			return Result{}, err
		}
		switch result.Decision {
		case DecisionBlock:
			return result, nil
		case DecisionRewrite:
			current.Content = strings.TrimSpace(result.Content)
		}
	}
	return Result{Decision: DecisionAllow, Content: strings.TrimSpace(current.Content)}, nil
}

type IdentityFilter struct{}

func (IdentityFilter) Apply(_ context.Context, input Input) (Result, error) {
	return Result{Decision: DecisionAllow, Content: input.Content}, nil
}
