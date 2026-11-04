package strategy

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
)

func TestValidateStrategyWithIndicatorParams(t *testing.T) {
	definition := validDefinition()
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsBadIndicatorParams(t *testing.T) {
	definition := validDefinition()
	definition.Indicators[0].Params = json.RawMessage(`{"period":0}`)
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected invalid indicator params to fail")
	}
}

func TestValidateRejectsDuplicateIndicatorID(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, definition.Indicators[0])
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected duplicate indicator id to fail")
	}
}

func TestValidateRejectsIndicatorDependencyCycle(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = []IndicatorSpec{
		{
			ID:     "first",
			Type:   "ema",
			Inputs: []ValueRef{{Scope: ValueScopeIndicator, Name: "second", Field: "value"}},
			Params: json.RawMessage(`{"period":1}`),
		},
		{
			ID:     "second",
			Type:   "sma",
			Inputs: []ValueRef{{Scope: ValueScopeIndicator, Name: "first", Field: "value"}},
			Params: json.RawMessage(`{"period":1}`),
		},
	}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected indicator dependency cycle to fail")
	}
}

func TestValidateRejectsPositionInputForIndicator(t *testing.T) {
	definition := validDefinition()
	definition.Indicators[0].Inputs = []ValueRef{{
		Scope: ValueScopePosition,
		Name:  "current",
		Field: "quantity",
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected position-dependent indicator to fail")
	}
}

func TestValidateRejectsExpressionTypeMismatch(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID: "invalid",
		When: Expression{
			Kind: ExprOperator,
			Op:   OperatorGT,
			Args: []Expression{Bool(true), Number(0)},
		},
		Action: ActionSpec{Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected boolean operand in numeric comparison to fail")
	}
}

func TestValidateAcceptsMarketPriceComparedWithAverageEntryPrice(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID:     "close_above_average_entry",
		When:   Market("close").GTE(Position("avg_entry_price")),
		Action: ActionSpec{Type: ActionClose, Side: TradeSideLong, SizePct: 1},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected market price and average entry price comparison to pass: %v", err)
	}
}

func TestValidateRejectsMarketPriceComparedWithPositionRatio(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID:     "invalid_price_ratio_comparison",
		When:   Market("close").GTE(Position("unrealized_pnl_pct")),
		Action: ActionSpec{Type: ActionClose, Side: TradeSideLong, SizePct: 1},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected price and position ratio comparison to fail")
	}
}

func TestValidateAcceptsATRExitPolicy(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "atr_15m",
		Type:   "atr",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "high"}, {Scope: ValueScopeMarket, Name: "bar", Field: "low"}, {Scope: ValueScopeMarket, Name: "bar", Field: "close"}},
		Params: json.RawMessage(`{"period":14}`),
	})
	definition.ExitPolicies = []ExitPolicySpec{{
		ID:      "long_tp_atr",
		Side:    TradeSideLong,
		Purpose: ExitPurposeTakeProfit,
		Anchor:  ExitAnchorSpec{Type: ExitAnchorAverageEntry},
		Distance: ExitDistanceSpec{
			Type:      ExitDistanceATRMultiple,
			Value:     2,
			Indicator: "atr_15m",
			Field:     "value",
			Sampling:  ExitSamplingEntry,
		},
		TriggerPrice: ExitTriggerHigh,
		ClosePct:     1,
		Execution:    ExitExecutionTriggerPrice,
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected ATR exit policy to pass: %v", err)
	}
}

func TestValidateRejectsATRExitPolicyWithNonVolatilityIndicator(t *testing.T) {
	definition := validDefinition()
	definition.ExitPolicies = []ExitPolicySpec{{
		ID:      "invalid_atr_source",
		Side:    TradeSideLong,
		Purpose: ExitPurposeTakeProfit,
		Anchor:  ExitAnchorSpec{Type: ExitAnchorAverageEntry},
		Distance: ExitDistanceSpec{
			Type:      ExitDistanceATRMultiple,
			Value:     2,
			Indicator: "ema_fast",
			Field:     "value",
			Sampling:  ExitSamplingLive,
		},
		TriggerPrice: ExitTriggerHigh,
		ClosePct:     1,
		Execution:    ExitExecutionNextOpen,
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected non-volatility ATR source to fail")
	}
}

func TestValidateAcceptsDirectIndicatorExitAnchor(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "extreme_5d",
		Type:   "extreme",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "high"}, {Scope: ValueScopeMarket, Name: "bar", Field: "low"}},
		Params: json.RawMessage(`{"period":5}`),
	})
	definition.ExitPolicies = []ExitPolicySpec{{
		ID:           "long_tp_5d_high",
		Side:         TradeSideLong,
		Purpose:      ExitPurposeTakeProfit,
		Anchor:       ExitAnchorSpec{Type: ExitAnchorIndicator, Indicator: "extreme_5d", Field: "highest"},
		Distance:     ExitDistanceSpec{Type: ExitDistanceDirect},
		TriggerPrice: ExitTriggerHigh,
		ClosePct:     1,
		Execution:    ExitExecutionTriggerPrice,
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected direct indicator exit anchor to pass: %v", err)
	}
}

func TestValidateRejectsNonPriceIndicatorExitAnchor(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "rsi_filter",
		Type:   "rsi",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "close"}},
		Params: json.RawMessage(`{"period":14}`),
	})
	definition.ExitPolicies = []ExitPolicySpec{{
		ID:           "invalid_rsi_anchor",
		Side:         TradeSideLong,
		Purpose:      ExitPurposeTakeProfit,
		Anchor:       ExitAnchorSpec{Type: ExitAnchorIndicator, Indicator: "rsi_filter", Field: "value"},
		Distance:     ExitDistanceSpec{Type: ExitDistanceDirect},
		TriggerPrice: ExitTriggerHigh,
		ClosePct:     1,
		Execution:    ExitExecutionTriggerPrice,
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected oscillator indicator anchor to fail")
	}
}

func TestValidateRejectsCrossWithNonIndicatorOperand(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "ema_slow",
		Type:   "ema",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "close"}},
		Params: json.RawMessage(`{"period":50}`),
	})
	definition.Rules = []RuleSpec{{
		ID: "invalid_cross",
		When: Market("close").CrossUp(
			Indicator("ema_slow"),
		),
		Action: ActionSpec{Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected cross with market operand to fail")
	}
}

func TestValidateAcceptsCrossBetweenDifferentIndicators(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "ema_slow",
		Type:   "ema",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "close"}},
		Params: json.RawMessage(`{"period":50}`),
	})
	definition.Rules = []RuleSpec{{
		ID:   "golden_cross",
		When: Indicator("ema_fast").CrossUp(Indicator("ema_slow")),
		Action: ActionSpec{
			Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected indicator cross to pass: %v", err)
	}
}

func TestValidateRejectsCrossAgainstSameIndicator(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID:   "same_cross",
		When: Indicator("ema_fast").CrossDown(Indicator("ema_fast")),
		Action: ActionSpec{
			Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected crossing an indicator with itself to fail")
	}
}

func TestValidateRejectsCrossBetweenIncompatibleIndicatorGroups(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "rsi_slow",
		Type:   "rsi",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "close"}},
		Params: json.RawMessage(`{"period":28}`),
	})
	definition.Rules = []RuleSpec{{
		ID:   "invalid_cross",
		When: Indicator("ema_fast").CrossUp(Indicator("rsi_slow")),
		Action: ActionSpec{
			Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected incompatible indicator cross groups to fail")
	}
}

func TestValidateSupertrendSemanticConditions(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "supertrend",
		Type:   "supertrend",
		Inputs: nil,
		Params: json.RawMessage(`{"period":10,"multiplier":3}`),
	})
	definition.Rules = []RuleSpec{
		{
			ID:   "bullish",
			When: IndicatorField("supertrend", "direction").EQ(1),
			Action: ActionSpec{
				Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
			},
		},
		{
			ID: "support",
			When: Market("close").GTE(
				IndicatorField("supertrend", "value"),
			),
			Action: ActionSpec{
				Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
			},
		},
		{
			ID:   "turn_bullish",
			When: IndicatorField("supertrend", "direction").TurnsBullish(),
			Action: ActionSpec{
				Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
			},
		},
		{
			ID:   "turn_bearish",
			When: IndicatorField("supertrend", "direction").TurnsBearish(),
			Action: ActionSpec{
				Type: ActionClose, Side: TradeSideLong, SizePct: 1,
			},
		},
	}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected Supertrend semantic conditions to pass: %v", err)
	}
}

func TestValidateRejectsTrendTransitionOnNonDirectionOutput(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID:   "invalid_transition",
		When: Indicator("ema_fast").TurnsBullish(),
		Action: ActionSpec{
			Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected trend transition on EMA to fail")
	}
}

func TestValidateRejectsInvalidSupertrendDirectionComparison(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "supertrend",
		Type:   "supertrend",
		Inputs: nil,
		Params: json.RawMessage(`{"period":10,"multiplier":3}`),
	})
	definition.Rules = []RuleSpec{{
		ID:   "invalid_direction",
		When: IndicatorField("supertrend", "direction").GT(0),
		Action: ActionSpec{
			Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected non-equality direction comparison to fail")
	}
}

func TestValidateRejectsComparisonAcrossValueDomains(t *testing.T) {
	definition := validDefinition()
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:     "rsi",
		Type:   "rsi",
		Inputs: []ValueRef{{Scope: ValueScopeMarket, Name: "bar", Field: "close"}},
		Params: json.RawMessage(`{"period":14}`),
	})
	definition.Rules = []RuleSpec{{
		ID:   "invalid_domain",
		When: Indicator("ema_fast").GT(Indicator("rsi")),
		Action: ActionSpec{
			Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected price and oscillator comparison to fail")
	}
}

func TestValidateRejectsNumericRuleRoot(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID:     "invalid",
		When:   Number(1),
		Action: ActionSpec{Type: ActionOpen, Side: TradeSideLong, SizePct: 0.2},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected numeric rule root to fail")
	}
}

func TestValidateRejectsUnsupportedSameBarReversal(t *testing.T) {
	definition := validDefinition()
	definition.Behavior.AllowSameBarReversal = true
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected unsupported same-bar reversal to fail")
	}
}

func TestValidateRejectsInactivePyramidingConfiguration(t *testing.T) {
	definition := validDefinition()
	definition.Position.MaxAdditions = 2
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected additions without pyramiding behavior to fail")
	}
}

func TestNormalizeMigratesLegacyPyramidingFlag(t *testing.T) {
	definition := validDefinition()
	definition.Position.AllowPyramiding = true
	normalized, err := Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.Behavior.AllowPyramiding {
		t.Fatal("expected legacy position flag to migrate to behavior")
	}
	if normalized.Position.AllowPyramiding {
		t.Fatal("expected normalized position flag to be cleared")
	}
}

func TestNormalizeDefaultsLeverageToOne(t *testing.T) {
	definition := validDefinition()
	normalized, err := Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Account.ExchangeLeverage != 1 ||
		normalized.Position.PositionLeverage != 1 {
		t.Fatalf("unexpected leverage defaults: account=%f position=%f",
			normalized.Account.ExchangeLeverage,
			normalized.Position.PositionLeverage,
		)
	}
}

func TestNormalizeDefaultsCapitalDecayPower(t *testing.T) {
	definition := validDefinition()
	normalized, err := Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Position.LongCapitalDecayPower != DefaultCapitalDecayPower ||
		normalized.Position.ShortCapitalDecayPower != DefaultCapitalDecayPower {
		t.Fatalf("unexpected capital decay defaults: %+v", normalized.Position)
	}
}

func TestNormalizeDefaultsExitPolicy(t *testing.T) {
	definition := validDefinition()
	definition.ExitPolicies = []ExitPolicySpec{{
		ID:       "long_tp",
		Side:     TradeSideLong,
		Purpose:  ExitPurposeTakeProfit,
		Distance: ExitDistanceSpec{Type: ExitDistancePercent, Value: 0.03},
	}}
	normalized, err := Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	policy := normalized.ExitPolicies[0]
	if policy.Anchor.Type != ExitAnchorAverageEntry ||
		policy.TriggerPrice != ExitTriggerHigh ||
		policy.ClosePct != 1 ||
		policy.Execution != ExitExecutionNextOpen {
		t.Fatalf("unexpected exit policy defaults: %+v", policy)
	}
}

func TestValidateRejectsInvalidCapitalDecayPower(t *testing.T) {
	definition := validDefinition()
	definition.Position.LongCapitalDecayPower = -0.1
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected negative long capital decay power to fail")
	}

	definition = validDefinition()
	definition.Position.ShortCapitalDecayPower = math.Inf(1)
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected infinite short capital decay power to fail")
	}
}

func TestNormalizeMigratesLegacyLeverageFields(t *testing.T) {
	definition := validDefinition()
	definition.Account.MarketType = MarketTypePerpetual
	definition.Account.ExchangeLeverage = 10
	definition.Position.TargetLeverage = 5
	definition.Position.MaxPositionPct = 4
	definition.Risk.MaxSymbolEquityPct = 0.25
	definition.Risk.MaxTotalExposurePct = 0.5

	normalized, err := Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Position.PositionLeverage != 5 {
		t.Fatalf("position leverage=%f, want migrated 5", normalized.Position.PositionLeverage)
	}
	if normalized.Position.TargetLeverage != 0 || normalized.Position.MaxPositionPct != 0 {
		t.Fatalf("legacy position fields were not cleared: %+v", normalized.Position)
	}
	if normalized.Risk.MaxSymbolEquityPct != 10 ||
		normalized.Risk.MaxTotalExposurePct != 10 {
		t.Fatalf("legacy exposure caps were not aligned to exchange leverage: %+v", normalized.Risk)
	}
}

func TestValidatePerpetualLeverageConfiguration(t *testing.T) {
	definition := validDefinition()
	definition.Account.MarketType = MarketTypePerpetual
	definition.Account.ExchangeLeverage = 10
	definition.Position.PositionLeverage = 5
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected perpetual leverage configuration to pass: %v", err)
	}

	definition.Position.PositionLeverage = 11
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected position leverage above exchange leverage to fail")
	}
}

func TestValidateRejectsSpotLeverage(t *testing.T) {
	definition := validDefinition()
	definition.Position.PositionLeverage = 2
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected spot leverage above 1 to fail")
	}
}

func TestValidateMultiTimeframeLongShortRules(t *testing.T) {
	definition := validDefinition()
	definition.Account.MarketType = MarketTypePerpetual
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:        "ema_slow_1h",
		Type:      "ema",
		Timeframe: "1h",
		Inputs: []ValueRef{{
			Scope: ValueScopeMarket,
			Name:  "bar",
			Field: "close",
		}},
		Params: json.RawMessage(`{"period":60}`),
	})
	number := 0.0
	definition.Rules = []RuleSpec{{
		ID: "open_short",
		When: Expression{
			Kind: ExprOperator,
			Op:   OperatorLT,
			Args: []Expression{
				{
					Kind: ExprReference,
					Ref: &ValueRef{
						Scope: ValueScopeIndicator,
						Name:  "ema_fast",
						Field: "value",
					},
				},
				{
					Kind:   ExprNumber,
					Number: &number,
				},
			},
		},
		Action: ActionSpec{
			Type:    ActionOpen,
			Side:    TradeSideShort,
			SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsShortRulesOnSpot(t *testing.T) {
	definition := validDefinition()
	value := true
	definition.Rules = []RuleSpec{{
		ID: "open_short",
		When: Expression{
			Kind: ExprBoolean,
			Bool: &value,
		},
		Action: ActionSpec{
			Type:    ActionOpen,
			Side:    TradeSideShort,
			SizePct: 0.2,
		},
	}}
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err == nil {
		t.Fatal("expected short rule on spot account to fail")
	}
}

func validDefinition() Definition {
	raw := []byte(`{
  "schema_version": 1,
  "id": "ema-boll-rsi",
  "name": "EMA BOLL RSI",
  "data": {
    "dataset_id": "binance-spot-15m-202501",
    "symbols": ["BTC-USDT"],
    "timeframe": "15m",
    "start": "2025-01-01T00:00:00Z",
    "end": "2025-02-01T00:00:00Z"
  },
  "account": {
    "market_type": "spot",
    "initial_cash": 10000,
    "quote_currency": "USDT"
  },
  "behavior": {
    "allow_pyramiding": false,
    "allow_same_bar_reversal": false,
    "cooldown_bars": 0
  },
  "execution": {
    "signal_time": "bar_close",
    "market_fill_time": "next_bar_open",
    "intrabar_conflict": "conservative",
    "fee": {"maker_rate": 0.001, "taker_rate": 0.001},
    "slippage": {"type": "bps", "value": 1}
  },
  "risk": {
    "max_symbol_equity_pct": 1,
    "max_total_exposure_pct": 1,
    "max_open_positions": 1,
    "max_drawdown_halt_pct": 0.25
  },
  "indicators": [
    {
      "id": "ema_fast",
      "type": "ema",
      "inputs": [{"scope": "market", "name": "bar", "field": "close"}],
      "params": {"period": 20}
    }
  ]
}`)
	definition, err := Decode(raw)
	if err != nil {
		panic(err)
	}
	return definition
}

func TestValidateStrategyWithEmptyInputsAutoInferred(t *testing.T) {
	definition := validDefinition()
	// 清空 inputs，模拟极简定义
	definition.Indicators[0].Inputs = nil

	// 新增一个 ATR 指标，同样不传 inputs
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:        "atr_15m",
		Type:      "atr",
		Timeframe: "15m",
		Inputs:    nil,
		Params:    json.RawMessage(`{"period":14}`),
	})
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:        "supertrend_15m",
		Type:      "supertrend",
		Timeframe: "15m",
		Inputs:    nil,
		Params:    json.RawMessage(`{"period":10,"multiplier":3}`),
	})
	definition.Indicators = append(definition.Indicators, IndicatorSpec{
		ID:        "extreme_5d",
		Type:      "extreme",
		Timeframe: "1d",
		Inputs:    nil,
		Params:    json.RawMessage(`{"period":5}`),
	})

	// 校验应由 Normalize 自动补齐指标所需行情字段，校验必须通过
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected auto inferred inputs to pass validate, got: %v", err)
	}
}
