package engine

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestHistoricalPositionFieldsUseSnapshotAndHistoricalMark(t *testing.T) {
	for _, side := range []float64{1, -1} {
		t.Run(map[float64]string{1: "long", -1: "short"}[side], func(t *testing.T) {
			past := portfolio{
				marketType: strategy.MarketTypePerpetual, cash: 1000,
				quantity: side * 2, avgPrice: 100, lastEntryPrice: 105, entryCount: 2,
			}
			current := portfolio{
				marketType: strategy.MarketTypePerpetual, cash: 500,
				quantity: side * 7, avgPrice: 200, lastEntryPrice: 210, entryCount: 5,
			}
			history := newPositionHistory([]compiler.RulePlan{{
				When: strategy.Position("quantity").Offset(-1).EQ(0),
			}}, 3)
			history.record(0, past)
			// Later changes to the source value must not affect the snapshot.
			past.quantity = 99
			ctx := evalContext{
				bars: []market.Bar{{Close: 110}, {Close: 250}}, barIndex: 1,
				portfolio: current, positionHistory: history,
			}
			want := map[string]float64{
				"quantity": side * 2, "abs_quantity": 2,
				"entry_price": 100, "avg_entry_price": 100, "last_entry_price": 105,
				"entry_count": 2, "side": side, "unrealized_pnl_pct": side * 0.1,
				"actual_leverage": 220 / (1000 + side*20),
			}
			for field, expected := range want {
				value, ok, err := evalRef(strategy.ValueRef{
					Scope: strategy.ValueScopePosition, Name: "current", Field: field, Offset: -1,
				}, ctx, 0)
				if err != nil || !ok || math.Abs(value-expected) > 1e-12 {
					t.Fatalf("%s = %v, %v, %v; want %v", field, value, ok, err, expected)
				}
			}
		})
	}
}

func TestHistoricalPositionMissingNeverFallsBackToCurrent(t *testing.T) {
	minInt := -int(^uint(0)>>1) - 1
	maxInt := int(^uint(0) >> 1)
	ctx := evalContext{
		bars: testBars(3), barIndex: 2,
		portfolio: portfolio{quantity: 99},
	}
	for _, offsets := range [][2]int{{-1, 0}, {minInt, 0}, {-1, minInt}, {0, minInt}, {maxInt, 0}, {0, maxInt}} {
		value, ok, err := evalRef(strategy.ValueRef{
			Scope: strategy.ValueScopePosition, Name: "current", Field: "quantity", Offset: offsets[0],
		}, ctx, offsets[1])
		if err != nil || ok || !math.IsNaN(value) {
			t.Fatalf("offsets %v = %v, %v, %v; want unavailable", offsets, value, ok, err)
		}
	}
	value, ok, err := evalRef(strategy.ValueRef{
		Scope: strategy.ValueScopePosition, Name: "current", Field: "quantity",
	}, ctx, 0)
	if err != nil || !ok || value != 99 {
		t.Fatalf("offset zero = %v, %v, %v; want current quantity", value, ok, err)
	}
	expression := strategy.Position("quantity").Offset(-1).GT(1)
	matched, err := evalBool(expression, ctx)
	if err != nil || matched {
		t.Fatalf("missing history comparison = %v, %v; want false", matched, err)
	}
}

func TestPositionHistoryCapacityEvictionAndIsolation(t *testing.T) {
	minInt := -int(^uint(0)>>1) - 1
	cases := []struct {
		name     string
		when     strategy.Expression
		capacity int
	}{
		{"current_only", strategy.Position("quantity").EQ(0), 0},
		{"market_history", strategy.Market("close").Offset(-3).GT(1), 0},
		{"nested_maximum", strategy.And(
			strategy.Position("quantity").Offset(-1).EQ(0),
			strategy.Position("side").Offset(-3).EQ(0),
		), 3},
		{"unavailable_min_int", strategy.Position("quantity").Offset(minInt).EQ(0), 0},
		{"unavailable_run_length", strategy.Position("quantity").Offset(-4).EQ(0), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := newPositionHistory([]compiler.RulePlan{{When: tc.when}}, 4)
			if tc.capacity == 0 {
				if history != nil {
					t.Fatalf("unneeded history allocated: %+v", history)
				}
				return
			}
			if len(history.snapshots) != tc.capacity {
				t.Fatalf("capacity = %d, want %d", len(history.snapshots), tc.capacity)
			}
			if _, ok := history.lookup(0); ok {
				t.Fatal("unrecorded zero-index slot must not be available")
			}
			for index := 0; index < 4; index++ {
				history.record(index, portfolio{quantity: float64(index + 1)})
			}
			if _, ok := history.lookup(0); ok {
				t.Fatal("evicted snapshot must not be available")
			}
			p, ok := history.lookup(1)
			if !ok || p.quantity != 2 {
				t.Fatalf("retained snapshot = %+v, %v", p, ok)
			}
			other := newPositionHistory([]compiler.RulePlan{{When: tc.when}}, 4)
			if _, ok := other.lookup(1); ok {
				t.Fatal("a separate run inherited history")
			}
		})
	}
}

// Every phase verifies historical fields through the production rule evaluator.
// Rules signal on closes 0..5 and fill at opens 1..6: enter, add, partial close,
// full close, reenter, close. The previous close must retain its original state.
func TestRunHistoryAcrossEntryAdditionPartialCloseAndReentry(t *testing.T) {
	for _, side := range []strategy.TradeSide{strategy.TradeSideLong, strategy.TradeSideShort} {
		t.Run(string(side), func(t *testing.T) {
			def := baseDefinition()
			def.Execution.Fee.TakerRate = 0
			def.Risk.MaxDrawdownHaltPct = 0
			enabled := true
			def.Behavior.AllowAdditions = &enabled
			def.Rules = []strategy.RuleSpec{
				{ID: "enter", When: strategy.Market("trade_count").EQ(0),
					Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: side, SizePct: 0.1}},
				{ID: "add", When: strategy.And(
					strategy.Market("trade_count").EQ(1),
					strategy.Position("quantity").Offset(-1).EQ(0),
				), Action: strategy.ActionSpec{Type: strategy.ActionAdd, Side: side, AdditionIndex: 1, SizePct: 0.1}},
				{ID: "partial", When: strategy.And(
					strategy.Market("trade_count").EQ(2),
					strategy.Position("entry_count").Offset(-1).EQ(1),
				), Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: side, SizePct: 0.5}},
				{ID: "full", When: strategy.And(
					strategy.Market("trade_count").EQ(3),
					strategy.Position("abs_quantity").Offset(-1).EQ(20),
				), Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: side, SizePct: 1}},
				{ID: "reenter", When: strategy.And(
					strategy.Market("trade_count").EQ(4),
					strategy.Position("entry_count").Offset(-1).EQ(2),
				), Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: side, SizePct: 0.1}},
				{ID: "close_reentry", When: strategy.And(
					strategy.Market("trade_count").EQ(5),
					strategy.Position("quantity").Offset(-1).EQ(0),
					strategy.Position("abs_quantity").Offset(-2).EQ(10),
					strategy.Position("entry_count").EQ(1),
				), Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: side, SizePct: 1}},
			}
			bars := constantPriceBars(7, 100)
			for index := range bars {
				bars[index].TradeCount = int64(index)
			}
			plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
			if err != nil {
				t.Fatal(err)
			}
			run := func() Result {
				result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			result := run()
			if len(result.Fills) != 6 {
				t.Fatalf("fills = %+v; want six phases", result.Fills)
			}
			wantIDs := []string{"enter", "add", "partial", "full", "reenter", "close_reentry"}
			sign := 1.0
			if side == strategy.TradeSideShort {
				sign = -1
			}
			wantQty := []float64{0, 10, 20, 10, 0, 10, 0}
			for index, fill := range result.Fills {
				if fill.RuleID != wantIDs[index] || fill.TimeMS != int64(index+1)*900_000 ||
					result.Orders[index].SignalTimeMS != fill.TimeMS {
					t.Fatalf("phase %d has incorrect fill or signal: %+v", index, fill)
				}
			}
			for index, point := range result.EquityCurve {
				if point.PositionQty != sign*wantQty[index] {
					t.Fatalf("close %d quantity = %v, want %v", index, point.PositionQty, sign*wantQty[index])
				}
			}
			if !reflect.DeepEqual(result, run()) {
				t.Fatal("reusing a plan leaked state between runs")
			}
		})
	}
}

func TestRunHistoryMeasuresHistoricalCloseRatherThanCurrentOpen(t *testing.T) {
	def := baseDefinition()
	def.Execution.Fee.TakerRate = 0
	def.Rules = []strategy.RuleSpec{
		{ID: "enter", When: strategy.Market("trade_count").EQ(0),
			Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.1}},
		{ID: "exit", When: strategy.And(
			strategy.Market("trade_count").EQ(2),
			strategy.Position("unrealized_pnl_pct").Offset(-1).EQ(0.1),
			strategy.Position("avg_entry_price").Offset(-1).EQ(100),
			strategy.Position("actual_leverage").Offset(-1).EQ(1100.0/10100),
		), Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: 1}},
	}
	bars := constantPriceBars(4, 100)
	for index := range bars {
		bars[index].TradeCount = int64(index)
	}
	bars[1].High, bars[1].Close = 110, 110
	bars[2].Open, bars[2].High, bars[2].Low, bars[2].Close = 150, 151, 140, 145
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 2 || result.Fills[1].RuleID != "exit" || result.Fills[1].TimeMS != 3*900_000 {
		t.Fatalf("historical closing PnL/leverage gate failed: %+v", result.Fills)
	}
}

func TestRunHistoryRecordsIntrabarExitBeforeNextBarEvaluation(t *testing.T) {
	def := baseDefinition()
	def.Execution.Fee.TakerRate = 0
	def.ExitPolicies = []strategy.ExitPolicySpec{{
		ID: "stop", Side: strategy.TradeSideLong, Purpose: strategy.ExitPurposeStopLoss,
		Anchor:       strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
		Distance:     strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.05},
		TriggerPrice: strategy.ExitTriggerLow, Execution: strategy.ExitExecutionTriggerPrice, ClosePct: 1,
	}}
	def.Rules = []strategy.RuleSpec{
		{ID: "enter", When: strategy.Market("trade_count").EQ(0),
			Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.1}},
		{ID: "reenter", When: strategy.And(
			strategy.Market("trade_count").EQ(2),
			strategy.Position("quantity").Offset(-1).EQ(0),
		), Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.1}},
	}
	bars := constantPriceBars(4, 100)
	for index := range bars {
		bars[index].TradeCount = int64(index)
	}
	bars[1].Low = 90
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 3 || result.Fills[1].RuleID != "stop" || result.Fills[2].RuleID != "reenter" ||
		result.Fills[2].TimeMS != 3*900_000 || result.EquityCurve[1].PositionQty != 0 {
		t.Fatalf("snapshot did not include same-bar triggered exit: %+v", result.Fills)
	}
}
