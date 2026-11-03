package engine

import (
	"fmt"
	"math"

	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type evalContext struct {
	bars       []market.Bar
	barIndex   int
	indicators map[string]map[string][]float64
	portfolio  portfolio
}

func evalBool(expression strategy.Expression, ctx evalContext) (bool, error) {
	switch expression.Kind {
	case strategy.ExprBoolean:
		if expression.Bool == nil {
			return false, fmt.Errorf("missing bool value")
		}
		return *expression.Bool, nil
	case strategy.ExprOperator:
		return evalOperator(expression, ctx)
	default:
		return false, fmt.Errorf("expression kind %q is not boolean", expression.Kind)
	}
}

func evalOperator(expression strategy.Expression, ctx evalContext) (bool, error) {
	switch expression.Op {
	case strategy.OperatorAnd:
		for _, arg := range expression.Args {
			value, err := evalBool(arg, ctx)
			if err != nil || !value {
				return value, err
			}
		}
		return true, nil
	case strategy.OperatorOr:
		for _, arg := range expression.Args {
			value, err := evalBool(arg, ctx)
			if err != nil {
				return false, err
			}
			if value {
				return true, nil
			}
		}
		return false, nil
	case strategy.OperatorNot:
		value, err := evalBool(expression.Args[0], ctx)
		return !value, err
	case strategy.OperatorGT, strategy.OperatorGTE, strategy.OperatorLT, strategy.OperatorLTE, strategy.OperatorEQ:
		left, right, ok, err := evalBinaryNumbers(expression, ctx, 0)
		if err != nil || !ok {
			return false, err
		}
		switch expression.Op {
		case strategy.OperatorGT:
			return left > right, nil
		case strategy.OperatorGTE:
			return left >= right, nil
		case strategy.OperatorLT:
			return left < right, nil
		case strategy.OperatorLTE:
			return left <= right, nil
		case strategy.OperatorEQ:
			return left == right, nil
		}
	case strategy.OperatorCrossUp, strategy.OperatorCrossDown:
		prevLeft, prevRight, ok, err := evalBinaryNumbers(expression, ctx, -1)
		if err != nil || !ok {
			return false, err
		}
		curLeft, curRight, ok, err := evalBinaryNumbers(expression, ctx, 0)
		if err != nil || !ok {
			return false, err
		}
		if expression.Op == strategy.OperatorCrossUp {
			return prevLeft <= prevRight && curLeft > curRight, nil
		}
		return prevLeft >= prevRight && curLeft < curRight, nil
	}
	return false, fmt.Errorf("unsupported operator %q", expression.Op)
}

func evalBinaryNumbers(expression strategy.Expression, ctx evalContext, offsetDelta int) (float64, float64, bool, error) {
	left, ok, err := evalNumber(expression.Args[0], ctx, offsetDelta)
	if err != nil || !ok {
		return 0, 0, ok, err
	}
	right, ok, err := evalNumber(expression.Args[1], ctx, offsetDelta)
	if err != nil || !ok {
		return 0, 0, ok, err
	}
	return left, right, true, nil
}

func evalNumber(expression strategy.Expression, ctx evalContext, offsetDelta int) (float64, bool, error) {
	switch expression.Kind {
	case strategy.ExprNumber:
		if expression.Number == nil {
			return 0, false, fmt.Errorf("missing number value")
		}
		return *expression.Number, true, nil
	case strategy.ExprReference:
		if expression.Ref == nil {
			return 0, false, fmt.Errorf("missing ref value")
		}
		value, ok, err := evalRef(*expression.Ref, ctx, offsetDelta)
		if err != nil || !ok || math.IsNaN(value) {
			return value, false, err
		}
		return value, true, nil
	default:
		return 0, false, fmt.Errorf("expression kind %q is not numeric", expression.Kind)
	}
}

func evalRef(ref strategy.ValueRef, ctx evalContext, offsetDelta int) (float64, bool, error) {
	index := ctx.barIndex + ref.Offset + offsetDelta
	if index < 0 || index >= len(ctx.bars) {
		return math.NaN(), false, nil
	}
	switch ref.Scope {
	case strategy.ValueScopeMarket:
		return marketField(ctx.bars[index], ref.Field)
	case strategy.ValueScopeIndicator:
		fields, ok := ctx.indicators[ref.Name]
		if !ok {
			return 0, false, fmt.Errorf("unknown indicator %q", ref.Name)
		}
		series, ok := fields[ref.Field]
		if !ok {
			return 0, false, fmt.Errorf("indicator %q has no output %q", ref.Name, ref.Field)
		}
		if index >= len(series) {
			return math.NaN(), false, nil
		}
		return series[index], true, nil
	case strategy.ValueScopePosition:
		return positionField(ctx.portfolio, ref.Field, ctx.bars[index].Close)
	default:
		return 0, false, fmt.Errorf("unknown ref scope %q", ref.Scope)
	}
}

func marketField(bar market.Bar, field string) (float64, bool, error) {
	switch field {
	case "open":
		return bar.Open, true, nil
	case "high":
		return bar.High, true, nil
	case "low":
		return bar.Low, true, nil
	case "close":
		return bar.Close, true, nil
	case "volume":
		return bar.Volume, true, nil
	case "quote_volume":
		return bar.QuoteVolume, true, nil
	case "trade_count":
		return float64(bar.TradeCount), true, nil
	default:
		return 0, false, fmt.Errorf("unknown market field %q", field)
	}
}

func positionField(portfolio portfolio, field string, markPrice float64) (float64, bool, error) {
	switch field {
	case "quantity":
		return portfolio.quantity, true, nil
	case "abs_quantity":
		return math.Abs(portfolio.quantity), true, nil
	case "entry_price", "avg_entry_price":
		return portfolio.avgPrice, true, nil
	case "last_entry_price":
		return portfolio.lastEntryPrice, true, nil
	case "entry_count":
		return float64(portfolio.entryCount), true, nil
	case "unrealized_pnl_pct":
		if portfolio.quantity == 0 || portfolio.avgPrice == 0 {
			return 0, true, nil
		}
		if portfolio.quantity > 0 {
			return (markPrice - portfolio.avgPrice) / portfolio.avgPrice, true, nil
		}
		return (portfolio.avgPrice - markPrice) / portfolio.avgPrice, true, nil
	case "side":
		switch portfolio.currentSide() {
		case strategy.TradeSideLong:
			return 1, true, nil
		case strategy.TradeSideShort:
			return -1, true, nil
		default:
			return 0, true, nil
		}
	default:
		return 0, false, fmt.Errorf("unknown position field %q", field)
	}
}
