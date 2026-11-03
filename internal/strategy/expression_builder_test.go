package strategy

import "testing"

func TestExpressionBuilderCreatesChainedConditions(t *testing.T) {
	expression := And(
		Indicator("ema_15m").GTE(2000),
		Indicator("ema_4h").GTE(2100),
	)
	if expression.Kind != ExprOperator || expression.Op != OperatorAnd || len(expression.Args) != 2 {
		t.Fatalf("unexpected root expression: %+v", expression)
	}
	assertIndicatorComparison(t, expression.Args[0], "ema_15m", OperatorGTE, 2000)
	assertIndicatorComparison(t, expression.Args[1], "ema_4h", OperatorGTE, 2100)
}

func TestParseConditionSupportsChainedIndicatorConditions(t *testing.T) {
	expression, err := ParseCondition("and(ema_15m.gte(2000), ema_4h.gte(2100))")
	if err != nil {
		t.Fatal(err)
	}
	if expression.Kind != ExprOperator || expression.Op != OperatorAnd || len(expression.Args) != 2 {
		t.Fatalf("unexpected root expression: %+v", expression)
	}
	assertIndicatorComparison(t, expression.Args[0], "ema_15m", OperatorGTE, 2000)
	assertIndicatorComparison(t, expression.Args[1], "ema_4h", OperatorGTE, 2100)
}

func TestParseConditionSupportsMarketAndIndicatorFields(t *testing.T) {
	expression, err := ParseCondition("and(market.close.cross_up(boll_15m.middle), rsi_4h.lt(70))")
	if err != nil {
		t.Fatal(err)
	}
	if expression.Op != OperatorAnd || len(expression.Args) != 2 {
		t.Fatalf("unexpected expression: %+v", expression)
	}
	first := expression.Args[0]
	if first.Op != OperatorCrossUp {
		t.Fatalf("first op=%q, want cross_up", first.Op)
	}
	if first.Args[0].Ref == nil || first.Args[0].Ref.Scope != ValueScopeMarket || first.Args[0].Ref.Field != "close" {
		t.Fatalf("unexpected market ref: %+v", first.Args[0])
	}
	if first.Args[1].Ref == nil || first.Args[1].Ref.Name != "boll_15m" || first.Args[1].Ref.Field != "middle" {
		t.Fatalf("unexpected indicator field ref: %+v", first.Args[1])
	}
}

func TestParseConditionSupportsPositionState(t *testing.T) {
	expression, err := ParseCondition("and(position.flat(), ema_15m.gte(2000))")
	if err != nil {
		t.Fatal(err)
	}
	if expression.Kind != ExprOperator || expression.Op != OperatorAnd || len(expression.Args) != 2 {
		t.Fatalf("unexpected root expression: %+v", expression)
	}
	positionExpr := expression.Args[0]
	if positionExpr.Op != OperatorEQ || len(positionExpr.Args) != 2 {
		t.Fatalf("unexpected position expression: %+v", positionExpr)
	}
	if positionExpr.Args[0].Ref == nil || positionExpr.Args[0].Ref.Scope != ValueScopePosition || positionExpr.Args[0].Ref.Field != "quantity" {
		t.Fatalf("unexpected position ref: %+v", positionExpr.Args[0])
	}
	if positionExpr.Args[1].Number == nil || *positionExpr.Args[1].Number != 0 {
		t.Fatalf("unexpected position comparison target: %+v", positionExpr.Args[1])
	}
}

func TestParseConditionSupportsTrendDirectionTransitions(t *testing.T) {
	bullish, err := ParseCondition("supertrend.direction.turn_bullish()")
	if err != nil {
		t.Fatal(err)
	}
	if bullish.Op != OperatorTurnBullish ||
		len(bullish.Args) != 1 ||
		bullish.Args[0].Ref == nil ||
		bullish.Args[0].Ref.Name != "supertrend" ||
		bullish.Args[0].Ref.Field != "direction" {
		t.Fatalf("unexpected bullish transition: %+v", bullish)
	}

	bearish := IndicatorField("supertrend", "direction").TurnsBearish()
	if bearish.Op != OperatorTurnBearish || len(bearish.Args) != 1 {
		t.Fatalf("unexpected bearish transition: %+v", bearish)
	}
}

func TestParseConditionSupportsPositionFieldsForPyramiding(t *testing.T) {
	expression, err := ParseCondition("and(position.long(), position.entry_count.lt(3), position.avg_entry_price.gte(market.close))")
	if err != nil {
		t.Fatal(err)
	}
	if expression.Kind != ExprOperator || expression.Op != OperatorAnd || len(expression.Args) != 3 {
		t.Fatalf("unexpected root expression: %+v", expression)
	}
	entryCount := expression.Args[1]
	if entryCount.Op != OperatorLT || entryCount.Args[0].Ref == nil || entryCount.Args[0].Ref.Field != "entry_count" {
		t.Fatalf("unexpected entry_count expression: %+v", entryCount)
	}
	avgEntry := expression.Args[2]
	if avgEntry.Op != OperatorGTE || avgEntry.Args[0].Ref == nil || avgEntry.Args[0].Ref.Field != "avg_entry_price" {
		t.Fatalf("unexpected avg_entry_price expression: %+v", avgEntry)
	}
	if avgEntry.Args[1].Ref == nil || avgEntry.Args[1].Ref.Scope != ValueScopeMarket || avgEntry.Args[1].Ref.Field != "close" {
		t.Fatalf("unexpected market close ref: %+v", avgEntry.Args[1])
	}
}

func TestNormalizeParsesRuleCondition(t *testing.T) {
	definition := validDefinition()
	definition.Rules = []RuleSpec{{
		ID:        "open_long",
		Condition: "and(ema_fast.gte(2000), ema_fast.lt(3000))",
		Action: ActionSpec{
			Type:    ActionOpen,
			Side:    TradeSideLong,
			SizePct: 0.2,
		},
	}}
	normalized, err := Normalize(definition)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Rules[0].When.Kind != ExprOperator {
		t.Fatalf("expected parsed expression, got %+v", normalized.Rules[0].When)
	}
}

func assertIndicatorComparison(t *testing.T, expression Expression, indicatorID string, op Operator, number float64) {
	t.Helper()
	if expression.Kind != ExprOperator || expression.Op != op || len(expression.Args) != 2 {
		t.Fatalf("unexpected comparison expression: %+v", expression)
	}
	left := expression.Args[0]
	if left.Ref == nil || left.Ref.Scope != ValueScopeIndicator || left.Ref.Name != indicatorID || left.Ref.Field != "value" {
		t.Fatalf("unexpected left ref: %+v", left)
	}
	right := expression.Args[1]
	if right.Number == nil || *right.Number != number {
		t.Fatalf("right number=%v, want %v", right.Number, number)
	}
}
