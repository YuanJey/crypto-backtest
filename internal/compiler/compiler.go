package compiler

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func Compile(definition strategy.Definition, registry *indicator.Registry) (Plan, error) {
	// Normalize may update nested policy/condition data. Own a complete copy
	// before entering the compatibility boundary.
	raw, err := json.Marshal(definition)
	if err != nil {
		return Plan{}, err
	}
	if err := json.Unmarshal(raw, &definition); err != nil {
		return Plan{}, err
	}
	normalized, err := strategy.Normalize(definition)
	if err != nil {
		return Plan{}, err
	}
	definition = normalized
	if err := strategy.Validate(definition, registry); err != nil {
		return Plan{}, err
	}
	baseTimeframeMS, err := market.ParseTimeframeMS(definition.Data.Timeframe)
	if err != nil {
		return Plan{}, err
	}

	orderedSpecs, err := orderIndicators(definition.Indicators)
	if err != nil {
		return Plan{}, err
	}
	indicators := make([]IndicatorPlan, 0, len(orderedSpecs))
	factories := make([]func() (indicator.RuntimeIndicator, error), 0, len(orderedSpecs))
	for _, spec := range orderedSpecs {
		indicatorDefinition, ok := registry.Definition(spec.Type)
		if !ok {
			return Plan{}, fmt.Errorf("unknown indicator type %q", spec.Type)
		}
		requiredInputs := 0
		for _, input := range indicatorDefinition.Inputs {
			if input.Required {
				requiredInputs++
			}
		}
		if len(spec.Inputs) < requiredInputs {
			return Plan{}, fmt.Errorf("indicator %q requires at least %d inputs", spec.ID, requiredInputs)
		}
		factory, err := registry.RuntimeFactory(spec.Type, spec.Params)
		if err != nil {
			return Plan{}, fmt.Errorf("indicator %q runtime: %w", spec.ID, err)
		}
		if _, err := factory(); err != nil {
			return Plan{}, fmt.Errorf("indicator %q runtime: %w", spec.ID, err)
		}
		factories = append(factories, factory)
		timeframe := spec.Timeframe
		if timeframe == "" {
			timeframe = definition.Data.Timeframe
		}
		timeframeMS, err := market.ParseTimeframeMS(timeframe)
		if err != nil {
			return Plan{}, err
		}
		outputs := make([]string, 0, len(indicatorDefinition.Outputs))
		for _, output := range indicatorDefinition.Outputs {
			outputs = append(outputs, output.Name)
		}
		indicators = append(indicators, IndicatorPlan{
			ID:          spec.ID,
			Type:        spec.Type,
			Timeframe:   timeframe,
			TimeframeMS: timeframeMS,
			Inputs:      spec.Inputs,
			Outputs:     outputs,
		})
	}

	rules := make([]RulePlan, 0, len(definition.Rules))
	for _, rule := range definition.Rules {
		if rule.Disabled {
			continue
		}
		rules = append(rules, RulePlan{
			ID:              rule.ID,
			SourceRuleID:    rule.SourceRuleID,
			Name:            rule.Name,
			Priority:        rule.Priority,
			When:            rule.When,
			Action:          rule.Action,
			AdditionTrigger: rule.AdditionTrigger,
		})
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			left, right := rules[i].SourceRuleID, rules[j].SourceRuleID
			if left == "" {
				left = rules[i].ID
			}
			if right == "" {
				right = rules[j].ID
			}
			if left != right {
				return left < right
			}
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Priority > rules[j].Priority
	})
	exitPolicies := append([]strategy.ExitPolicySpec(nil), definition.ExitPolicies...)
	sort.SliceStable(exitPolicies, func(i, j int) bool {
		if exitPolicies[i].Priority != exitPolicies[j].Priority {
			return exitPolicies[i].Priority > exitPolicies[j].Priority
		}
		if exitPolicies[i].Purpose != exitPolicies[j].Purpose {
			return exitPolicies[i].Purpose == strategy.ExitPurposeStopLoss
		}
		return exitPolicies[i].ID < exitPolicies[j].ID
	})

	logic, defaults, err := strategy.AdaptV1(definition)
	if err != nil {
		return Plan{}, err
	}
	encoded, err := json.Marshal(ExecutionPlan{
		Logic:           logic,
		Defaults:        defaults,
		BaseTimeframeMS: baseTimeframeMS,
		Indicators:      indicators,
		Rules:           rules,
		ExitPolicies:    exitPolicies,
	})
	if err != nil {
		return Plan{}, err
	}
	return Plan{encoded: encoded, factories: factories}, nil
}

func orderIndicators(specs []strategy.IndicatorSpec) ([]strategy.IndicatorSpec, error) {
	byID := make(map[string]strategy.IndicatorSpec, len(specs))
	indegree := make(map[string]int, len(specs))
	dependents := make(map[string][]string, len(specs))
	for _, spec := range specs {
		byID[spec.ID] = spec
		indegree[spec.ID] = 0
	}
	for _, spec := range specs {
		seen := map[string]struct{}{}
		for _, input := range spec.Inputs {
			if input.Scope != strategy.ValueScopeIndicator {
				continue
			}
			if _, ok := seen[input.Name]; ok {
				continue
			}
			seen[input.Name] = struct{}{}
			indegree[spec.ID]++
			dependents[input.Name] = append(dependents[input.Name], spec.ID)
		}
	}

	ordered := make([]strategy.IndicatorSpec, 0, len(specs))
	emitted := make(map[string]bool, len(specs))
	for len(ordered) < len(specs) {
		progress := false
		for _, spec := range specs {
			if emitted[spec.ID] || indegree[spec.ID] != 0 {
				continue
			}
			emitted[spec.ID] = true
			ordered = append(ordered, byID[spec.ID])
			for _, dependent := range dependents[spec.ID] {
				indegree[dependent]--
			}
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("indicator dependency graph contains a cycle")
		}
	}
	return ordered, nil
}
