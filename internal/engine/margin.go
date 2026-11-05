package engine

import (
	"fmt"
	"math"

	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

// This fixed-rate linear cross-margin model uses OHLC as a mark-price proxy.
// It does not model exchange tiers, maintenance deductions or liquidation fees.
type MarginReport struct {
	Model            string  `json:"model"`
	MMRDefinition    string  `json:"mmr_definition"`
	MaintenanceRate  float64 `json:"maintenance_margin_rate"`
	Liquidated       bool    `json:"liquidated"`
	LiquidationCount int     `json:"liquidation_count"`
	// MMR is equity / maintenance margin. A value of 1 means 100%.
	MinimumMMR *float64 `json:"minimum_mmr"`
	// Deprecated E/N metric retained for existing report consumers.
	MinimumRatio *float64           `json:"minimum_margin_ratio"`
	Liquidations []LiquidationEvent `json:"liquidations"`
}

type LiquidationEvent struct {
	OrderID           string             `json:"order_id"`
	BarOpenTimeMS     int64              `json:"bar_open_time_ms"`
	TimeMS            int64              `json:"time_ms"`
	Phase             string             `json:"phase"`
	Side              strategy.TradeSide `json:"side"`
	Quantity          float64            `json:"quantity"`
	TriggerPrice      float64            `json:"trigger_price"`
	FillPrice         float64            `json:"fill_price"`
	EquityBefore      float64            `json:"equity_before"`
	MaintenanceMargin float64            `json:"maintenance_margin"`
	MMR               *float64           `json:"mmr"`
	// Deprecated E/N value; not the account MMR.
	MarginRatio float64 `json:"margin_ratio"`
	CashAfter   float64 `json:"cash_after"`
	Deficit     float64 `json:"deficit,omitempty"`
}

func newMarginReport(account strategy.AccountSpec) *MarginReport {
	if account.MaintenanceMarginRate == 0 {
		return nil
	}
	return &MarginReport{Model: "linear_cross_fixed_mmr_ohlc_adverse_first",
		MMRDefinition:   "equity_over_maintenance_margin",
		MaintenanceRate: account.MaintenanceMarginRate, Liquidations: []LiquidationEvent{}}
}

func positionMMR(p portfolio, price, rate float64) *float64 {
	required := p.positionNotional(price) * rate
	if p.quantity == 0 || !isPositiveFinite(required) {
		return nil
	}
	mmr := p.equity(price) / required
	if !isUsableExitNumber(mmr) {
		return nil
	}
	return &mmr
}

func (m *MarginReport) observe(p portfolio, price float64) {
	if m == nil || p.quantity == 0 {
		return
	}
	if mmr := positionMMR(p, price, m.MaintenanceRate); mmr != nil &&
		(m.MinimumMMR == nil || *mmr < *m.MinimumMMR) {
		m.MinimumMMR = mmr
	}
	ratio := p.equity(price) / p.positionNotional(price)
	if isUsableExitNumber(ratio) && (m.MinimumRatio == nil || ratio < *m.MinimumRatio) {
		value := ratio
		m.MinimumRatio = &value
	}
}

func marginBreached(p portfolio, price, rate float64) bool {
	if rate == 0 || p.quantity == 0 {
		return false
	}
	equity, required := p.equity(price), p.positionNotional(price)*rate
	// Treat equality consistently despite floating-point cancellation.
	return equity <= required || near(equity, required)
}

func liquidationPrice(p portfolio, rate float64) *float64 {
	if rate == 0 || p.quantity == 0 {
		return nil
	}
	q := math.Abs(p.quantity)
	var price float64
	if p.quantity > 0 {
		price = (p.avgPrice - p.cash/q) / (1 - rate)
	} else {
		price = (p.avgPrice + p.cash/q) / (1 + rate)
	}
	if !isPositiveFinite(price) {
		return nil
	}
	return &price
}

// Open gaps and fee-induced breaches are executable at the observed open,
// never at an unreachable theoretical threshold from a preceding bar.
func forceLiquidation(p *portfolio, result *Result, sequence *int, rawPrice float64,
	timeMS, barOpenMS int64, phase string, logic strategy.LogicDefinition, config strategy.RunConfig) error {
	m := result.Margin
	m.observe(*p, rawPrice)
	event := LiquidationEvent{BarOpenTimeMS: barOpenMS, TimeMS: timeMS, Phase: phase,
		Side: p.currentSide(), Quantity: p.quantity, TriggerPrice: rawPrice,
		EquityBefore: p.equity(rawPrice), MaintenanceMargin: p.positionNotional(rawPrice) * m.MaintenanceRate,
		MMR:         positionMMR(*p, rawPrice, m.MaintenanceRate),
		MarginRatio: p.equity(rawPrice) / p.positionNotional(rawPrice)}
	*sequence++
	result.Orders = append(result.Orders, Order{ID: fmt.Sprintf("order-%d", *sequence),
		RuleID: "__liquidation__", SignalTimeMS: timeMS, Reason: "liquidation",
		Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: event.Side, SizePct: 1},
		Status: OrderStatusPending})
	if !executeOrder(p, result, len(result.Orders)-1, rawPrice, timeMS, logic, config, true) {
		return fmt.Errorf("liquidation fill failed")
	}
	fill := result.Fills[len(result.Fills)-1]
	event.OrderID, event.FillPrice, event.CashAfter = fill.OrderID, fill.Price, p.cash
	event.Deficit = math.Max(0, -p.cash)
	m.Liquidated, m.LiquidationCount = true, m.LiquidationCount+1
	m.Liquidations = append(m.Liquidations, event)
	return nil
}

// Follow the adverse excursion first. A previously active intrabar stop
// encountered before the liquidation boundary can close/reduce the position.
// Favorable take profit and close/next-open stops cannot rescue an OHLC breach
// whose intrabar ordering is unknown.
func processMarginIntrabar(p *portfolio, result *Result, sequence *int, bar market.Bar,
	timeMS int64, exits []exitPolicyRuntime, states []indicatorState,
	logic strategy.LogicDefinition, config strategy.RunConfig) (bool, error) {
	m := result.Margin
	if m == nil || m.Liquidated || p.quantity == 0 {
		return false, nil
	}
	adverse := bar.Low
	if p.quantity < 0 {
		adverse = bar.High
	}
	cursor := bar.Open
	handled := false
	for p.quantity != 0 {
		if marginBreached(*p, cursor, m.MaintenanceRate) {
			return true, forceLiquidation(p, result, sequence, cursor, timeMS, bar.OpenTimeMS,
				"intrabar", logic, config)
		}
		breached := marginBreached(*p, adverse, m.MaintenanceRate)
		threshold := liquidationPrice(*p, m.MaintenanceRate)
		var stop *exitCandidate
		for i := range exits {
			runtime := &exits[i]
			spec := runtime.spec
			if runtime.fired || spec.Side != p.currentSide() || spec.Purpose != strategy.ExitPurposeStopLoss ||
				spec.Execution != strategy.ExitExecutionTriggerPrice ||
				(p.quantity > 0 && spec.TriggerPrice != strategy.ExitTriggerLow) ||
				(p.quantity < 0 && spec.TriggerPrice != strategy.ExitTriggerHigh) {
				continue
			}
			target, ok := exitTarget(*p, *runtime, states)
			if !ok || !exitPolicyHit(spec, target, bar) {
				continue
			}
			target = exitTriggerFillPrice(spec, target, bar)
			earlier := p.quantity > 0 && target <= cursor && (!breached || threshold != nil && target > *threshold) ||
				p.quantity < 0 && target >= cursor && (!breached || threshold != nil && target < *threshold)
			closer := stop == nil || p.quantity > 0 && target > stop.target || p.quantity < 0 && target < stop.target
			if earlier && closer {
				stop = &exitCandidate{runtime: runtime, target: target}
			}
		}
		if stop == nil {
			if !breached {
				m.observe(*p, adverse)
				return handled, nil
			}
			rawPrice := adverse
			if threshold != nil {
				rawPrice = *threshold
			}
			if marginBreached(*p, cursor, m.MaintenanceRate) {
				rawPrice = cursor
			}
			return true, forceLiquidation(p, result, sequence, rawPrice, timeMS, bar.OpenTimeMS,
				"intrabar", logic, config)
		}
		m.observe(*p, stop.target)
		handled = true
		stop.runtime.fired = true
		*sequence++
		result.Orders = append(result.Orders, Order{ID: fmt.Sprintf("order-%d", *sequence),
			RuleID: stop.runtime.spec.ID, SignalTimeMS: timeMS, Status: OrderStatusPending,
			Action: strategy.ActionSpec{Type: strategy.ActionClose, Side: p.currentSide(), SizePct: stop.runtime.spec.ClosePct}})
		if !executeOrder(p, result, len(result.Orders)-1, stop.target, timeMS, logic, config, false) {
			return true, fmt.Errorf("protective stop fill failed during margin evaluation")
		}
		cursor = stop.target
	}
	m.observe(*p, adverse)
	return handled, nil
}
