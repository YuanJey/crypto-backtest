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

func actionSizingDefinition() strategy.Definition {
	def := baseDefinition()
	def.Account.ExchangeLeverage = 10
	def.Position = strategy.PositionSpec{PositionLeverage: 5}
	def.Execution.Fee.TakerRate = 0
	def.Execution.Slippage = strategy.SlippageSpec{Type: strategy.SlippageTypeNone}
	def.Risk.MaxDrawdownHaltPct = 0
	return def
}

func runActionSizing(t *testing.T, def strategy.Definition, bars []market.Bar) Result {
	t.Helper()
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func sizingRule(id string, action strategy.ActionSpec) strategy.RuleSpec {
	return strategy.RuleSpec{ID: id, When: strategy.Bool(true), Action: action}
}

func TestRunRuleLocalEntryAndSequentialAdditionSizes(t *testing.T) {
	for _, side := range []strategy.TradeSide{strategy.TradeSideLong, strategy.TradeSideShort} {
		t.Run(string(side), func(t *testing.T) {
			def := actionSizingDefinition()
			def.Rules = []strategy.RuleSpec{
				sizingRule("a_second", strategy.ActionSpec{Type: strategy.ActionAdd, Side: side, SizePct: 0.03, AdditionIndex: 2}),
				sizingRule("b_first", strategy.ActionSpec{Type: strategy.ActionAdd, Side: side, SizePct: 0.01, AdditionIndex: 1}),
				sizingRule("z_enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: side, SizePct: 0.02}),
			}
			result := runActionSizing(t, def, constantPriceBars(8, 100))
			if len(result.Fills) != 3 || len(result.Orders) != 3 {
				t.Fatalf("want one entry and exactly two additions: %+v", result.Orders)
			}
			for index, expected := range []float64{1000, 100, 300} {
				fill := result.Fills[index]
				if got := math.Abs(fill.Quantity) * fill.Price; math.Abs(got-expected) > 1e-9 {
					t.Fatalf("fill %d notional=%f, want %f", index, got, expected)
				}
				if fill.TimeMS != int64(index+1)*900_000 {
					t.Fatalf("fill %d must occur at the next bar open: %+v", index, fill)
				}
			}
		})
	}
}

func TestRunEntryEligibilityIsSeparateFromAdditionEligibility(t *testing.T) {
	cases := []struct {
		name  string
		rules []strategy.RuleSpec
	}{
		{"entry does not pyramid", []strategy.RuleSpec{
			sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.02}),
		}},
		{"addition cannot open a flat account", []strategy.RuleSpec{
			sizingRule("add", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.02, AdditionIndex: 1}),
		}},
		{"addition cannot reverse direction", []strategy.RuleSpec{
			sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.02}),
			sizingRule("add", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideShort, SizePct: 0.02, AdditionIndex: 1}),
		}},
		{"addition cannot skip the first step", []strategy.RuleSpec{
			sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.02}),
			sizingRule("add", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizePct: 0.02, AdditionIndex: 2}),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := actionSizingDefinition()
			// Legacy globals must not change new action eligibility or sizing.
			def.Behavior.AllowPyramiding = true
			def.Position.InitialSizePct = 0.8
			def.Position.MaxAdditions = 4
			def.Position.AddSizePct = 0.9
			def.Rules = tc.rules
			result := runActionSizing(t, def, constantPriceBars(8, 100))
			want := 1
			if tc.name == "addition cannot open a flat account" {
				want = 0
			}
			if len(result.Fills) != want || len(result.Orders) != want {
				t.Fatalf("fills=%d orders=%d, want %d", len(result.Fills), len(result.Orders), want)
			}
			if want == 1 && math.Abs(result.Fills[0].Quantity*result.Fills[0].Price-1000) > 1e-9 {
				t.Fatal("new entry incorrectly inherited global sizing")
			}
		})
	}
}

func TestRunAdditionMultiplierUsesActualFirstFillAndExchangeCap(t *testing.T) {
	def := actionSizingDefinition()
	def.Account.ExchangeLeverage = 5
	def.Rules = []strategy.RuleSpec{
		sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.2}),
		sizingRule("add_1", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizeMultiplier: 2, AdditionIndex: 1}),
		sizingRule("add_2", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizeMultiplier: 4, AdditionIndex: 2}),
	}
	result := runActionSizing(t, def, constantPriceBars(5, 100))
	if len(result.Fills) != 3 {
		t.Fatalf("want entry and two additions: %+v", result.Fills)
	}
	for index, want := range []float64{10000, 20000, 20000} {
		fill := result.Fills[index]
		if got := fill.Quantity * fill.Price; math.Abs(got-want) > 1e-9 {
			t.Fatalf("fill %d notional=%f, want %f", index, got, want)
		}
	}
}

func TestRunFullCloseResetsAdditionSequenceButPartialCloseDoesNot(t *testing.T) {
	for _, closePct := range []float64{1, 0.5} {
		t.Run(map[bool]string{true: "full", false: "partial"}[closePct == 1], func(t *testing.T) {
			def := actionSizingDefinition()
			closeRule := sizingRule("close", strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: closePct})
			closeRule.Priority = 100
			closeRule.When = strategy.Market("close").EQ(101)
			def.Rules = []strategy.RuleSpec{
				closeRule,
				sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.02}),
				sizingRule("add", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizeMultiplier: 2, AdditionIndex: 1}),
			}
			bars := constantPriceBars(7, 100)
			bars[2].Close = 101
			bars[2].High = 102
			result := runActionSizing(t, def, bars)
			want := 3
			if closePct == 1 {
				want = 5
			}
			if len(result.Fills) != want {
				t.Fatalf("fills=%d, want %d: %+v", len(result.Fills), want, result.Orders)
			}
			if closePct == 1 {
				for _, pair := range [][2]int{{0, 1}, {3, 4}} {
					first, add := result.Fills[pair[0]], result.Fills[pair[1]]
					if math.Abs(add.Quantity*add.Price-2*first.Quantity*first.Price) > 1e-9 {
						t.Fatal("addition did not use this round's first notional")
					}
				}
			}
		})
	}
}

func TestRunMigratedRuleProvenancePreservesLegacyTieOrdering(t *testing.T) {
	legacy := actionSizingDefinition()
	legacy.Position.InitialSizePct = 0.02
	legacy.Position.MaxAdditions = 1
	legacy.Position.AddSizeMultiplier = 2
	legacy.Behavior.AllowPyramiding = true
	legacy.Rules = []strategy.RuleSpec{
		sizingRule("a", strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong}),
		sizingRule("a0", strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: 1}),
	}
	expected := runActionSizing(t, legacy, constantPriceBars(7, 100))
	modern := actionSizingDefinition()
	modern.Rules = []strategy.RuleSpec{
		sizingRule("a", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.02}),
		sizingRule("a__add_1", strategy.ActionSpec{Type: strategy.ActionAdd, Side: strategy.TradeSideLong, SizeMultiplier: 2, AdditionIndex: 1}),
		legacy.Rules[1],
	}
	modern.Rules[0].SourceRuleID = "a"
	modern.Rules[1].SourceRuleID = "a"
	actual := runActionSizing(t, modern, constantPriceBars(7, 100))
	if len(actual.Fills) != len(expected.Fills) {
		t.Fatalf("migrated fill count=%d, legacy=%d", len(actual.Fills), len(expected.Fills))
	}
	for index, fill := range actual.Fills {
		before := expected.Fills[index]
		if fill.TimeMS != before.TimeMS || fill.Quantity != before.Quantity || fill.Fee != before.Fee {
			t.Fatalf("fill %d changed after rule split: %+v != %+v", index, fill, before)
		}
	}
}

func martingaleDefinition(mode strategy.AdditionSizingMode, side strategy.TradeSide) strategy.Definition {
	def := actionSizingDefinition()
	enabled := true
	def.Behavior.AllowAdditions = &enabled
	def.Rules = []strategy.RuleSpec{
		sizingRule("enter", strategy.ActionSpec{Type: strategy.ActionEnter, Side: side, SizePct: 0.002}),
	}
	for index := 1; index <= 3; index++ {
		def.Rules = append(def.Rules, sizingRule(
			[]string{"add_1", "add_2", "add_3"}[index-1],
			strategy.ActionSpec{Type: strategy.ActionAdd, Side: side, SizingMode: mode, SizeMultiplier: 2, AdditionIndex: index},
		))
	}
	return def
}

func TestRunMartingaleSizingModesAndMasterSwitch(t *testing.T) {
	for _, mode := range []strategy.AdditionSizingMode{strategy.AdditionSizeCurrentMultiple, strategy.AdditionSizeFirstGeometric} {
		for _, side := range []strategy.TradeSide{strategy.TradeSideLong, strategy.TradeSideShort} {
			for _, enabled := range []bool{false, true} {
				t.Run(string(mode)+"/"+string(side)+map[bool]string{true: "/on", false: "/off"}[enabled], func(t *testing.T) {
					def := martingaleDefinition(mode, side)
					def.Behavior.AllowAdditions = &enabled
					result := runActionSizing(t, def, constantPriceBars(7, 100))
					want := []float64{100}
					if enabled {
						if mode == strategy.AdditionSizeCurrentMultiple {
							want = []float64{100, 200, 600, 1800}
						} else {
							want = []float64{100, 200, 400, 800}
						}
					}
					if len(result.Fills) != len(want) || len(result.Orders) != len(want) {
						t.Fatalf("unexpected actions: %+v", result.Orders)
					}
					for index, notional := range want {
						fill := result.Fills[index]
						if math.Abs(math.Abs(fill.Quantity)*fill.Price-notional) > 1e-9 {
							t.Fatalf("fill %d=%+v, want notional %f", index, fill, notional)
						}
					}
				})
			}
		}
	}
}

func TestRunCurrentPositionMultipleUsesFillPriceAndQuantity(t *testing.T) {
	def := martingaleDefinition(strategy.AdditionSizeCurrentMultiple, strategy.TradeSideLong)
	def.Rules = def.Rules[:2]
	bars := constantPriceBars(4, 100)
	bars[2].Open, bars[2].Close, bars[2].High, bars[2].Low = 110, 110, 111, 109
	result := runActionSizing(t, def, bars)
	if len(result.Fills) != 2 {
		t.Fatalf("unexpected fills: %+v", result.Fills)
	}
	if result.Fills[1].Price != 110 || math.Abs(result.Fills[1].Quantity-2*result.Fills[0].Quantity) > 1e-9 {
		t.Fatalf("addition must double pre-fill position quantity using the fill price: %+v", result.Fills)
	}
}

func TestRunMartingaleAfterPartialCloseAndFullReset(t *testing.T) {
	for _, mode := range []strategy.AdditionSizingMode{strategy.AdditionSizeCurrentMultiple, strategy.AdditionSizeFirstGeometric} {
		for _, closePct := range []float64{0.5, 1} {
			t.Run(string(mode)+map[bool]string{true: "/full", false: "/partial"}[closePct == 1], func(t *testing.T) {
				def := martingaleDefinition(mode, strategy.TradeSideLong)
				def.Rules = def.Rules[:3] // Entry and two additions.
				closeRule := sizingRule("close", strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: closePct})
				closeRule.Priority = 100
				closeRule.When = strategy.Market("close").EQ(101)
				def.Rules = append(def.Rules, closeRule)
				bars := constantPriceBars(8, 100)
				bars[2].Close, bars[2].High = 101, 102
				result := runActionSizing(t, def, bars)
				want := []float64{100, 200, 150, 300}
				if mode == strategy.AdditionSizeFirstGeometric {
					want[3] = 400
				}
				if closePct == 1 {
					want = []float64{100, 200, 300, 100, 200, 600}
					if mode == strategy.AdditionSizeFirstGeometric {
						want[5] = 400
					}
				}
				if len(result.Fills) != len(want) {
					t.Fatalf("unexpected fills: %+v", result.Fills)
				}
				for index, notional := range want {
					fill := result.Fills[index]
					if math.Abs(math.Abs(fill.Quantity)*fill.Price-notional) > 1e-9 {
						t.Fatalf("fill %d=%+v, want %f", index, fill, notional)
					}
				}
			})
		}
	}
}

func TestRunMartingaleModesHonorExchangeCap(t *testing.T) {
	for _, mode := range []strategy.AdditionSizingMode{strategy.AdditionSizeCurrentMultiple, strategy.AdditionSizeFirstGeometric} {
		t.Run(string(mode), func(t *testing.T) {
			def := martingaleDefinition(mode, strategy.TradeSideLong)
			def.Account.ExchangeLeverage = 1
			def.Position.PositionLeverage = 1
			def.Rules[0].Action.SizePct = 0.2
			def.Rules = def.Rules[:3]
			result := runActionSizing(t, def, constantPriceBars(5, 100))
			if len(result.Fills) != 3 {
				t.Fatalf("unexpected fills: %+v", result.Fills)
			}
			last := result.Fills[2]
			if math.Abs(last.Quantity*last.Price-4000) > 1e-9 {
				t.Fatalf("final addition must be clipped to available exposure: %+v", last)
			}
		})
	}
}

func TestRunMasterSwitchCanDisableLegacyPyramiding(t *testing.T) {
	def := actionSizingDefinition()
	def.Behavior.AllowPyramiding = true
	disabled := false
	def.Behavior.AllowAdditions = &disabled
	def.Position.InitialSizePct = 0.01
	def.Position.MaxAdditions = 2
	def.Rules = []strategy.RuleSpec{sizingRule("old", strategy.ActionSpec{Type: strategy.ActionOpen, Side: strategy.TradeSideLong})}
	result := runActionSizing(t, def, constantPriceBars(6, 100))
	if len(result.Fills) != 1 || len(result.Orders) != 1 {
		t.Fatalf("master switch must disable legacy additions without blocking entry: %+v", result.Orders)
	}
}
