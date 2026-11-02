package engine

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/bytedance/crypto-backtest/internal/compiler"
	"github.com/bytedance/crypto-backtest/internal/market"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

type indicatorState struct {
	plan       compiler.IndicatorPlan
	aggregator barAggregator
	last       map[string]float64
}

func Run(ctx context.Context, plan compiler.Plan, request RunRequest) (Result, error) {
	reportSpec := normalizeReportSpec(request.Report)
	bars := append([]market.Bar(nil), request.Bars...)
	sort.Slice(bars, func(i, j int) bool { return bars[i].OpenTimeMS < bars[j].OpenTimeMS })
	if len(bars) == 0 {
		return Result{}, fmt.Errorf("bars must not be empty")
	}

	states := make([]indicatorState, 0, len(plan.Indicators))
	indicatorSeries := map[string]map[string][]float64{}
	for _, indicatorPlan := range plan.Indicators {
		indicatorPlan.Runtime.Reset()
		last := map[string]float64{}
		indicatorSeries[indicatorPlan.ID] = map[string][]float64{}
		for _, output := range indicatorPlan.Outputs {
			last[output] = math.NaN()
			indicatorSeries[indicatorPlan.ID][output] = make([]float64, 0, len(bars))
		}
		states = append(states, indicatorState{
			plan:       indicatorPlan,
			aggregator: newBarAggregator(indicatorPlan.TimeframeMS, plan.BaseTimeframeMS),
			last:       last,
		})
	}

	portfolio := newPortfolio(plan.Strategy.Account.MarketType, plan.Strategy.Account.InitialCash)
	result := Result{}
	pendingOrderIndexes := make([]int, 0)
	orderSequence := 0

	for index, bar := range bars {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if !bar.IsClosed() {
			return Result{}, fmt.Errorf("bar at open_time_ms=%d is not closed", bar.OpenTimeMS)
		}
		if len(pendingOrderIndexes) > 0 {
			for _, orderIndex := range pendingOrderIndexes {
				fillOrder(&portfolio, &result, orderIndex, bar, plan)
			}
			pendingOrderIndexes = pendingOrderIndexes[:0]
		}

		for stateIndex := range states {
			state := &states[stateIndex]
			completedBar, completed := state.aggregator.Update(bar)
			if completed {
				inputs := make([]float64, 0, len(state.plan.Inputs))
				evalCtx := evalContext{bars: bars, barIndex: index, indicators: indicatorSeries, portfolio: portfolio}
				for _, input := range state.plan.Inputs {
					if input.Scope == strategy.ValueScopeMarket {
						value, _, err := marketField(completedBar, input.Field)
						if err != nil {
							return Result{}, err
						}
						inputs = append(inputs, value)
						continue
					}
					value, ok, err := evalRef(input, evalCtx, 0)
					if err != nil {
						return Result{}, err
					}
					if !ok {
						value = math.NaN()
					}
					inputs = append(inputs, value)
				}
				outputValues := state.plan.Runtime.Update(inputs)
				for outputIndex, outputName := range state.plan.Outputs {
					value := math.NaN()
					if outputIndex < len(outputValues) {
						value = outputValues[outputIndex]
					}
					state.last[outputName] = value
				}
			}
			for _, outputName := range state.plan.Outputs {
				indicatorSeries[state.plan.ID][outputName] = append(indicatorSeries[state.plan.ID][outputName], state.last[outputName])
			}
		}

		evalCtx := evalContext{bars: bars, barIndex: index, indicators: indicatorSeries, portfolio: portfolio}
		for _, rule := range plan.Rules {
			matched, err := evalBool(rule.When, evalCtx)
			if err != nil {
				return Result{}, fmt.Errorf("rule %q: %w", rule.ID, err)
			}
			if !matched || !eligible(portfolio, rule.Action, plan.Strategy) {
				continue
			}
			orderSequence++
			result.Orders = append(result.Orders, Order{
				ID:           fmt.Sprintf("order-%d", orderSequence),
				RuleID:       rule.ID,
				SignalTimeMS: bar.OpenTimeMS + plan.BaseTimeframeMS,
				Action:       rule.Action,
				Status:       OrderStatusPending,
			})
			pendingOrderIndexes = append(pendingOrderIndexes, len(result.Orders)-1)
			break
		}

		result.EquityCurve = append(result.EquityCurve, EquityPoint{
			TimeMS:      bar.OpenTimeMS + plan.BaseTimeframeMS,
			Equity:      portfolio.equity(bar.Close),
			Cash:        portfolio.cash,
			PositionQty: portfolio.quantity,
			Close:       bar.Close,
		})
	}

	if len(result.EquityCurve) > 0 {
		result.FinalEquity = result.EquityCurve[len(result.EquityCurve)-1].Equity
	}
	if err := applyReportSpec(&result, reportSpec, indicatorSeries, plan.Strategy.Account.InitialCash); err != nil {
		return Result{}, err
	}
	return result, nil
}

func fillOrder(portfolio *portfolio, result *Result, orderIndex int, bar market.Bar, plan compiler.Plan) {
	order := &result.Orders[orderIndex]
	price := fillPrice(bar.Open, order.Action, plan.Strategy.Execution.Slippage)
	feeRate := plan.Strategy.Execution.Fee.TakerRate
	var quantity, fee, realized float64
	var ok bool
	switch order.Action.Type {
	case strategy.ActionOpen:
		sizePct, sizeOK := resolveOpenSizePct(*portfolio, order.Action, plan.Strategy.Position, price)
		if !sizeOK {
			ok = false
			break
		}
		quantity, fee, ok = portfolio.open(order.Action.Side, price, sizePct, feeRate, allowPyramiding(plan.Strategy))
	case strategy.ActionClose:
		sizePct := order.Action.SizePct
		if sizePct == 0 {
			sizePct = 1
		}
		quantity, fee, realized, ok = portfolio.close(order.Action.Side, price, sizePct, feeRate)
	}
	if !ok {
		order.Status = OrderStatusSkipped
		return
	}
	order.Status = OrderStatusFilled
	result.Fills = append(result.Fills, Fill{
		OrderID:     order.ID,
		RuleID:      order.RuleID,
		TimeMS:      bar.OpenTimeMS,
		Price:       price,
		Quantity:    quantity,
		Fee:         fee,
		RealizedPNL: realized,
		Side:        order.Action.Side,
	})
}

func fillPrice(open float64, action strategy.ActionSpec, slippage strategy.SlippageSpec) float64 {
	if slippage.Type == strategy.SlippageTypeNone || slippage.Value == 0 {
		return open
	}
	rate := slippage.Value / 10_000
	switch action.Type {
	case strategy.ActionOpen:
		if action.Side == strategy.TradeSideLong {
			return open * (1 + rate)
		}
		return open * (1 - rate)
	case strategy.ActionClose:
		if action.Side == strategy.TradeSideLong {
			return open * (1 - rate)
		}
		return open * (1 + rate)
	default:
		return open
	}
}

func eligible(portfolio portfolio, action strategy.ActionSpec, strategyDef strategy.Definition) bool {
	currentSide := portfolio.currentSide()
	switch action.Type {
	case strategy.ActionOpen:
		if currentSide == "" {
			return true
		}
		if currentSide != action.Side || !allowPyramiding(strategyDef) {
			return false
		}
		maxEntries := 1 + strategyDef.Position.MaxAdditions
		return maxEntries <= 1 || portfolio.entryCount < maxEntries
	case strategy.ActionClose:
		return currentSide == action.Side
	default:
		return false
	}
}

func allowPyramiding(strategyDef strategy.Definition) bool {
	return strategyDef.Position.AllowPyramiding || strategyDef.Behavior.AllowPyramiding
}

func resolveOpenSizePct(portfolio portfolio, action strategy.ActionSpec, position strategy.PositionSpec, price float64) (float64, bool) {
	sizePct := action.SizePct
	if sizePct == 0 {
		if portfolio.entryCount == 0 {
			sizePct = position.InitialSizePct
		} else if position.AddSizePct > 0 {
			sizePct = position.AddSizePct
		} else if position.AddSizeMultiplier > 0 {
			sizePct = position.InitialSizePct * math.Pow(position.AddSizeMultiplier, float64(portfolio.entryCount))
		} else {
			sizePct = position.InitialSizePct
		}
	}
	if sizePct <= 0 {
		return 0, false
	}
	if position.MaxPositionPct > 0 {
		equity := portfolio.equity(price)
		if equity <= 0 {
			return 0, false
		}
		currentNotional := portfolio.positionNotional(price)
		maxNotional := equity * position.MaxPositionPct
		availableNotional := maxNotional - currentNotional
		if availableNotional <= 0 {
			return 0, false
		}
		requestedNotional := equity * sizePct
		if requestedNotional > availableNotional {
			sizePct = availableNotional / equity
		}
	}
	return sizePct, sizePct > 0
}
