package engine

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func marginDefinition(side strategy.TradeSide) strategy.Definition {
	def := actionSizingDefinition()
	def.Position.PositionLeverage = 1
	def.Account.MaintenanceMarginRate = 0.005
	def.Rules = []strategy.RuleSpec{
		sizingRule("entry", strategy.ActionSpec{Type: strategy.ActionEnter, Side: side, SizePct: 1}),
		sizingRule("addition", strategy.ActionSpec{Type: strategy.ActionAdd, Side: side, AdditionIndex: 1,
			SizingMode: strategy.AdditionSizeCurrentMultiple, SizeMultiplier: 4}),
	}
	return def
}

func runMargin(t *testing.T, def strategy.Definition, bars []market.Bar) Result {
	t.Helper()
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	report := DefaultReportSpec()
	report.Ledger = true
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars, Report: report})
	if err != nil {
		t.Fatal(err)
	}
	balance, err := ReplayLedger(plan.Snapshot().Defaults.Account, result.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	last := result.EquityCurve[len(result.EquityCurve)-1]
	if balance.Cash != last.Cash || balance.PositionQty != last.PositionQty {
		t.Fatal("margin execution failed ledger replay")
	}
	return result
}

func TestMarginLiquidatesIntrabarBothSidesAndHaltsReentry(t *testing.T) {
	for _, side := range []strategy.TradeSide{strategy.TradeSideLong, strategy.TradeSideShort} {
		t.Run(string(side), func(t *testing.T) {
			def := marginDefinition(side)
			bars := constantPriceBars(8, 100)
			// No position exists on bar 0, even though its extremes are huge.
			bars[0].Low, bars[0].High = 1, 1000
			bars[2].Low, bars[2].High = 70, 130
			result := runMargin(t, def, bars)
			if result.Margin == nil || !result.Margin.Liquidated || result.Margin.LiquidationCount != 1 {
				t.Fatalf("intrabar breach missed: %+v", result.Margin)
			}
			if len(result.Fills) != 3 || len(result.Orders) != 3 {
				t.Fatalf("unexpected fills/reentry: %+v", result.Fills)
			}
			event := result.Margin.Liquidations[0]
			want := 80 / 0.995
			if side == strategy.TradeSideShort {
				want = 120 / 1.005
			}
			if !near(event.TriggerPrice, want) || !near(event.EquityBefore, event.MaintenanceMargin) ||
				event.Phase != "intrabar" || event.BarOpenTimeMS != 1800000 || event.TimeMS != 2700000 {
				t.Fatalf("wrong threshold or visibility: %+v", event)
			}
			if result.Fills[2].Reason != "liquidation" || result.Orders[2].Reason != "liquidation" {
				t.Fatal("liquidation reason missing from execution events")
			}
			if !near(*result.Margin.MinimumRatio, 0.005) {
				t.Fatalf("minimum ratio included unexecuted prices: %f", *result.Margin.MinimumRatio)
			}
			if result.Margin.MinimumMMR == nil || !near(*result.Margin.MinimumMMR, 1) ||
				event.MMR == nil || !near(*event.MMR, event.EquityBefore/event.MaintenanceMargin) {
				t.Fatal("MMR must reach 100% at the maintenance boundary")
			}
			for _, point := range result.EquityCurve[2:] {
				if point.PositionQty != 0 || point.Equity != event.CashAfter {
					t.Fatal("liquidated run resumed trading")
				}
			}
		})
	}
}

func TestMarginEqualityTriggersButFavorableExtremeDoesNot(t *testing.T) {
	for _, side := range []strategy.TradeSide{strategy.TradeSideLong, strategy.TradeSideShort} {
		def := marginDefinition(side)
		bars := constantPriceBars(5, 100)
		bars[2].High = 130
		if side == strategy.TradeSideShort {
			bars[2].High, bars[2].Low = 100, 70
		}
		result := runMargin(t, def, bars)
		if result.Margin.Liquidated {
			t.Fatal("favorable extreme caused liquidation")
		}
		if side == strategy.TradeSideLong {
			bars[2].Low = 80 / 0.995
		} else {
			bars[2].High = 120 / 1.005
		}
		result = runMargin(t, def, bars)
		if !result.Margin.Liquidated {
			t.Fatal("equality breach was not liquidated")
		}
	}
}

func TestMarginGapLiquidatesBeforePendingCloseAndRecordsDeficit(t *testing.T) {
	def := marginDefinition(strategy.TradeSideLong)
	def.Rules = append(def.Rules, strategy.RuleSpec{ID: "close", Priority: 10,
		When:   strategy.Position("entry_count").EQ(2),
		Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: strategy.TradeSideLong, SizePct: 1}})
	bars := constantPriceBars(6, 100)
	bars[3].Open, bars[3].Low, bars[3].High = 50, 45, 100
	result := runMargin(t, def, bars)
	event := result.Margin.Liquidations[0]
	if event.Phase != "open" || event.FillPrice != 50 || event.TimeMS != bars[3].OpenTimeMS || event.CashAfter != -15000 || event.Deficit != 15000 {
		t.Fatalf("gap filled at unavailable threshold or lost deficit: %+v", event)
	}
	if result.Margin.MinimumMMR == nil || *result.Margin.MinimumMMR != -120 {
		t.Fatal("gap MMR was clamped or used notional instead of maintenance margin")
	}
	found := false
	for _, order := range result.Orders {
		if order.RuleID == "close" {
			found = order.Status == OrderStatusSkipped && order.Reason == "account_liquidated"
		}
	}
	if !found {
		t.Fatal("pending close rescued a previously liquidated gap position")
	}
}

func TestMarginChecksFeesImmediatelyAfterOpeningFill(t *testing.T) {
	def := marginDefinition(strategy.TradeSideLong)
	def.Account.MaintenanceMarginRate = 0.9
	def.Execution.Fee.TakerRate = 0.2
	result := runMargin(t, def, constantPriceBars(8, 100))
	event := result.Margin.Liquidations[0]
	if event.Phase != "post_fill" || event.TimeMS != 900000 || event.EquityBefore != 8000 || event.CashAfter != 6000 || len(result.Fills) != 2 {
		t.Fatalf("post-fee breach missed or trading resumed: %+v", event)
	}
}

func TestMarginStopsCanPreventOrReduceLiquidation(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		closePct, low float64
		execution     strategy.ExitExecution
		liquidated    bool
	}{
		{"full protective stop", 1, 70, strategy.ExitExecutionTriggerPrice, false},
		{"partial stop prevents breach", 0.5, 70, strategy.ExitExecutionTriggerPrice, false},
		{"partial stop then liquidation", 0.5, 60, strategy.ExitExecutionTriggerPrice, true},
		{"next open stop cannot rescue intrabar", 1, 70, strategy.ExitExecutionNextOpen, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			def := marginDefinition(strategy.TradeSideLong)
			def.ExitPolicies = []strategy.ExitPolicySpec{{ID: "stop", Side: strategy.TradeSideLong,
				Purpose: strategy.ExitPurposeStopLoss, Anchor: strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
				Distance:     strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.05},
				TriggerPrice: strategy.ExitTriggerLow, Execution: scenario.execution, ClosePct: scenario.closePct}}
			bars := constantPriceBars(4, 100)
			bars[2].Low = scenario.low
			result := runMargin(t, def, bars)
			if result.Margin.Liquidated != scenario.liquidated {
				t.Fatalf("wrong stop/liq ordering: %+v", result.Margin)
			}
			if scenario.execution == strategy.ExitExecutionTriggerPrice {
				if result.Fills[2].RuleID != "stop" || result.Fills[2].Price != 95 {
					t.Fatal("closer protective stop skipped")
				}
				if scenario.liquidated && result.Margin.Liquidations[0].Quantity != 250 {
					t.Fatal("liquidation ignored partial close")
				}
			}
		})
	}
}

func TestMarginTakeProfitCannotOptimisticallyRescueAmbiguousBar(t *testing.T) {
	def := marginDefinition(strategy.TradeSideLong)
	def.ExitPolicies = []strategy.ExitPolicySpec{{ID: "tp", Side: strategy.TradeSideLong,
		Purpose: strategy.ExitPurposeTakeProfit, Anchor: strategy.ExitAnchorSpec{Type: strategy.ExitAnchorAverageEntry},
		Distance:     strategy.ExitDistanceSpec{Type: strategy.ExitDistancePercent, Value: 0.1},
		TriggerPrice: strategy.ExitTriggerHigh, Execution: strategy.ExitExecutionTriggerPrice}}
	bars := constantPriceBars(5, 100)
	bars[2].High, bars[2].Low = 120, 70
	result := runMargin(t, def, bars)
	if !result.Margin.Liquidated || result.Fills[2].Reason != "liquidation" {
		t.Fatal("optimistic intrabar take profit ordering")
	}
}

func TestMarginSummarySurvivesOmittedDetailsAndDefaultsRemainUnchanged(t *testing.T) {
	def := marginDefinition(strategy.TradeSideLong)
	def.Account.MaintenanceMarginRate = 0
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(plan.Snapshot())
	bars := constantPriceBars(5, 100)
	bars[2].Low = 70
	legacy, err := Run(context.Background(), plan, RunRequest{Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Margin != nil || legacy.EquityCurve[2].MarginRatio != nil {
		t.Fatal("legacy run silently enabled margin")
	}
	config := plan.Snapshot().Defaults
	config.Account.MaintenanceMarginRate = 0.005
	result, err := Run(context.Background(), plan, RunRequest{Bars: bars, Config: &config,
		Report: ReportSpec{Metrics: []MetricType{MetricFinalEquity, MetricMinimumMMR}}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(plan.Snapshot())
	if string(before) != string(after) || result.Margin == nil || !result.Margin.Liquidated ||
		result.EquityCurve != nil || result.Fills != nil || result.Ledger != nil || result.TotalFills != 3 {
		t.Fatal("margin summary or run configuration isolation lost")
	}
	if !near(result.Metrics[MetricMinimumMMR], 1) {
		t.Fatal("minimum MMR metric missing from summary report")
	}
}

func TestMinimumMMRUsesMaintenanceMarginAndClosingPointsAgree(t *testing.T) {
	for _, rate := range []float64{0.005, 0.01} {
		def := marginDefinition(strategy.TradeSideLong)
		def.Account.MaintenanceMarginRate = rate
		result := runMargin(t, def, constantPriceBars(5, 100))
		if result.Margin.MMRDefinition != "equity_over_maintenance_margin" ||
			result.Margin.MinimumMMR == nil || !near(*result.Margin.MinimumMMR, 0.2/rate) {
			t.Fatalf("wrong minimum MMR at rate %v: %+v", rate, result.Margin)
		}
		for _, point := range result.EquityCurve {
			if point.PositionQty == 0 {
				if point.MMR != nil {
					t.Fatal("flat position has an MMR")
				}
				continue
			}
			if point.MMR == nil || point.MaintenanceMargin == nil ||
				*point.MMR != point.Equity / *point.MaintenanceMargin {
				t.Fatal("closing MMR is not equity / maintenance")
			}
		}
	}
}

func TestMinimumMMRIsUndefinedForDisabledOrNeverOpenedRuns(t *testing.T) {
	for _, rate := range []float64{0, 0.005} {
		def := marginDefinition(strategy.TradeSideLong)
		def.Account.MaintenanceMarginRate = rate
		def.Rules = nil
		plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
		if err != nil {
			t.Fatal(err)
		}
		result, err := Run(context.Background(), plan, RunRequest{Bars: constantPriceBars(3, 100),
			Report: ReportSpec{Metrics: []MetricType{MetricMinimumMMR}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, present := result.Metrics[MetricMinimumMMR]; present {
			t.Fatal("undefined MMR was reported as zero")
		}
		if result.Margin != nil && result.Margin.MinimumMMR != nil {
			t.Fatal("never-opened run has a minimum MMR")
		}
	}
}

func TestMarginConfigRejectsInvalidRatesAndSpot(t *testing.T) {
	for _, rate := range []float64{-0.001, 1, math.Inf(1), math.NaN()} {
		def := marginDefinition(strategy.TradeSideLong)
		def.Account.MaintenanceMarginRate = rate
		if _, err := compiler.Compile(def, indicator.MustBuiltinRegistry()); err == nil {
			t.Fatalf("invalid rate accepted: %v", rate)
		}
	}
	def := marginDefinition(strategy.TradeSideLong)
	def.Account.MarketType = strategy.MarketTypeSpot
	def.Account.ExchangeLeverage = 1
	if _, err := compiler.Compile(def, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("spot liquidation model accepted")
	}
}
