package engine

import (
	"math"

	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type exitPolicyRuntime struct {
	spec            strategy.ExitPolicySpec
	sampledDistance float64
	hasSample       bool
	fired           bool
}

type exitCandidate struct {
	runtime *exitPolicyRuntime
	target  float64
}

func newExitPolicyRuntimes(specs []strategy.ExitPolicySpec) []exitPolicyRuntime {
	runtimes := make([]exitPolicyRuntime, 0, len(specs))
	for _, spec := range specs {
		if spec.Disabled {
			continue
		}
		runtimes = append(runtimes, exitPolicyRuntime{spec: spec})
	}
	return runtimes
}

func resetExitPolicyRuntimes(runtimes []exitPolicyRuntime) {
	for index := range runtimes {
		runtimes[index].sampledDistance = 0
		runtimes[index].hasSample = false
		runtimes[index].fired = false
	}
}

func snapshotExitPolicies(runtimes []exitPolicyRuntime, states []indicatorState, firstEntry bool) {
	if firstEntry {
		resetExitPolicyRuntimes(runtimes)
	}
	for index := range runtimes {
		runtime := &runtimes[index]
		if runtime.spec.Distance.Type != strategy.ExitDistanceATRMultiple {
			continue
		}
		sampling := runtime.spec.Distance.Sampling
		if sampling == strategy.ExitSamplingEntry && !firstEntry {
			continue
		}
		if sampling != strategy.ExitSamplingEntry && sampling != strategy.ExitSamplingEachEntry {
			continue
		}
		value, ok := latestIndicatorValue(states, runtime.spec.Distance.Indicator, runtime.spec.Distance.Field)
		if !ok {
			continue
		}
		runtime.sampledDistance = value
		runtime.hasSample = true
	}
}

func latestIndicatorValue(states []indicatorState, indicatorID, field string) (float64, bool) {
	for _, state := range states {
		if state.plan.ID != indicatorID {
			continue
		}
		value, ok := state.last[field]
		return value, ok && isUsableExitNumber(value)
	}
	return 0, false
}

func selectExitCandidate(
	portfolio portfolio,
	bar market.Bar,
	runtimes []exitPolicyRuntime,
	states []indicatorState,
) *exitCandidate {
	if portfolio.quantity == 0 {
		return nil
	}
	for index := range runtimes {
		runtime := &runtimes[index]
		if runtime.fired || runtime.spec.Side != portfolio.currentSide() {
			continue
		}
		target, ok := exitTarget(portfolio, *runtime, states)
		if !ok || !exitPolicyHit(runtime.spec, target, bar) {
			continue
		}
		return &exitCandidate{runtime: runtime, target: target}
	}
	return nil
}

func exitTarget(portfolio portfolio, runtime exitPolicyRuntime, states []indicatorState) (float64, bool) {
	anchor, ok := exitAnchorValue(portfolio, runtime.spec.Anchor.Type)
	if !ok {
		return 0, false
	}
	distance, ok := exitDistanceValue(anchor, runtime, states)
	if !ok || distance <= 0 {
		return 0, false
	}
	direction := 1.0
	if (runtime.spec.Side == strategy.TradeSideLong && runtime.spec.Purpose == strategy.ExitPurposeStopLoss) ||
		(runtime.spec.Side == strategy.TradeSideShort && runtime.spec.Purpose == strategy.ExitPurposeTakeProfit) {
		direction = -1
	}
	target := anchor + direction*distance
	return target, isUsableExitNumber(target) && target > 0
}

func exitAnchorValue(portfolio portfolio, anchorType strategy.ExitAnchorType) (float64, bool) {
	var value float64
	switch anchorType {
	case strategy.ExitAnchorAverageEntry:
		value = portfolio.avgPrice
	case strategy.ExitAnchorFirstEntry:
		value = portfolio.firstEntryPrice
	case strategy.ExitAnchorLastEntry:
		value = portfolio.lastEntryPrice
	case strategy.ExitAnchorHighestSinceEntry:
		value = portfolio.highestSinceEntry
	case strategy.ExitAnchorLowestSinceEntry:
		value = portfolio.lowestSinceEntry
	default:
		return 0, false
	}
	return value, isUsableExitNumber(value) && value > 0
}

func exitDistanceValue(anchor float64, runtime exitPolicyRuntime, states []indicatorState) (float64, bool) {
	spec := runtime.spec.Distance
	switch spec.Type {
	case strategy.ExitDistancePercent:
		return anchor * spec.Value, true
	case strategy.ExitDistanceFixedAmount:
		return spec.Value, true
	case strategy.ExitDistanceATRMultiple:
		var indicatorValue float64
		var ok bool
		if spec.Sampling == strategy.ExitSamplingLive {
			indicatorValue, ok = latestIndicatorValue(states, spec.Indicator, spec.Field)
		} else {
			indicatorValue, ok = runtime.sampledDistance, runtime.hasSample
		}
		if !ok || !isUsableExitNumber(indicatorValue) || indicatorValue <= 0 {
			return 0, false
		}
		return indicatorValue * spec.Value, true
	default:
		return 0, false
	}
}

func exitPolicyHit(spec strategy.ExitPolicySpec, target float64, bar market.Bar) bool {
	trigger := bar.Close
	switch spec.TriggerPrice {
	case strategy.ExitTriggerHigh:
		trigger = bar.High
	case strategy.ExitTriggerLow:
		trigger = bar.Low
	}
	if exitTriggersAbove(spec) {
		return trigger >= target
	}
	return trigger <= target
}

func exitTriggersAbove(spec strategy.ExitPolicySpec) bool {
	return (spec.Side == strategy.TradeSideLong && spec.Purpose == strategy.ExitPurposeTakeProfit) ||
		(spec.Side == strategy.TradeSideShort && spec.Purpose == strategy.ExitPurposeStopLoss)
}

func exitTriggerFillPrice(spec strategy.ExitPolicySpec, target float64, bar market.Bar) float64 {
	if spec.TriggerPrice == strategy.ExitTriggerClose {
		return bar.Close
	}
	if exitTriggersAbove(spec) && bar.Open >= target {
		return bar.Open
	}
	if !exitTriggersAbove(spec) && bar.Open <= target {
		return bar.Open
	}
	return target
}

func isUsableExitNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
