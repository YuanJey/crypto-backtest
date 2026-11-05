package engine

import (
	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type additionRuntime struct {
	spec      *strategy.AdditionTriggerSpec
	sample    float64
	hasSample bool
}

func newAdditionRuntimes(rules []compiler.RulePlan) []additionRuntime {
	runtimes := make([]additionRuntime, len(rules))
	for index, rule := range rules {
		runtimes[index].spec = rule.AdditionTrigger
	}
	return runtimes
}

func resetAdditionRuntimes(runtimes []additionRuntime) {
	for index := range runtimes {
		runtimes[index].sample = 0
		runtimes[index].hasSample = false
	}
}

// Called at the fill, before updating indicators with the fill bar. Only the
// most recent completed indicator bar is visible at this point.
func snapshotAdditions(runtimes []additionRuntime, states []indicatorState, firstEntry bool) {
	if firstEntry {
		resetAdditionRuntimes(runtimes)
	}
	for index := range runtimes {
		runtime := &runtimes[index]
		if runtime.spec == nil || runtime.spec.Distance.Type != strategy.ExitDistanceATRMultiple {
			continue
		}
		distance := runtime.spec.Distance
		if distance.Sampling == strategy.ExitSamplingLive ||
			(distance.Sampling == strategy.ExitSamplingEntry && !firstEntry) {
			continue
		}
		// A missing new sample must not silently reuse an old each-entry sample.
		runtime.sample, runtime.hasSample = latestIndicatorValue(states, distance.Indicator, distance.Field)
	}
}

func additionHit(portfolio portfolio, action strategy.ActionSpec, runtime additionRuntime, bar market.Bar, states []indicatorState) bool {
	if runtime.spec == nil {
		return true
	}
	if portfolio.quantity == 0 || portfolio.currentSide() != action.Side {
		return false
	}
	spec := *runtime.spec
	anchor, ok := exitAnchorValue(portfolio, spec.Anchor, states)
	if !ok {
		return false
	}
	value, ok := spec.DistanceAt(action.AdditionIndex)
	if !ok {
		return false
	}
	distanceSpec := spec.Distance
	distanceSpec.Value = value
	distance, ok := distanceValue(anchor, distanceSpec, runtime.sample, runtime.hasSample, states)
	if !ok || !isPositiveFinite(distance) {
		return false
	}
	above := (action.Side == strategy.TradeSideShort && spec.Direction == strategy.AdditionAdverse) ||
		(action.Side == strategy.TradeSideLong && spec.Direction == strategy.AdditionFavorable)
	target := anchor - distance
	if above {
		target = anchor + distance
	}
	if !isPositiveFinite(target) {
		return false
	}
	price := bar.Close
	switch spec.TriggerPrice {
	case strategy.ExitTriggerHigh:
		price = bar.High
	case strategy.ExitTriggerLow:
		price = bar.Low
	}
	if above {
		return price >= target
	}
	return price <= target
}
