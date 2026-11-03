package strategy

import "fmt"

type SeriesRef struct {
	ref ValueRef
}

func Indicator(id string) SeriesRef {
	return IndicatorField(id, "value")
}

func IndicatorField(id string, field string) SeriesRef {
	return SeriesRef{ref: ValueRef{Scope: ValueScopeIndicator, Name: id, Field: field}}
}

func Market(field string) SeriesRef {
	return SeriesRef{ref: ValueRef{Scope: ValueScopeMarket, Name: "bar", Field: field}}
}

func Position(field string) SeriesRef {
	return SeriesRef{ref: ValueRef{Scope: ValueScopePosition, Name: "current", Field: field}}
}

func PositionFlat() Expression {
	return Position("quantity").EQ(0)
}

func PositionLong() Expression {
	return Position("quantity").GT(0)
}

func PositionShort() Expression {
	return Position("quantity").LT(0)
}

func (r SeriesRef) Ref() Expression {
	ref := r.ref
	return Expression{Kind: ExprReference, Ref: &ref}
}

func (r SeriesRef) Offset(offset int) SeriesRef {
	r.ref.Offset = offset
	return r
}

func (r SeriesRef) Field(field string) SeriesRef {
	r.ref.Field = field
	return r
}

func (r SeriesRef) GT(value interface{}) Expression {
	return compare(OperatorGT, r.Ref(), value)
}

func (r SeriesRef) GTE(value interface{}) Expression {
	return compare(OperatorGTE, r.Ref(), value)
}

func (r SeriesRef) LT(value interface{}) Expression {
	return compare(OperatorLT, r.Ref(), value)
}

func (r SeriesRef) LTE(value interface{}) Expression {
	return compare(OperatorLTE, r.Ref(), value)
}

func (r SeriesRef) EQ(value interface{}) Expression {
	return compare(OperatorEQ, r.Ref(), value)
}

func (r SeriesRef) CrossUp(value interface{}) Expression {
	return compare(OperatorCrossUp, r.Ref(), value)
}

func (r SeriesRef) CrossDown(value interface{}) Expression {
	return compare(OperatorCrossDown, r.Ref(), value)
}

func (r SeriesRef) TurnsBullish() Expression {
	return Expression{
		Kind: ExprOperator,
		Op:   OperatorTurnBullish,
		Args: []Expression{r.Ref()},
	}
}

func (r SeriesRef) TurnsBearish() Expression {
	return Expression{
		Kind: ExprOperator,
		Op:   OperatorTurnBearish,
		Args: []Expression{r.Ref()},
	}
}

func And(args ...Expression) Expression {
	return Expression{Kind: ExprOperator, Op: OperatorAnd, Args: args}
}

func Or(args ...Expression) Expression {
	return Expression{Kind: ExprOperator, Op: OperatorOr, Args: args}
}

func Not(arg Expression) Expression {
	return Expression{Kind: ExprOperator, Op: OperatorNot, Args: []Expression{arg}}
}

func Number(value float64) Expression {
	return Expression{Kind: ExprNumber, Number: &value}
}

func Bool(value bool) Expression {
	return Expression{Kind: ExprBoolean, Bool: &value}
}

func compare(op Operator, left Expression, right interface{}) Expression {
	return Expression{Kind: ExprOperator, Op: op, Args: []Expression{left, operandExpression(right)}}
}

func operandExpression(value interface{}) Expression {
	switch typed := value.(type) {
	case Expression:
		return typed
	case SeriesRef:
		return typed.Ref()
	case int:
		return Number(float64(typed))
	case int64:
		return Number(float64(typed))
	case float64:
		return Number(typed)
	case float32:
		return Number(float64(typed))
	default:
		panic(fmt.Sprintf("unsupported expression operand %T", value))
	}
}
