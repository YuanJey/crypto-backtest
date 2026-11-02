package strategy

import (
	"encoding/json"
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

	// 校验应由 Normalize 自动补齐 high/low/close 与 close，校验必须通过
	if err := Validate(definition, indicator.MustBuiltinRegistry()); err != nil {
		t.Fatalf("expected auto inferred inputs to pass validate, got: %v", err)
	}
}

