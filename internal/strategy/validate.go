package strategy

import (
	"fmt"
	"math"
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
	if err := validatePosition(definition.Position, definition.Behavior, definition.Account); err != nil {
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
	if len(data.Symbols) != 1 {
		return fmt.Errorf("data.symbols must contain exactly one symbol")
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
	if !isFinite(account.InitialCash) || account.InitialCash <= 0 {
		return fmt.Errorf("account.initial_cash must be > 0")
	}
	if strings.TrimSpace(account.QuoteCurrency) == "" {
		return fmt.Errorf("account.quote_currency is required")
	}
	if math.IsNaN(account.ExchangeLeverage) ||
		math.IsInf(account.ExchangeLeverage, 0) ||
		account.ExchangeLeverage < 1 {
		return fmt.Errorf("account.exchange_leverage must be finite and >= 1")
	}
	if account.MarketType == MarketTypeSpot && account.ExchangeLeverage != 1 {
		return fmt.Errorf("spot account.exchange_leverage must be 1")
	}
	return nil
}

func validateBehavior(behavior BehaviorSpec) error {
	if behavior.AllowSameBarReversal {
		return fmt.Errorf("behavior.allow_same_bar_reversal is not supported by the next-bar-open engine")
	}
	if behavior.CooldownBars < 0 {
		return fmt.Errorf("behavior.cooldown_bars must be >= 0")
	}
	return nil
}

func validatePosition(position PositionSpec, behavior BehaviorSpec, account AccountSpec) error {
	for field, value := range map[string]float64{
		"initial_size_pct":          position.InitialSizePct,
		"add_size_pct":              position.AddSizePct,
		"add_size_multiplier":       position.AddSizeMultiplier,
		"position_leverage":         position.PositionLeverage,
		"long_capital_decay_power":  position.LongCapitalDecayPower,
		"short_capital_decay_power": position.ShortCapitalDecayPower,
	} {
		if !isFinite(value) {
			return fmt.Errorf("position.%s must be finite", field)
		}
	}
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
	if position.PositionLeverage < 1 ||
		position.PositionLeverage > account.ExchangeLeverage {
		return fmt.Errorf(
			"position.position_leverage must be finite and in [1, account.exchange_leverage]",
		)
	}
	if position.LongCapitalDecayPower <= 0 {
		return fmt.Errorf("position.long_capital_decay_power must be finite and > 0")
	}
	if position.ShortCapitalDecayPower <= 0 {
		return fmt.Errorf("position.short_capital_decay_power must be finite and > 0")
	}
	if position.MaxAdditions > 0 && !behavior.AllowPyramiding {
		return fmt.Errorf("position.max_additions requires behavior.allow_pyramiding=true")
	}
	if position.AddSizePct > 0 && position.AddSizeMultiplier > 0 {
		return fmt.Errorf("position.add_size_pct and position.add_size_multiplier are mutually exclusive")
	}
	if position.MaxAdditions == 0 && (position.AddSizePct > 0 || position.AddSizeMultiplier > 0) {
		return fmt.Errorf("position addition sizing requires max_additions > 0")
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
	if !isFinite(risk.MaxDrawdownHaltPct) {
		return fmt.Errorf("risk.max_drawdown_halt_pct must be finite")
	}
	if risk.MaxOpenPositions != 1 {
		return fmt.Errorf("risk.max_open_positions must be 1 for the single-symbol engine")
	}
	if risk.MaxDrawdownHaltPct < 0 || risk.MaxDrawdownHaltPct > 1 {
		return fmt.Errorf("risk.max_drawdown_halt_pct must be in [0, 1]")
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type indicatorOutputs map[string]map[string]indicator.OutputDefinition

func validateIndicators(indicators []IndicatorSpec, registry *indicator.Registry, baseTimeframe string) (indicatorOutputs, error) {
	if registry == nil {
		return nil, fmt.Errorf("indicator registry is required")
	}
	if len(indicators) == 0 {
		return nil, fmt.Errorf("indicators must not be empty")
	}
	ids := map[string]struct{}{}
	outputsByID := indicatorOutputs{}
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
		requiredInputs := 0
		for _, input := range definition.Inputs {
			if input.Required {
				requiredInputs++
			}
		}
		if len(spec.Inputs) < requiredInputs || len(spec.Inputs) > len(definition.Inputs) {
			return nil, fmt.Errorf(
				"indicator %q requires %d to %d inputs, got %d",
				spec.ID,
				requiredInputs,
				len(definition.Inputs),
				len(spec.Inputs),
			)
		}
		for _, input := range spec.Inputs {
			if err := validateValueRef(input, nil); err != nil {
				return nil, fmt.Errorf("indicator %q input: %w", spec.ID, err)
			}
			if input.Scope != ValueScopeMarket && input.Scope != ValueScopeIndicator {
				return nil, fmt.Errorf("indicator %q input scope must be market or indicator", spec.ID)
			}
		}
		if err := registry.ValidateParams(spec.Type, spec.Params); err != nil {
			return nil, fmt.Errorf("indicator %q params: %w", spec.ID, err)
		}
		outputsByID[spec.ID] = map[string]indicator.OutputDefinition{}
		for _, output := range definition.Outputs {
			outputsByID[spec.ID][output.Name] = output
		}
	}
	for _, spec := range indicators {
		for _, input := range spec.Inputs {
			if err := validateValueRef(input, outputsByID); err != nil {
				return nil, fmt.Errorf("indicator %q input: %w", spec.ID, err)
			}
		}
	}
	if err := validateIndicatorGraph(indicators); err != nil {
		return nil, err
	}
	return outputsByID, nil
}

func validateIndicatorGraph(indicators []IndicatorSpec) error {
	dependencies := make(map[string][]string, len(indicators))
	for _, spec := range indicators {
		seen := map[string]struct{}{}
		for _, input := range spec.Inputs {
			if input.Scope != ValueScopeIndicator {
				continue
			}
			if _, ok := seen[input.Name]; ok {
				continue
			}
			seen[input.Name] = struct{}{}
			dependencies[spec.ID] = append(dependencies[spec.ID], input.Name)
		}
	}

	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(indicators))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("indicator dependency cycle includes %q", id)
		case visited:
			return nil
		}
		state[id] = visiting
		for _, dependency := range dependencies[id] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = visited
		return nil
	}
	for _, spec := range indicators {
		if err := visit(spec.ID); err != nil {
			return err
		}
	}
	return nil
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

func validateRules(rules []RuleSpec, indicatorOutputs indicatorOutputs, marketType MarketType, position PositionSpec) error {
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
		expressionType, err := validateExpression(rule.When, indicatorOutputs)
		if err != nil {
			return fmt.Errorf("rule %q when: %w", rule.ID, err)
		}
		if expressionType != expressionTypeBool {
			return fmt.Errorf("rule %q when: root expression must be boolean", rule.ID)
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

type expressionType int

const (
	expressionTypeNumber expressionType = iota
	expressionTypeBool
)

func validateExpression(expression Expression, indicatorOutputs indicatorOutputs) (expressionType, error) {
	switch expression.Kind {
	case ExprOperator:
		if expression.Ref != nil || expression.Number != nil || expression.Bool != nil {
			return 0, fmt.Errorf("operator expression contains fields from another expression kind")
		}
		return validateOperatorExpression(expression, indicatorOutputs)
	case ExprReference:
		if expression.Ref == nil {
			return 0, fmt.Errorf("reference expression requires ref")
		}
		if expression.Op != "" || len(expression.Args) != 0 || expression.Number != nil || expression.Bool != nil {
			return 0, fmt.Errorf("reference expression contains fields from another expression kind")
		}
		if err := validateValueRef(*expression.Ref, indicatorOutputs); err != nil {
			return 0, err
		}
		return expressionTypeNumber, nil
	case ExprNumber:
		if expression.Number == nil {
			return 0, fmt.Errorf("number expression requires number")
		}
		if expression.Op != "" || len(expression.Args) != 0 || expression.Ref != nil || expression.Bool != nil {
			return 0, fmt.Errorf("number expression contains fields from another expression kind")
		}
		return expressionTypeNumber, nil
	case ExprBoolean:
		if expression.Bool == nil {
			return 0, fmt.Errorf("boolean expression requires bool")
		}
		if expression.Op != "" || len(expression.Args) != 0 || expression.Ref != nil || expression.Number != nil {
			return 0, fmt.Errorf("boolean expression contains fields from another expression kind")
		}
		return expressionTypeBool, nil
	default:
		return 0, fmt.Errorf("unknown expression kind %q", expression.Kind)
	}
}

func validateOperatorExpression(expression Expression, indicatorOutputs indicatorOutputs) (expressionType, error) {
	expectedArgs := map[Operator]int{
		OperatorNot:         1,
		OperatorGT:          2,
		OperatorGTE:         2,
		OperatorLT:          2,
		OperatorLTE:         2,
		OperatorEQ:          2,
		OperatorCrossUp:     2,
		OperatorCrossDown:   2,
		OperatorTurnBullish: 1,
		OperatorTurnBearish: 1,
	}
	switch expression.Op {
	case OperatorAnd, OperatorOr:
		if len(expression.Args) < 2 {
			return 0, fmt.Errorf("operator %q requires at least 2 args", expression.Op)
		}
	case OperatorNot,
		OperatorGT,
		OperatorGTE,
		OperatorLT,
		OperatorLTE,
		OperatorEQ,
		OperatorCrossUp,
		OperatorCrossDown,
		OperatorTurnBullish,
		OperatorTurnBearish:
		if len(expression.Args) != expectedArgs[expression.Op] {
			return 0, fmt.Errorf("operator %q requires %d args", expression.Op, expectedArgs[expression.Op])
		}
	default:
		return 0, fmt.Errorf("unknown operator %q", expression.Op)
	}
	expectedType := expressionTypeNumber
	if expression.Op == OperatorAnd || expression.Op == OperatorOr || expression.Op == OperatorNot {
		expectedType = expressionTypeBool
	}
	for index, arg := range expression.Args {
		actualType, err := validateExpression(arg, indicatorOutputs)
		if err != nil {
			return 0, err
		}
		if actualType != expectedType {
			return 0, fmt.Errorf("operator %q arg %d has incompatible type", expression.Op, index)
		}
	}
	if expression.Op == OperatorCrossUp || expression.Op == OperatorCrossDown {
		for index, arg := range expression.Args {
			if arg.Kind != ExprReference || arg.Ref == nil || arg.Ref.Scope != ValueScopeIndicator {
				return 0, fmt.Errorf("operator %q arg %d must reference an indicator output", expression.Op, index)
			}
		}
		left := expression.Args[0].Ref
		right := expression.Args[1].Ref
		if left.Name == right.Name && left.Field == right.Field {
			return 0, fmt.Errorf("operator %q requires two different indicator outputs", expression.Op)
		}
		leftOutput, leftOK := lookupIndicatorOutput(*left, indicatorOutputs)
		rightOutput, rightOK := lookupIndicatorOutput(*right, indicatorOutputs)
		if !leftOK || !rightOK ||
			leftOutput.CrossGroup == "" ||
			leftOutput.CrossGroup != rightOutput.CrossGroup {
			return 0, fmt.Errorf("operator %q requires outputs from the same cross group", expression.Op)
		}
	}
	if expression.Op == OperatorTurnBullish || expression.Op == OperatorTurnBearish {
		arg := expression.Args[0]
		if arg.Kind != ExprReference || arg.Ref == nil {
			return 0, fmt.Errorf("operator %q requires an indicator reference", expression.Op)
		}
		output, ok := lookupIndicatorOutput(*arg.Ref, indicatorOutputs)
		if !ok || output.Semantic != indicator.OutputSemanticTrendDirection {
			return 0, fmt.Errorf("operator %q requires a trend direction output", expression.Op)
		}
	}
	if expression.Op == OperatorGT ||
		expression.Op == OperatorGTE ||
		expression.Op == OperatorLT ||
		expression.Op == OperatorLTE ||
		expression.Op == OperatorEQ {
		if err := validateComparisonSemantics(expression, indicatorOutputs); err != nil {
			return 0, err
		}
	}
	return expressionTypeBool, nil
}

func validateValueRef(ref ValueRef, indicatorOutputs indicatorOutputs) error {
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
		case "quantity", "abs_quantity", "entry_price", "avg_entry_price", "last_entry_price", "entry_count", "unrealized_pnl_pct", "side", "actual_leverage":
			return nil
		default:
			return fmt.Errorf("unknown position field %q", ref.Field)
		}
	default:
		return fmt.Errorf("unknown scope %q", ref.Scope)
	}
}

func lookupIndicatorOutput(
	ref ValueRef,
	outputs indicatorOutputs,
) (indicator.OutputDefinition, bool) {
	if ref.Scope != ValueScopeIndicator || outputs == nil {
		return indicator.OutputDefinition{}, false
	}
	fields, ok := outputs[ref.Name]
	if !ok {
		return indicator.OutputDefinition{}, false
	}
	output, ok := fields[ref.Field]
	return output, ok
}

func validateComparisonSemantics(expression Expression, outputs indicatorOutputs) error {
	for index, arg := range expression.Args {
		if arg.Ref == nil {
			continue
		}
		output, ok := lookupIndicatorOutput(*arg.Ref, outputs)
		if !ok || output.Semantic != indicator.OutputSemanticTrendDirection {
			continue
		}
		if expression.Op != OperatorEQ {
			return fmt.Errorf("trend direction only supports operator %q", OperatorEQ)
		}
		other := expression.Args[1-index]
		if other.Kind != ExprNumber ||
			other.Number == nil ||
			(*other.Number != 1 && *other.Number != -1) {
			return fmt.Errorf("trend direction must be compared with 1 or -1")
		}
	}

	leftDomain := referenceDomain(expression.Args[0], outputs)
	rightDomain := referenceDomain(expression.Args[1], outputs)
	if leftDomain != "" && rightDomain != "" && leftDomain != rightDomain {
		return fmt.Errorf(
			"operator %q cannot compare %s with %s",
			expression.Op,
			leftDomain,
			rightDomain,
		)
	}
	return nil
}

func referenceDomain(expression Expression, outputs indicatorOutputs) string {
	if expression.Kind != ExprReference || expression.Ref == nil {
		return ""
	}
	ref := *expression.Ref
	switch ref.Scope {
	case ValueScopeMarket:
		switch ref.Field {
		case "open", "high", "low", "close":
			return "price"
		case "volume", "quote_volume", "trade_count":
			return "volume"
		}
	case ValueScopeIndicator:
		output, ok := lookupIndicatorOutput(ref, outputs)
		if !ok {
			return ""
		}
		switch output.Semantic {
		case indicator.OutputSemanticPrice, indicator.OutputSemanticTrendLevel:
			return "price"
		case indicator.OutputSemanticOscillator:
			return "oscillator"
		case indicator.OutputSemanticVolatility:
			return "volatility"
		case indicator.OutputSemanticTrendDirection:
			return "trend_direction"
		}
	case ValueScopePosition:
		switch ref.Field {
		case "entry_price", "avg_entry_price", "last_entry_price":
			return "price"
		case "quantity", "abs_quantity":
			return "quantity"
		case "entry_count":
			return "count"
		case "unrealized_pnl_pct":
			return "ratio"
		case "side":
			return "side"
		case "actual_leverage":
			return "leverage"
		}
	}
	return ""
}
