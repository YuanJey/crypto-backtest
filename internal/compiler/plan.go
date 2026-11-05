package compiler

import (
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type Plan struct {
	Strategy        strategy.Definition
	BaseTimeframeMS int64
	Indicators      []IndicatorPlan
	Rules           []RulePlan
	ExitPolicies    []strategy.ExitPolicySpec
}

type IndicatorPlan struct {
	ID          string
	Type        string
	Timeframe   string
	TimeframeMS int64
	Inputs      []strategy.ValueRef
	Outputs     []string
	Runtime     indicator.RuntimeIndicator
}

type RulePlan struct {
	ID              string
	SourceRuleID    string
	Name            string
	Priority        int
	When            strategy.Expression
	Action          strategy.ActionSpec
	AdditionTrigger *strategy.AdditionTriggerSpec
}
