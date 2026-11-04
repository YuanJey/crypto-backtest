package engine

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type indicatorState struct {
	plan       compiler.IndicatorPlan
	aggregator barAggregator
	last       map[string]float64
}

func Run(ctx context.Context, plan compiler.Plan, request RunRequest) (Result, error) {
	reportSpec := normalizeReportSpec(request.Report)
	if err := validateReportSpec(reportSpec, plan); err != nil {
		return Result{}, err
	}
	bars := append([]market.Bar(nil), request.Bars...)
	sort.Slice(bars, func(i, j int) bool { return bars[i].OpenTimeMS < bars[j].OpenTimeMS })
	if len(bars) == 0 {
		return Result{}, fmt.Errorf("bars must not be empty")
	}
	for index, bar := range bars {
		if index > 0 && bars[index-1].OpenTimeMS == bar.OpenTimeMS {
			return Result{}, fmt.Errorf("duplicate bar at open_time_ms=%d", bar.OpenTimeMS)
		}
		if bar.OpenTimeMS%plan.BaseTimeframeMS != 0 {
			return Result{}, fmt.Errorf("bar at open_time_ms=%d is not aligned to base timeframe", bar.OpenTimeMS)
		}
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
	exitPolicies := newExitPolicyRuntimes(plan.ExitPolicies)
	result := Result{}
	pendingOrderIndexes := make([]int, 0)
	orderSequence := 0
	peakEquity := plan.Strategy.Account.InitialCash
	riskHalted := false
	lastFillBarIndex := -1

	for index, bar := range bars {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if !bar.IsClosed() {
			return Result{}, fmt.Errorf("bar at open_time_ms=%d is not closed", bar.OpenTimeMS)
		}
		riskHalted = riskHalted || drawdownHalted(
			portfolio.equity(bar.Open),
			peakEquity,
			plan.Strategy.Risk.MaxDrawdownHaltPct,
		)
		if len(pendingOrderIndexes) > 0 {
			for _, orderIndex := range pendingOrderIndexes {
				previousEntryCount := portfolio.entryCount
				hadPosition := portfolio.quantity != 0
				if fillOrder(&portfolio, &result, orderIndex, bar, plan, riskHalted) {
					lastFillBarIndex = index
					if portfolio.entryCount > previousEntryCount {
						snapshotExitPolicies(exitPolicies, states, previousEntryCount == 0)
					}
					if hadPosition && portfolio.quantity == 0 {
						resetExitPolicyRuntimes(exitPolicies)
					}
				}
			}
			pendingOrderIndexes = pendingOrderIndexes[:0]
		}

		exitHandled := false
		if candidate := selectExitCandidate(portfolio, bar, exitPolicies, states); candidate != nil {
			candidate.runtime.fired = true
			orderSequence++
			action := strategy.ActionSpec{
				Type:    strategy.ActionClose,
				Side:    candidate.runtime.spec.Side,
				SizePct: candidate.runtime.spec.ClosePct,
			}
			result.Orders = append(result.Orders, Order{
				ID:           fmt.Sprintf("order-%d", orderSequence),
				RuleID:       candidate.runtime.spec.ID,
				SignalTimeMS: bar.OpenTimeMS + plan.BaseTimeframeMS,
				Action:       action,
				Status:       OrderStatusPending,
			})
			orderIndex := len(result.Orders) - 1
			if candidate.runtime.spec.Execution == strategy.ExitExecutionNextOpen {
				pendingOrderIndexes = append(pendingOrderIndexes, orderIndex)
			} else {
				rawPrice := exitTriggerFillPrice(candidate.runtime.spec, candidate.target, bar)
				price := fillPrice(rawPrice, action, plan.Strategy.Execution.Slippage)
				quantity, fee, realized, ok := portfolio.close(
					action.Side,
					price,
					action.SizePct,
					plan.Strategy.Execution.Fee.TakerRate,
				)
				if ok {
					result.Orders[orderIndex].Status = OrderStatusFilled
					result.Fills = append(result.Fills, Fill{
						OrderID:     result.Orders[orderIndex].ID,
						RuleID:      candidate.runtime.spec.ID,
						TimeMS:      bar.OpenTimeMS + plan.BaseTimeframeMS,
						Price:       price,
						Quantity:    quantity,
						Fee:         fee,
						RealizedPNL: realized,
						Side:        action.Side,
					})
					lastFillBarIndex = index
					if portfolio.quantity == 0 {
						resetExitPolicyRuntimes(exitPolicies)
					}
				} else {
					result.Orders[orderIndex].Status = OrderStatusSkipped
				}
			}
			exitHandled = true
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
		portfolio.updateExtremes(bar.High, bar.Low)

		currentEquity := portfolio.equity(bar.Close)
		if currentEquity > peakEquity {
			peakEquity = currentEquity
		}
		riskHalted = riskHalted || drawdownHalted(
			currentEquity,
			peakEquity,
			plan.Strategy.Risk.MaxDrawdownHaltPct,
		)

		evalCtx := evalContext{bars: bars, barIndex: index, indicators: indicatorSeries, portfolio: portfolio}
		cooldownActive := lastFillBarIndex >= 0 &&
			index-lastFillBarIndex < plan.Strategy.Behavior.CooldownBars
		for _, rule := range plan.Rules {
			if exitHandled {
				break
			}
			matched, err := evalBool(rule.When, evalCtx)
			if err != nil {
				return Result{}, fmt.Errorf("rule %q: %w", rule.ID, err)
			}
			if !matched || !eligible(portfolio, rule.Action, plan.Strategy, riskHalted, cooldownActive) {
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
			TimeMS:           bar.OpenTimeMS + plan.BaseTimeframeMS,
			Equity:           currentEquity,
			Cash:             portfolio.cash,
			PositionQty:      portfolio.quantity,
			PositionLeverage: portfolio.positionLeverage(bar.Close),
			Close:            bar.Close,
		})
	}

	if len(result.EquityCurve) > 0 {
		result.FinalEquity = result.EquityCurve[len(result.EquityCurve)-1].Equity
	}
	result.TotalOrders = len(result.Orders)
	result.TotalFills = len(result.Fills)
	if err := applyReportSpec(&result, reportSpec, indicatorSeries, plan.Strategy.Account.InitialCash); err != nil {
		return Result{}, err
	}
	return result, nil
}

func fillOrder(portfolio *portfolio, result *Result, orderIndex int, bar market.Bar, plan compiler.Plan, riskHalted bool) bool {
	order := &result.Orders[orderIndex]
	price := fillPrice(bar.Open, order.Action, plan.Strategy.Execution.Slippage)
	feeRate := plan.Strategy.Execution.Fee.TakerRate
	var quantity, fee, realized float64
	var ok bool
	switch order.Action.Type {
	case strategy.ActionEnter, strategy.ActionAdd, strategy.ActionOpen:
		if !eligible(*portfolio, order.Action, plan.Strategy, riskHalted, false) {
			break
		}
		notionalPct, sizeOK := resolveOpenNotionalPct(
			*portfolio,
			order.Action,
			plan.Strategy.Position,
			plan.Strategy.Account.InitialCash,
			plan.Strategy.Account.ExchangeLeverage,
			price,
		)
		if !sizeOK {
			ok = false
			break
		}
		quantity, fee, ok = portfolio.open(
			order.Action.Side,
			price,
			notionalPct,
			feeRate,
			additionsAllowed(plan.Strategy, order.Action.Type),
		)
	case strategy.ActionClose:
		sizePct := order.Action.SizePct
		if sizePct == 0 {
			sizePct = 1
		}
		quantity, fee, realized, ok = portfolio.close(order.Action.Side, price, sizePct, feeRate)
	}
	if !ok {
		order.Status = OrderStatusSkipped
		return false
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
	return true
}

func fillPrice(open float64, action strategy.ActionSpec, slippage strategy.SlippageSpec) float64 {
	if slippage.Type == strategy.SlippageTypeNone || slippage.Value == 0 {
		return open
	}
	rate := slippage.Value / 10_000
	switch action.Type {
	case strategy.ActionEnter, strategy.ActionAdd, strategy.ActionOpen:
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

func eligible(
	portfolio portfolio,
	action strategy.ActionSpec,
	strategyDef strategy.Definition,
	riskHalted bool,
	cooldownActive bool,
) bool {
	currentSide := portfolio.currentSide()
	switch action.Type {
	case strategy.ActionEnter:
		return !riskHalted && !cooldownActive && currentSide == ""
	case strategy.ActionAdd:
		return !riskHalted && !cooldownActive &&
			additionsAllowed(strategyDef, action.Type) &&
			currentSide == action.Side && portfolio.entryCount == action.AdditionIndex
	case strategy.ActionOpen:
		if riskHalted || cooldownActive {
			return false
		}
		if currentSide == "" {
			return true
		}
		if currentSide != action.Side || !additionsAllowed(strategyDef, action.Type) {
			return false
		}
		maxEntries := 1 + strategyDef.Position.MaxAdditions
		return portfolio.entryCount < maxEntries
	case strategy.ActionClose:
		return currentSide == action.Side
	default:
		return false
	}
}

func additionsAllowed(def strategy.Definition, actionType strategy.ActionType) bool {
	if def.Behavior.AllowAdditions != nil {
		return *def.Behavior.AllowAdditions
	}
	return actionType == strategy.ActionAdd || def.Behavior.AllowPyramiding
}

func resolveOpenNotionalPct(
	portfolio portfolio,
	action strategy.ActionSpec,
	position strategy.PositionSpec,
	initialCash float64,
	exchangeLeverage float64,
	price float64,
) (float64, bool) {
	equity := portfolio.equity(price)
	if equity <= 0 {
		return 0, false
	}

	var notionalPct float64
	if action.Type == strategy.ActionEnter {
		notionalPct = action.SizePct * position.PositionLeverage
	} else if action.Type == strategy.ActionAdd {
		switch action.EffectiveSizingMode() {
		case strategy.AdditionSizeFirstMultiple:
			notionalPct = portfolio.firstNotional * action.SizeMultiplier / equity
		case strategy.AdditionSizeCurrentMultiple:
			notionalPct = portfolio.positionNotional(price) * action.SizeMultiplier / equity
		case strategy.AdditionSizeFirstGeometric:
			notionalPct = portfolio.firstNotional *
				math.Pow(action.SizeMultiplier, float64(action.AdditionIndex)) / equity
		case strategy.AdditionSizeEquityPct:
			notionalPct = action.SizePct
		default:
			return 0, false
		}
	} else if portfolio.entryCount == 0 {
		allocationPct := action.SizePct
		if allocationPct == 0 {
			allocationPct = position.InitialSizePct
		}
		notionalPct = allocationPct * position.PositionLeverage
	} else {
		switch {
		case action.SizePct > 0:
			notionalPct = action.SizePct
		case position.AddSizePct > 0:
			notionalPct = position.AddSizePct
		case position.AddSizeMultiplier > 0:
			notionalPct = portfolio.firstNotional *
				math.Pow(position.AddSizeMultiplier, float64(portfolio.entryCount)) /
				equity
		default:
			notionalPct = portfolio.firstNotional / equity
		}
	}
	if portfolio.entryCount == 0 {
		// Keep the same safety cap as the martingale position sizing model:
		// a single initial entry cannot exceed the current account equity.
		if notionalPct > 1 {
			notionalPct = 1
		}
		decayPower := position.LongCapitalDecayPower
		if action.Side == strategy.TradeSideShort {
			decayPower = position.ShortCapitalDecayPower
		}
		notionalPct *= capitalDecayFactor(initialCash, equity, decayPower)
	}
	if notionalPct <= 0 || math.IsNaN(notionalPct) || math.IsInf(notionalPct, 0) {
		return 0, false
	}

	availableNotional := equity*exchangeLeverage - portfolio.positionNotional(price)
	if availableNotional <= 0 {
		return 0, false
	}
	requestedNotional := equity * notionalPct
	if requestedNotional > availableNotional {
		notionalPct = availableNotional / equity
	}
	return notionalPct, notionalPct > 0
}

func capitalDecayFactor(initialEquity, currentEquity, power float64) float64 {
	if initialEquity <= 0 || currentEquity <= 0 {
		return 1
	}
	if power <= 0 {
		power = strategy.DefaultCapitalDecayPower
	}
	return math.Min(1, math.Pow(initialEquity/currentEquity, power))
}

func drawdownHalted(equity, peak, limit float64) bool {
	if limit <= 0 || peak <= 0 {
		return false
	}
	return (peak-equity)/peak >= limit
}
