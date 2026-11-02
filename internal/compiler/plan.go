package compiler

import (
	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

type Plan struct {
	Strategy        strategy.Definition
	BaseTimeframeMS int64
	Indicators      []IndicatorPlan
	Rules           []RulePlan
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
	ID       string
	Name     string
	Priority int
	When     strategy.Expression
	Action   strategy.ActionSpec
}
