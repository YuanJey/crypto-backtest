package engine

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestLedgerReplaysSpotAndBothPerpetualSides(t *testing.T) {
	for _, marketType := range []strategy.MarketType{strategy.MarketTypeSpot, strategy.MarketTypePerpetual} {
		for _, side := range []strategy.TradeSide{strategy.TradeSideLong, strategy.TradeSideShort} {
			if marketType == strategy.MarketTypeSpot && side == strategy.TradeSideShort {
				continue
			}
			t.Run(string(marketType)+"/"+string(side), func(t *testing.T) {
				def := baseDefinition()
				def.Account.MarketType = marketType
				def.Behavior.AllowPyramiding = true
				def.Rules = []strategy.RuleSpec{
					{ID: "entry", When: strategy.Bool(true), Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: side, SizePct: 0.4}},
				}
				plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
				if err != nil {
					t.Fatal(err)
				}
				report := DefaultReportSpec()
				report.Ledger = true
				result, err := Run(context.Background(), plan, RunRequest{Bars: constantPriceBars(4, 101.123), Report: report})
				if err != nil {
					t.Fatal(err)
				}
				// Include partial and final closes through the same production path.
				p := newPortfolio(marketType, def.Account.InitialCash)
				first := result.Ledger[0]
				applyMatchedFill(&p, matchedFill{quantity: first.Fill.Quantity, notional: first.Notional, fee: first.Fill.Fee}, first.Action, first.Fill.Price)
				config := plan.Snapshot().Defaults
				for i, pct := range []float64{0.25, 1} {
					result.Orders = append(result.Orders, Order{ID: "close-" + string(rune('a'+i)), Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: side, SizePct: pct}})
					if !executeOrder(&p, &result, len(result.Orders)-1, 107.456, int64(i+5)*900_000, plan.Snapshot().Logic, config, false) {
						t.Fatal("close failed")
					}
				}
				balance, err := ReplayLedger(config.Account, result.Ledger)
				if err != nil {
					t.Fatal(err)
				}
				if balance.Cash != p.cash || balance.PositionQty != 0 || balance.AveragePrice != 0 {
					t.Fatalf("bad replay: %+v", balance)
				}
				if len(result.Ledger) != len(result.Fills) {
					t.Fatal("not every fill has a ledger event")
				}
				// Spot conserves principal plus PNL minus all fees; perpetual
				// conserves initial cash plus realized PNL minus all fees.
				expected := def.Account.InitialCash
				for _, fill := range result.Fills {
					expected += fill.RealizedPNL - fill.Fee
				}
				if math.Abs(balance.Cash-expected) > 1e-8 {
					t.Fatalf("cash conservation failed: %.12f vs %.12f", balance.Cash, expected)
				}
				tampered := append([]LedgerEvent(nil), result.Ledger...)
				tampered[1].Cash++
				if _, err := ReplayLedger(config.Account, tampered); err == nil {
					t.Fatal("tampered balance accepted")
				}
				tampered = append([]LedgerEvent(nil), result.Ledger...)
				tampered[1].Fill.OrderID = tampered[0].Fill.OrderID
				if _, err := ReplayLedger(config.Account, tampered); err == nil {
					t.Fatal("duplicate fill accepted")
				}
			})
		}
	}
}

func TestRunConfigOverridesAreIsolatedAndEnforceRisk(t *testing.T) {
	def := actionSizingDefinition()
	def.Rules = []strategy.RuleSpec{sizingRule("entry", strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.2})}
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(plan.Snapshot())
	config := plan.Snapshot().Defaults
	config.Account.InitialCash = 20_000
	config.Risk.MaxSymbolEquityPct, config.Risk.MaxTotalExposurePct = 0.1, 0.2
	result, err := Run(context.Background(), plan, RunRequest{Bars: constantPriceBars(4, 100), Config: &config})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fills) != 1 || result.Fills[0].Quantity*result.Fills[0].Price != 2000 {
		t.Fatalf("explicit risk cap ignored: %+v", result.Fills)
	}
	after, _ := json.Marshal(plan.Snapshot())
	if string(before) != string(after) {
		t.Fatal("run config mutated plan defaults")
	}
	config.Account.InitialCash = -1
	if _, err := Run(context.Background(), plan, RunRequest{Bars: constantPriceBars(4, 100), Config: &config}); err == nil {
		t.Fatal("invalid experiment accepted")
	}
}
