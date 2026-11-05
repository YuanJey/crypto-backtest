package engine

import (
	"context"
	"math"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func additionTrigger(distance strategy.ExitDistanceSpec) *strategy.AdditionTriggerSpec {
	return &strategy.AdditionTriggerSpec{
		Anchor: strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry}, Distance: distance,
		Direction: strategy.AdditionAdverse, TriggerPrice: strategy.ExitTriggerClose,
	}
}

func TestAdditionAnchorsDirectionsAndPrices(t *testing.T) {
	p := newPortfolio(strategy.MarketTypePerpetual, 10_000)
	p.quantity, p.avgPrice, p.firstEntryPrice, p.lastEntryPrice = 1, 100, 110, 90
	for _, tc := range []struct {
		name      string
		side      strategy.TradeSide
		direction strategy.AdditionDirection
		anchor    strategy.ExitAnchorType
		target    float64
		above     bool
	}{
		{"long average adverse", strategy.TradeSideLong, strategy.AdditionAdverse, strategy.ExitAnchorAverageEntry, 95, false},
		{"long first favorable", strategy.TradeSideLong, strategy.AdditionFavorable, strategy.ExitAnchorFirstEntry, 115.5, true},
		{"short last adverse", strategy.TradeSideShort, strategy.AdditionAdverse, strategy.ExitAnchorLastEntry, 94.5, true},
		{"short average favorable", strategy.TradeSideShort, strategy.AdditionFavorable, strategy.ExitAnchorAverageEntry, 95, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.quantity = 1
			if tc.side == strategy.TradeSideShort {
				p.quantity = -1
			}
			spec := additionTrigger(strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.05})
			spec.Anchor.Type, spec.Direction = tc.anchor, tc.direction
			action := strategy.ActionSpec{Side: tc.side, AdditionIndex: 1}
			for _, price := range []strategy.ExitTriggerPrice{strategy.ExitTriggerClose, strategy.ExitTriggerHigh, strategy.ExitTriggerLow} {
				spec.TriggerPrice = price
				outside := tc.target + 0.01
				if tc.above {
					outside = tc.target - 0.01
				}
				bar := market.Bar{Close: outside, High: outside, Low: outside}
				if additionHit(p, action, additionRuntime{spec: spec}, bar, nil) {
					t.Fatal("price outside threshold must not trigger")
				}
				switch price {
				case strategy.ExitTriggerClose:
					bar.Close = tc.target
				case strategy.ExitTriggerHigh:
					bar.High = tc.target
				case strategy.ExitTriggerLow:
					bar.Low = tc.target
				}
				if !additionHit(p, action, additionRuntime{spec: spec}, bar, nil) {
					t.Fatal("selected trigger price at threshold must trigger")
				}
			}
		})
	}
}

func TestRunAdditionSpacingAndExtraCondition(t *testing.T) {
	def := actionSizingDefinition()
	def.Position.PositionLeverage = 1
	def.Rules = []strategy.RuleSpec{
		sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.02}),
		sizingRule("add1", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.02, AdditionIndex: 1}),
		sizingRule("add2", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.02, AdditionIndex: 2}),
	}
	for i := 1; i < 3; i++ {
		def.Rules[i].AdditionTrigger = additionTrigger(strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.05})
		def.Rules[i].AdditionTrigger.Anchor.Type = strategy.ExitAnchorLastEntry
		def.Rules[i].AdditionTrigger.Spacing = &strategy.AdditionSpacingSpec{Mode: strategy.AdditionSpacingGeometric, Value: 2}
	}
	def.Rules[1].When = strategy.Market("close").LT(94)
	bars := constantPriceBars(7, 100)
	for i, value := range []float64{100, 95, 93, 92, 82.5, 82, 82} {
		bars[i].Open, bars[i].Close, bars[i].High, bars[i].Low = value, value, value, value
	}
	bars[1].Open = 100 // Entry fills here; close touches spacing but fails extra condition.
	result := runActionSizing(t, def, bars)
	if len(result.Fills) != 3 {
		t.Fatalf("want entry and two additions: %+v", result.Orders)
	}
	for i, want := range []int{1, 3, 5} {
		if result.Fills[i].TimeMS != int64(want)*900_000 {
			t.Fatalf("fill %d=%+v, want next open of bar %d", i, result.Fills[i], want)
		}
	}
	if result.Orders[1].SignalTimeMS != 3*900_000 || result.Fills[1].Price != 92 {
		t.Fatal("addition must use signal close followed by next open")
	}
}

func atrAdditionDefinition(sampling strategy.ExitSampling, timeframe string, period int) strategy.Definition {
	def := actionSizingDefinition()
	def.Position.PositionLeverage = 1
	def.Indicators = []strategy.IndicatorSpec{{ID: "atr", Type: "atr", Timeframe: timeframe, Params: []byte(`{"period":1}`)}}
	if period == 3 {
		def.Indicators[0].Params = []byte(`{"period":3}`)
	}
	def.Rules = []strategy.RuleSpec{
		sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.01}),
		sizingRule("add", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.01, AdditionIndex: 1}),
	}
	def.Rules[1].AdditionTrigger = additionTrigger(strategy.ExitDistanceSpec{
		Type: strategy.ExitDistanceATRMultiple, Value: 1, Indicator: "atr", Sampling: sampling,
	})
	return def
}

func TestRunAdditionATRUsesEntrySnapshotOrCurrentClosedValue(t *testing.T) {
	bars := constantPriceBars(6, 100)
	bars[1].High, bars[1].Low = 101, 99                                     // ATR needs a previous close; entry sample = 2.
	bars[2].High, bars[2].Low, bars[2].Close = 110, 95, 97                  // Current ATR = 15.
	bars[3].Open, bars[3].High, bars[3].Low, bars[3].Close = 96, 97, 95, 95 // ATR = 2.
	bars[4].Open = 94
	for _, tc := range []struct {
		sampling strategy.ExitSampling
		fillBar  int
	}{
		{strategy.ExitSamplingEntry, 3},
		{strategy.ExitSamplingEachEntry, 3},
		{strategy.ExitSamplingLive, 4},
	} {
		t.Run(string(tc.sampling), func(t *testing.T) {
			def := atrAdditionDefinition(tc.sampling, "15m", 1)
			def.Rules[0].When = strategy.Indicator("atr").GT(0)
			result := runActionSizing(t, def, bars)
			if len(result.Fills) != 2 || result.Fills[1].TimeMS != int64(tc.fillBar)*900_000 {
				t.Fatalf("sampling %s fills=%+v, want addition bar %d", tc.sampling, result.Fills, tc.fillBar)
			}
		})
	}
}

func TestRunAdditionHigherTimeframeAndWarmup(t *testing.T) {
	bars := constantPriceBars(8, 100)
	bars[0].High, bars[0].Low = 101, 99
	bars[2].High, bars[2].Low = 101, 99
	bars[3].Close = 99 // ATR = 2 after the second completed 30m candle.
	bars[4].High, bars[4].Low, bars[4].Close = 110, 95, 97
	bars[5].Open = 96
	for _, sampling := range []strategy.ExitSampling{strategy.ExitSamplingEntry, strategy.ExitSamplingLive} {
		def := atrAdditionDefinition(sampling, "30m", 1)
		def.Rules[0].When = strategy.Market("close").LT(100)
		result := runActionSizing(t, def, bars)
		if len(result.Fills) != 2 || result.Fills[1].TimeMS != 5*900_000 {
			t.Fatalf("must use completed 30m ATR, not fill-bar future range: %+v", result.Fills)
		}
	}
	def := atrAdditionDefinition(strategy.ExitSamplingEntry, "15m", 3)
	// First entry is before ATR warm-up; later ATR cannot replace this snapshot.
	result := runActionSizing(t, def, bars)
	if len(result.Fills) != 1 {
		t.Fatalf("missing entry ATR must not be fabricated later: %+v", result.Fills)
	}
}

func TestAdditionSnapshotRefreshResetAndPartialClose(t *testing.T) {
	states := []indicatorState{{plan: compiler.IndicatorPlan{ID: "atr"}, last: map[string]float64{"value": 2}}}
	rules := make([]compiler.RulePlan, 3)
	for i, sampling := range []strategy.ExitSampling{strategy.ExitSamplingEntry, strategy.ExitSamplingEachEntry, strategy.ExitSamplingLive} {
		rules[i].AdditionTrigger = additionTrigger(strategy.ExitDistanceSpec{Type: strategy.ExitDistanceATRMultiple, Value: 1, Indicator: "atr", Field: "value", Sampling: sampling})
	}
	runtimes := newAdditionRuntimes(rules)
	snapshotAdditions(runtimes, states, true)
	states[0].last["value"] = 4
	snapshotAdditions(runtimes, states, false)
	if runtimes[0].sample != 2 || runtimes[1].sample != 4 || runtimes[2].hasSample {
		t.Fatalf("invalid ATR snapshots: %+v", runtimes)
	}
	p := newPortfolio(strategy.MarketTypePerpetual, 10_000)
	p.open(strategy.TradeSideLong, 100, 0.01, 0, true)
	p.close(strategy.TradeSideLong, 100, 0.5, 0)
	if p.entryCount != 1 || p.quantity == 0 || runtimes[0].sample != 2 {
		t.Fatal("partial close must retain the entry index and sample")
	}
	p.close(strategy.TradeSideLong, 100, 1, 0)
	resetAdditionRuntimes(runtimes)
	if p.entryCount != 0 || runtimes[0].hasSample || runtimes[1].hasSample {
		t.Fatal("full close must reset the cycle")
	}
	states[0].last["value"] = 6
	snapshotAdditions(runtimes, states, true)
	if runtimes[0].sample != 6 || runtimes[1].sample != 6 {
		t.Fatal("re-entry must sample the new cycle")
	}
	states[0].last["value"] = math.NaN()
	snapshotAdditions(runtimes, states, false)
	if runtimes[1].hasSample {
		t.Fatal("missing each-entry ATR cannot reuse a stale sample")
	}
}

func TestRunAdditionSnapshotsResetAcrossFullClose(t *testing.T) {
	def := atrAdditionDefinition(strategy.ExitSamplingEntry, "15m", 1)
	def.Rules[0].When = strategy.Indicator("atr").GT(0)
	closeRule := sizingRule("close", strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: 1})
	closeRule.Priority = 10
	closeRule.When = strategy.Market("close").EQ(101)
	def.Rules = append(def.Rules, closeRule)
	bars := constantPriceBars(9, 100)
	bars[1].High, bars[1].Low = 101, 99 // First cycle ATR=2.
	bars[2].Close, bars[2].Low = 97, 97
	bars[3].Close, bars[3].High = 101, 101
	bars[4].High, bars[4].Low = 105, 95 // Close fills, re-entry signals; new ATR=10.
	bars[5].Close, bars[5].Low = 97, 97 // New cycle must not use old ATR=2.
	bars[6].Close, bars[6].Low = 89, 89
	bars[7].Open = 88
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 5 || result.Fills[4].RuleID != "add" || result.Fills[4].TimeMS != 7*900_000 {
		t.Fatalf("new cycle must reset addition index and ATR sample: %+v", result.Fills)
	}
}

func TestRunAdditionTracksUpdatedAverageAfterFill(t *testing.T) {
	def := actionSizingDefinition()
	def.Position.PositionLeverage = 1
	def.Rules = []strategy.RuleSpec{
		sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.01}),
		sizingRule("add1", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.01, AdditionIndex: 1}),
		sizingRule("add2", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.01, AdditionIndex: 2}),
	}
	for i := 1; i < 3; i++ {
		def.Rules[i].AdditionTrigger = additionTrigger(strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.05})
	}
	bars := constantPriceBars(5, 100)
	bars[1].Close, bars[1].Low = 95, 95
	bars[2].Open, bars[2].Close, bars[2].Low = 90, 91, 90 // Updated avg ~=94.74; threshold ~=90.00.
	bars[3].Close, bars[3].Low = 89, 89
	bars[4].Open = 88
	result := runActionSizing(t, def, bars)
	if len(result.Fills) != 3 || result.Fills[2].TimeMS != 4*900_000 {
		t.Fatalf("second addition must use updated weighted average: %+v", result.Fills)
	}
}

func TestRunAdditionRetainsATRAfterPartialClose(t *testing.T) {
	def := atrAdditionDefinition(strategy.ExitSamplingEntry, "15m", 1)
	def.Rules[0].When = strategy.Indicator("atr").GT(0)
	closeRule := sizingRule("partial", strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: 0.5})
	closeRule.Priority = 10
	closeRule.When = strategy.Market("close").EQ(101)
	def.Rules = append(def.Rules, closeRule)
	bars := constantPriceBars(6, 100)
	bars[1].High, bars[1].Low = 101, 99 // Entry ATR=2.
	bars[2].High, bars[2].Low, bars[2].Close = 110, 90, 101
	bars[3].Close, bars[3].Low = 97, 97 // Partial fills, then addition uses retained ATR=2.
	bars[4].Open = 96
	result := runActionSizing(t, def, bars)
	if len(result.Fills) != 3 || result.Fills[1].RuleID != "partial" ||
		result.Fills[2].RuleID != "add" || result.Fills[2].TimeMS != 4*900_000 {
		t.Fatalf("partial close must retain addition index and ATR: %+v", result.Fills)
	}
}
