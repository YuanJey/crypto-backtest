package compiler

import (
	"encoding/json"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type Plan struct {
	encoded   []byte
	factories []func() (indicator.RuntimeIndicator, error)
}

// ExecutionPlan is a private-to-the-run copy of immutable compiled metadata.
// Plan itself exposes neither writable slices nor mutable runtime instances.
type ExecutionPlan struct {
	Logic           strategy.LogicDefinition
	Defaults        strategy.RunConfig
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
}

func (p Plan) Snapshot() ExecutionPlan {
	var snapshot ExecutionPlan
	// encoded is produced only by the validated compiler and is never exposed.
	if len(p.encoded) > 0 {
		if err := json.Unmarshal(p.encoded, &snapshot); err != nil {
			panic("invalid internal compiled plan: " + err.Error())
		}
	}
	return snapshot
}

func (p Plan) Definition() strategy.Definition {
	snapshot := p.Snapshot()
	return strategy.ComposeV1(snapshot.Logic, snapshot.Defaults)
}

func (p Plan) NewIndicatorRuntimes() ([]indicator.RuntimeIndicator, error) {
	runtimes := make([]indicator.RuntimeIndicator, 0, len(p.factories))
	for _, factory := range p.factories {
		runtime, err := factory()
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, runtime)
	}
	return runtimes, nil
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
