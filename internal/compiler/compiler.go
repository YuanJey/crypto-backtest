package compiler

import (
	"fmt"
	"sort"

	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/market"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

func Compile(definition strategy.Definition, registry *indicator.Registry) (Plan, error) {
	if err := strategy.Validate(definition, registry); err != nil {
		return Plan{}, err
	}
	baseTimeframeMS, err := market.ParseTimeframeMS(definition.Data.Timeframe)
	if err != nil {
		return Plan{}, err
	}

	indicators := make([]IndicatorPlan, 0, len(definition.Indicators))
	for _, spec := range definition.Indicators {
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
		runtime, err := registry.BuildRuntime(spec.Type, spec.Params)
		if err != nil {
			return Plan{}, fmt.Errorf("indicator %q runtime: %w", spec.ID, err)
		}
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
			Runtime:     runtime,
		})
	}

	rules := make([]RulePlan, 0, len(definition.Rules))
	for _, rule := range definition.Rules {
		if rule.Disabled {
			continue
		}
		rules = append(rules, RulePlan{
			ID:       rule.ID,
			Name:     rule.Name,
			Priority: rule.Priority,
			When:     rule.When,
			Action:   rule.Action,
		})
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Priority > rules[j].Priority
	})

	return Plan{
		Strategy:        definition,
		BaseTimeframeMS: baseTimeframeMS,
		Indicators:      indicators,
		Rules:           rules,
	}, nil
}
