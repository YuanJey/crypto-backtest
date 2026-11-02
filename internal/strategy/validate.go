package strategy

import (
	"fmt"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
)

type IndicatorParamValidator interface {
	ValidateParams(indicatorType string, params []byte) error
}

func Validate(definition Definition, registry *indicator.Registry) error {
	normalized, err := Normalize(definition)
	if err != nil {
		return err
	}
	definition = normalized
	if definition.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if strings.TrimSpace(definition.ID) == "" {
		return fmt.Errorf("strategy id is required")
	}
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("strategy name is required")
	}
	if err := validateData(definition.Data); err != nil {
		return err
	}
	if err := validateAccount(definition.Account); err != nil {
		return err
	}
	if err := validateBehavior(definition.Behavior); err != nil {
		return err
	}
	if err := validatePosition(definition.Position); err != nil {
		return err
	}
	if err := validateExecution(definition.Execution); err != nil {
		return err
	}
	if err := validateRisk(definition.Risk); err != nil {
		return err
	}
	indicatorOutputs, err := validateIndicators(definition.Indicators, registry, definition.Data.Timeframe)
	if err != nil {
		return err
	}
	return validateRules(definition.Rules, indicatorOutputs, definition.Account.MarketType, definition.Position)
}

func validateData(data DataSelection) error {
	if strings.TrimSpace(data.DatasetID) == "" {
		return fmt.Errorf("data.dataset_id is required")
	}
	if len(data.Symbols) == 0 {
		return fmt.Errorf("data.symbols must not be empty")
	}
	for _, symbol := range data.Symbols {
		if _, err := market.NormalizeSymbol(symbol); err != nil {
			return fmt.Errorf("data.symbols: %w", err)
		}
	}
	if err := market.RequireBaseTimeframe(data.Timeframe); err != nil {
		return fmt.Errorf("data.timeframe: %w", err)
	}
	if data.Start.IsZero() || data.End.IsZero() {
		return fmt.Errorf("data.start and data.end are required")
	}
	if !data.Start.Before(data.End) {
		return fmt.Errorf("data.start must be earlier than data.end")
	}
	timeframeMS, err := market.ParseTimeframeMS(data.Timeframe)
	if err != nil {
		return fmt.Errorf("data.timeframe: %w", err)
	}
	if err := market.RequireAlignedRange(data.Start.UTC().UnixMilli(), data.End.UTC().UnixMilli(), timeframeMS); err != nil {
		return fmt.Errorf("data range: %w", err)
	}
	return nil
}

func validateAccount(account AccountSpec) error {
	switch account.MarketType {
	case MarketTypeSpot, MarketTypePerpetual:
	default:
		return fmt.Errorf("account.market_type is invalid")
	}
	if account.InitialCash <= 0 {
		return fmt.Errorf("account.initial_cash must be > 0")
	}
	if strings.TrimSpace(account.QuoteCurrency) == "" {
		return fmt.Errorf("account.quote_currency is required")
	}
	return nil
}

func validateBehavior(behavior BehaviorSpec) error {
	if behavior.CooldownBars < 0 {
		return fmt.Errorf("behavior.cooldown_bars must be >= 0")
	}
	return nil
}

func validatePosition(position PositionSpec) error {
	if position.InitialSizePct < 0 || position.InitialSizePct > 1 {
		return fmt.Errorf("position.initial_size_pct must be in [0, 1]")
	}
	if position.MaxAdditions < 0 {
		return fmt.Errorf("position.max_additions must be >= 0")
	}
	if position.AddSizePct < 0 || position.AddSizePct > 1 {
		return fmt.Errorf("position.add_size_pct must be in [0, 1]")
	}
	if position.AddSizeMultiplier < 0 {
		return fmt.Errorf("position.add_size_multiplier must be >= 0")
	}
	if position.MaxPositionPct < 0 || position.MaxPositionPct > 1 {
		return fmt.Errorf("position.max_position_pct must be in [0, 1]")
	}
	return nil
}

func validateExecution(execution ExecutionSpec) error {
	if execution.SignalTime != SignalTimeClose {
		return fmt.Errorf("execution.signal_time must be %q", SignalTimeClose)
	}
	if execution.MarketFillTime != MarketFillNextOpen {
		return fmt.Errorf("execution.market_fill_time must be %q", MarketFillNextOpen)
	}
	if execution.IntrabarConflict != IntrabarConflictConservative {
		return fmt.Errorf("execution.intrabar_conflict must be %q", IntrabarConflictConservative)
	}
	if execution.Fee.MakerRate < 0 || execution.Fee.TakerRate < 0 {
		return fmt.Errorf("execution.fee rates must be >= 0")
	}
	switch execution.Slippage.Type {
	case SlippageTypeNone, SlippageTypeBPS:
	default:
		return fmt.Errorf("execution.slippage.type is invalid")
	}
	if execution.Slippage.Value < 0 {
		return fmt.Errorf("execution.slippage.value must be >= 0")
	}
	return nil
}

func validateRisk(risk RiskSpec) error {
	if risk.MaxSymbolEquityPct <= 0 || risk.MaxSymbolEquityPct > 1 {
		return fmt.Errorf("risk.max_symbol_equity_pct must be in (0, 1]")
	}
	if risk.MaxTotalExposurePct <= 0 || risk.MaxTotalExposurePct > 1 {
		return fmt.Errorf("risk.max_total_exposure_pct must be in (0, 1]")
	}
	if risk.MaxOpenPositions < 1 {
		return fmt.Errorf("risk.max_open_positions must be >= 1")
	}
	if risk.MaxDrawdownHaltPct < 0 || risk.MaxDrawdownHaltPct > 1 {
		return fmt.Errorf("risk.max_drawdown_halt_pct must be in [0, 1]")
	}
	return nil
}

func validateIndicators(indicators []IndicatorSpec, registry *indicator.Registry, baseTimeframe string) (map[string]map[string]struct{}, error) {
	if registry == nil {
		return nil, fmt.Errorf("indicator registry is required")
	}
	if len(indicators) == 0 {
		return nil, fmt.Errorf("indicators must not be empty")
	}
	ids := map[string]struct{}{}
	outputsByID := map[string]map[string]struct{}{}
	for _, spec := range indicators {
		if strings.TrimSpace(spec.ID) == "" {
			return nil, fmt.Errorf("indicator id is required")
		}
		if _, exists := ids[spec.ID]; exists {
			return nil, fmt.Errorf("duplicate indicator id %q", spec.ID)
		}
		ids[spec.ID] = struct{}{}
		if strings.TrimSpace(spec.Type) == "" {
			return nil, fmt.Errorf("indicator %q type is required", spec.ID)
		}
		definition, ok := registry.Definition(spec.Type)
		if !ok {
			return nil, fmt.Errorf("indicator %q has unknown type %q", spec.ID, spec.Type)
		}
		if err := validateIndicatorTimeframe(spec.Timeframe, baseTimeframe); err != nil {
			return nil, fmt.Errorf("indicator %q timeframe: %w", spec.ID, err)
		}
		if len(spec.Inputs) == 0 {
			return nil, fmt.Errorf("indicator %q inputs must not be empty", spec.ID)
		}
		for _, input := range spec.Inputs {
			if err := validateValueRef(input, nil); err != nil {
				return nil, fmt.Errorf("indicator %q input: %w", spec.ID, err)
			}
		}
		if err := registry.ValidateParams(spec.Type, spec.Params); err != nil {
			return nil, fmt.Errorf("indicator %q params: %w", spec.ID, err)
		}
		outputsByID[spec.ID] = map[string]struct{}{}
		for _, output := range definition.Outputs {
			outputsByID[spec.ID][output.Name] = struct{}{}
		}
	}
	for _, spec := range indicators {
		for _, input := range spec.Inputs {
			if err := validateValueRef(input, outputsByID); err != nil {
				return nil, fmt.Errorf("indicator %q input: %w", spec.ID, err)
			}
		}
	}
	return outputsByID, nil
}

func validateIndicatorTimeframe(timeframe string, baseTimeframe string) error {
	if timeframe == "" {
		timeframe = baseTimeframe
	}
	timeframeMS, err := market.ParseTimeframeMS(timeframe)
	if err != nil {
		return err
	}
	baseTimeframeMS, err := market.ParseTimeframeMS(baseTimeframe)
	if err != nil {
		return err
	}
	if timeframeMS < baseTimeframeMS {
		return fmt.Errorf("must be >= base timeframe %s", baseTimeframe)
	}
	if timeframeMS%baseTimeframeMS != 0 {
		return fmt.Errorf("must be an integer multiple of base timeframe %s", baseTimeframe)
	}
	return nil
}

func validateRules(rules []RuleSpec, indicatorOutputs map[string]map[string]struct{}, marketType MarketType, position PositionSpec) error {
	ids := map[string]struct{}{}
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("rule id is required")
		}
		if _, exists := ids[rule.ID]; exists {
			return fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		ids[rule.ID] = struct{}{}
		if rule.Disabled {
			continue
		}
		if err := validateExpression(rule.When, indicatorOutputs); err != nil {
			return fmt.Errorf("rule %q when: %w", rule.ID, err)
		}
		if err := validateAction(rule.Action, marketType, position); err != nil {
			return fmt.Errorf("rule %q action: %w", rule.ID, err)
		}
	}
	return nil
}

func validateAction(action ActionSpec, marketType MarketType, position PositionSpec) error {
	switch action.Type {
	case ActionOpen, ActionClose:
	default:
		return fmt.Errorf("type must be open or close")
	}
	switch action.Side {
	case TradeSideLong:
	case TradeSideShort:
		if marketType == MarketTypeSpot {
			return fmt.Errorf("short side requires perpetual market")
		}
	default:
		return fmt.Errorf("side must be long or short")
	}
	if action.SizePct < 0 || action.SizePct > 1 {
		return fmt.Errorf("size_pct must be in [0, 1]")
	}
	if action.Type == ActionOpen && action.SizePct == 0 && position.InitialSizePct <= 0 {
		return fmt.Errorf("open action requires size_pct or position.initial_size_pct")
	}
	return nil
}

func validateExpression(expression Expression, indicatorOutputs map[string]map[string]struct{}) error {
	switch expression.Kind {
	case ExprOperator:
		return validateOperatorExpression(expression, indicatorOutputs)
	case ExprReference:
		if expression.Ref == nil {
			return fmt.Errorf("reference expression requires ref")
		}
		return validateValueRef(*expression.Ref, indicatorOutputs)
	case ExprNumber:
		if expression.Number == nil {
			return fmt.Errorf("number expression requires number")
		}
		return nil
	case ExprBoolean:
		if expression.Bool == nil {
			return fmt.Errorf("boolean expression requires bool")
		}
		return nil
	default:
		return fmt.Errorf("unknown expression kind %q", expression.Kind)
	}
}

func validateOperatorExpression(expression Expression, indicatorOutputs map[string]map[string]struct{}) error {
	expectedArgs := map[Operator]int{
		OperatorNot:       1,
		OperatorGT:        2,
		OperatorGTE:       2,
		OperatorLT:        2,
		OperatorLTE:       2,
		OperatorEQ:        2,
		OperatorCrossUp:   2,
		OperatorCrossDown: 2,
	}
	switch expression.Op {
	case OperatorAnd, OperatorOr:
		if len(expression.Args) < 2 {
			return fmt.Errorf("operator %q requires at least 2 args", expression.Op)
		}
	case OperatorNot, OperatorGT, OperatorGTE, OperatorLT, OperatorLTE, OperatorEQ, OperatorCrossUp, OperatorCrossDown:
		if len(expression.Args) != expectedArgs[expression.Op] {
			return fmt.Errorf("operator %q requires %d args", expression.Op, expectedArgs[expression.Op])
		}
	default:
		return fmt.Errorf("unknown operator %q", expression.Op)
	}
	for _, arg := range expression.Args {
		if err := validateExpression(arg, indicatorOutputs); err != nil {
			return err
		}
	}
	return nil
}

func validateValueRef(ref ValueRef, indicatorOutputs map[string]map[string]struct{}) error {
	if ref.Offset > 0 {
		return fmt.Errorf("offset must be <= 0")
	}
	switch ref.Scope {
	case ValueScopeMarket:
		if ref.Name != "bar" {
			return fmt.Errorf("market ref name must be bar")
		}
		switch ref.Field {
		case "open", "high", "low", "close", "volume", "quote_volume", "trade_count":
			return nil
		default:
			return fmt.Errorf("unknown market field %q", ref.Field)
		}
	case ValueScopeIndicator:
		if strings.TrimSpace(ref.Name) == "" || strings.TrimSpace(ref.Field) == "" {
			return fmt.Errorf("indicator ref requires name and field")
		}
		if indicatorOutputs != nil {
			fields, ok := indicatorOutputs[ref.Name]
			if !ok {
				return fmt.Errorf("unknown indicator %q", ref.Name)
			}
			if _, ok := fields[ref.Field]; !ok {
				return fmt.Errorf("indicator %q has no output %q", ref.Name, ref.Field)
			}
		}
		return nil
	case ValueScopePosition:
		if ref.Name != "current" {
			return fmt.Errorf("position ref name must be current")
		}
		switch ref.Field {
		case "quantity", "abs_quantity", "entry_price", "avg_entry_price", "last_entry_price", "entry_count", "unrealized_pnl_pct", "side":
			return nil
		default:
			return fmt.Errorf("unknown position field %q", ref.Field)
		}
	default:
		return fmt.Errorf("unknown scope %q", ref.Scope)
	}
}
