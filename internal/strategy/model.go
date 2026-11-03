package strategy

import (
	"encoding/json"
	"time"
)

const (
	SchemaVersion            = 1
	DefaultCapitalDecayPower = 0.5
)

type Definition struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Data          DataSelection   `json:"data"`
	Account       AccountSpec     `json:"account"`
	Behavior      BehaviorSpec    `json:"behavior"`
	Position      PositionSpec    `json:"position,omitempty"`
	Execution     ExecutionSpec   `json:"execution"`
	Risk          RiskSpec        `json:"risk"`
	Indicators    []IndicatorSpec `json:"indicators"`
	Rules         []RuleSpec      `json:"rules,omitempty"`
}

type DataSelection struct {
	DatasetID string    `json:"dataset_id"`
	Symbols   []string  `json:"symbols"`
	Timeframe string    `json:"timeframe"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
}

type AccountSpec struct {
	MarketType       MarketType `json:"market_type"`
	InitialCash      float64    `json:"initial_cash"`
	QuoteCurrency    string     `json:"quote_currency"`
	ExchangeLeverage float64    `json:"exchange_leverage,omitempty"`
}

type MarketType string

const (
	MarketTypeSpot      MarketType = "spot"
	MarketTypePerpetual MarketType = "perpetual"
)

type BehaviorSpec struct {
	AllowPyramiding      bool `json:"allow_pyramiding"`
	AllowSameBarReversal bool `json:"allow_same_bar_reversal"`
	CooldownBars         int  `json:"cooldown_bars"`
}

type PositionSpec struct {
	InitialSizePct float64 `json:"initial_size_pct,omitempty"`
	// AllowPyramiding is a legacy alias. Normalize migrates it to
	// Behavior.AllowPyramiding so runtime policy has one source of truth.
	AllowPyramiding   bool    `json:"allow_pyramiding,omitempty"`
	MaxAdditions      int     `json:"max_additions,omitempty"`
	AddSizePct        float64 `json:"add_size_pct,omitempty"`
	AddSizeMultiplier float64 `json:"add_size_multiplier,omitempty"`
	// PositionLeverage scales the initial allocation. Multiplier additions
	// inherit it through the actual first-entry notional.
	PositionLeverage float64 `json:"position_leverage,omitempty"`
	// Capital decay reduces new initial positions as equity grows. It never
	// enlarges positions when equity is below the initial account balance.
	LongCapitalDecayPower  float64 `json:"long_capital_decay_power,omitempty"`
	ShortCapitalDecayPower float64 `json:"short_capital_decay_power,omitempty"`
	// MaxPositionPct and TargetLeverage are legacy aliases retained for
	// decoding old revisions. Normalize clears both fields.
	MaxPositionPct float64 `json:"max_position_pct,omitempty"`
	TargetLeverage float64 `json:"target_leverage,omitempty"`
}

type ExecutionSpec struct {
	SignalTime       SignalTime       `json:"signal_time"`
	MarketFillTime   MarketFillTime   `json:"market_fill_time"`
	IntrabarConflict IntrabarConflict `json:"intrabar_conflict"`
	Fee              FeeSpec          `json:"fee"`
	Slippage         SlippageSpec     `json:"slippage"`
}

type SignalTime string

const (
	SignalTimeClose SignalTime = "bar_close"
)

type MarketFillTime string

const (
	MarketFillNextOpen MarketFillTime = "next_bar_open"
)

type IntrabarConflict string

const (
	IntrabarConflictConservative IntrabarConflict = "conservative"
)

type FeeSpec struct {
	MakerRate float64 `json:"maker_rate"`
	TakerRate float64 `json:"taker_rate"`
}

type SlippageSpec struct {
	Type  SlippageType `json:"type"`
	Value float64      `json:"value"`
}

type SlippageType string

const (
	SlippageTypeNone SlippageType = "none"
	SlippageTypeBPS  SlippageType = "bps"
)

type RiskSpec struct {
	MaxSymbolEquityPct  float64 `json:"max_symbol_equity_pct"`
	MaxTotalExposurePct float64 `json:"max_total_exposure_pct"`
	MaxOpenPositions    int     `json:"max_open_positions"`
	MaxDrawdownHaltPct  float64 `json:"max_drawdown_halt_pct"`
}

type IndicatorSpec struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timeframe string          `json:"timeframe,omitempty"`
	Inputs    []ValueRef      `json:"inputs"`
	Params    json.RawMessage `json:"params"`
}

type ValueRef struct {
	Scope  ValueScope `json:"scope"`
	Name   string     `json:"name"`
	Field  string     `json:"field"`
	Offset int        `json:"offset,omitempty"`
}

type ValueScope string

const (
	ValueScopeMarket    ValueScope = "market"
	ValueScopeIndicator ValueScope = "indicator"
	ValueScopePosition  ValueScope = "position"
)

type RuleSpec struct {
	ID        string     `json:"id"`
	Name      string     `json:"name,omitempty"`
	Disabled  bool       `json:"disabled,omitempty"`
	Priority  int        `json:"priority,omitempty"`
	Condition string     `json:"condition,omitempty"`
	When      Expression `json:"when"`
	Action    ActionSpec `json:"action"`
}

type Expression struct {
	Kind   ExprKind     `json:"kind"`
	Op     Operator     `json:"op,omitempty"`
	Args   []Expression `json:"args,omitempty"`
	Ref    *ValueRef    `json:"ref,omitempty"`
	Number *float64     `json:"number,omitempty"`
	Bool   *bool        `json:"bool,omitempty"`
}

type ExprKind string

const (
	ExprOperator  ExprKind = "operator"
	ExprReference ExprKind = "reference"
	ExprNumber    ExprKind = "number"
	ExprBoolean   ExprKind = "boolean"
)

type Operator string

const (
	OperatorAnd         Operator = "and"
	OperatorOr          Operator = "or"
	OperatorNot         Operator = "not"
	OperatorGT          Operator = "gt"
	OperatorGTE         Operator = "gte"
	OperatorLT          Operator = "lt"
	OperatorLTE         Operator = "lte"
	OperatorEQ          Operator = "eq"
	OperatorCrossUp     Operator = "cross_up"
	OperatorCrossDown   Operator = "cross_down"
	OperatorTurnBullish Operator = "turn_bullish"
	OperatorTurnBearish Operator = "turn_bearish"
)

type ActionSpec struct {
	Type    ActionType `json:"type"`
	Side    TradeSide  `json:"side"`
	SizePct float64    `json:"size_pct"`
}

type ActionType string

const (
	ActionOpen  ActionType = "open"
	ActionClose ActionType = "close"
)

type TradeSide string

const (
	TradeSideLong  TradeSide = "long"
	TradeSideShort TradeSide = "short"
)
