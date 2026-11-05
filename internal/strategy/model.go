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
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Data          DataSelection    `json:"data"`
	Account       AccountSpec      `json:"account"`
	Behavior      BehaviorSpec     `json:"behavior"`
	Position      PositionSpec     `json:"position,omitempty"`
	ExitPolicies  []ExitPolicySpec `json:"exit_policies,omitempty"`
	Execution     ExecutionSpec    `json:"execution"`
	Risk          RiskSpec         `json:"risk"`
	Indicators    []IndicatorSpec  `json:"indicators"`
	Rules         []RuleSpec       `json:"rules,omitempty"`
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
	// AllowAdditions is the explicit master switch. Nil preserves the
	// pre-switch behavior of historical enter/add and legacy open revisions.
	AllowAdditions       *bool `json:"allow_additions,omitempty"`
	AllowPyramiding      bool  `json:"allow_pyramiding"`
	AllowSameBarReversal bool  `json:"allow_same_bar_reversal"`
	CooldownBars         int   `json:"cooldown_bars"`
}

type PositionSpec struct {
	// Legacy sizing fields are retained only for ActionOpen revisions.
	// New enter/add actions carry their own allocation and addition index.
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

type ExitPolicySpec struct {
	ID           string           `json:"id"`
	Disabled     bool             `json:"disabled,omitempty"`
	Side         TradeSide        `json:"side"`
	Purpose      ExitPurpose      `json:"purpose"`
	Anchor       ExitAnchorSpec   `json:"anchor"`
	Distance     ExitDistanceSpec `json:"distance"`
	TriggerPrice ExitTriggerPrice `json:"trigger_price,omitempty"`
	ClosePct     float64          `json:"close_pct,omitempty"`
	Priority     int              `json:"priority,omitempty"`
	Execution    ExitExecution    `json:"execution,omitempty"`
}

type ExitAnchorSpec struct {
	Type      ExitAnchorType `json:"type"`
	Indicator string         `json:"indicator,omitempty"`
	Field     string         `json:"field,omitempty"`
}

type ExitAnchorType string

const (
	ExitAnchorAverageEntry      ExitAnchorType = "avg_entry_price"
	ExitAnchorFirstEntry        ExitAnchorType = "first_entry_price"
	ExitAnchorLastEntry         ExitAnchorType = "last_entry_price"
	ExitAnchorHighestSinceEntry ExitAnchorType = "highest_since_entry"
	ExitAnchorLowestSinceEntry  ExitAnchorType = "lowest_since_entry"
	ExitAnchorIndicator         ExitAnchorType = "indicator"
)

type ExitDistanceSpec struct {
	Type      ExitDistanceType `json:"type"`
	Value     float64          `json:"value"`
	Indicator string           `json:"indicator,omitempty"`
	Field     string           `json:"field,omitempty"`
	Sampling  ExitSampling     `json:"sampling,omitempty"`
}

type ExitDistanceType string

const (
	ExitDistancePercent     ExitDistanceType = "percent"
	ExitDistanceATRMultiple ExitDistanceType = "atr_multiple"
	ExitDistanceFixedAmount ExitDistanceType = "fixed_amount"
	ExitDistanceDirect      ExitDistanceType = "direct"
)

type ExitSampling string

const (
	ExitSamplingEntry     ExitSampling = "entry"
	ExitSamplingEachEntry ExitSampling = "each_entry"
	ExitSamplingLive      ExitSampling = "live"
)

type ExitPurpose string

const (
	ExitPurposeTakeProfit ExitPurpose = "take_profit"
	ExitPurposeStopLoss   ExitPurpose = "stop_loss"
)

type ExitTriggerPrice string

const (
	ExitTriggerClose ExitTriggerPrice = "close"
	ExitTriggerHigh  ExitTriggerPrice = "high"
	ExitTriggerLow   ExitTriggerPrice = "low"
)

type ExitExecution string

const (
	ExitExecutionNextOpen     ExitExecution = "next_open"
	ExitExecutionTriggerPrice ExitExecution = "trigger_price"
)

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
	// AdditionTrigger is ANDed with When and only applies to an add action.
	AdditionTrigger *AdditionTriggerSpec `json:"addition_trigger,omitempty"`
	// SourceRuleID preserves the ordering of rules split from a legacy open.
	// It is immutable provenance, not a reference to a live rule.
	SourceRuleID string `json:"source_rule_id,omitempty"`
}

type AdditionTriggerSpec struct {
	Anchor       ExitAnchorSpec       `json:"anchor"`
	Distance     ExitDistanceSpec     `json:"distance"`
	Direction    AdditionDirection    `json:"direction,omitempty"`
	TriggerPrice ExitTriggerPrice     `json:"trigger_price,omitempty"`
	Spacing      *AdditionSpacingSpec `json:"spacing,omitempty"`
}

type AdditionDirection string

const (
	AdditionAdverse   AdditionDirection = "adverse"
	AdditionFavorable AdditionDirection = "favorable"
)

type AdditionSpacingSpec struct {
	Mode  AdditionSpacingMode `json:"mode"`
	Value float64             `json:"value,omitempty"`
}

type AdditionSpacingMode string

const (
	AdditionSpacingFixed     AdditionSpacingMode = "fixed"
	AdditionSpacingLinear    AdditionSpacingMode = "linear"
	AdditionSpacingGeometric AdditionSpacingMode = "geometric"
)

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
	// AdditionIndex is 1 for the first addition, 2 for the second, etc.
	AdditionIndex int `json:"addition_index,omitempty"`
	// SizeMultiplier is the factor used by the selected addition sizing mode.
	// It is mutually exclusive with SizePct.
	SizeMultiplier float64            `json:"size_multiplier,omitempty"`
	SizingMode     AdditionSizingMode `json:"sizing_mode,omitempty"`
}

type AdditionSizingMode string

const (
	AdditionSizeEquityPct       AdditionSizingMode = "equity_pct"
	AdditionSizeFirstMultiple   AdditionSizingMode = "first_entry_multiple"
	AdditionSizeCurrentMultiple AdditionSizingMode = "current_position_multiple"
	AdditionSizeFirstGeometric  AdditionSizingMode = "first_entry_geometric"
)

func (action ActionSpec) EffectiveSizingMode() AdditionSizingMode {
	if action.SizingMode != "" {
		return action.SizingMode
	}
	if action.SizeMultiplier > 0 {
		return AdditionSizeFirstMultiple
	}
	return AdditionSizeEquityPct
}

type ActionType string

const (
	ActionEnter ActionType = "enter"
	ActionAdd   ActionType = "add"
	// ActionOpen retains legacy combined entry/pyramiding behavior.
	ActionOpen  ActionType = "open"
	ActionClose ActionType = "close"
)

type TradeSide string

const (
	TradeSideLong  TradeSide = "long"
	TradeSideShort TradeSide = "short"
)
