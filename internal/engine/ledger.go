package engine

import (
	"fmt"
	"math"

	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type LedgerEvent struct {
	Sequence     int                 `json:"sequence"`
	Action       strategy.ActionType `json:"action"`
	Fill         Fill                `json:"fill"`
	Notional     float64             `json:"notional"`
	Cash         float64             `json:"cash"`
	PositionQty  float64             `json:"position_qty"`
	AveragePrice float64             `json:"average_price"`
}

// All cash, quantity, entry count and cost basis writes originate here.
func applyMatchedFill(p *portfolio, fill matchedFill, action strategy.ActionType, price float64) {
	if action != strategy.ActionClose {
		if p.marketType == strategy.MarketTypeSpot {
			p.cash -= fill.notional + fill.fee
		} else {
			p.cash -= fill.fee
		}
		if p.entryCount == 0 {
			p.firstNotional = fill.notional
			p.firstEntryPrice = price
			p.highestSinceEntry = price
			p.lowestSinceEntry = price
		}
		p.addQuantity(fill.quantity, price)
		p.entryCount++
		p.lastEntryPrice = price
		return
	}
	if p.marketType == strategy.MarketTypeSpot {
		p.cash += fill.notional - fill.fee
	} else {
		p.cash += fill.realized - fill.fee
	}
	p.quantity -= fill.quantity
	if math.Abs(p.quantity) < 1e-12 {
		p.resetPosition()
	}
}

type LedgerBalance struct {
	Cash         float64 `json:"cash"`
	PositionQty  float64 `json:"position_qty"`
	AveragePrice float64 `json:"average_price"`
}

// ReplayLedger rejects duplicate/out-of-order events and verifies accounting
// from fills rather than trusting the recorded balance snapshots.
func ReplayLedger(account strategy.AccountSpec, events []LedgerEvent) (LedgerBalance, error) {
	p := newPortfolio(account.MarketType, account.InitialCash)
	seen := make(map[string]bool)
	for index, event := range events {
		if event.Sequence != index+1 || event.Fill.OrderID == "" || seen[event.Fill.OrderID] {
			return LedgerBalance{}, fmt.Errorf("invalid ledger sequence or duplicate fill at %d", index+1)
		}
		seen[event.Fill.OrderID] = true
		fill := event.Fill
		if !isPositiveFinite(fill.Price) || !isPositiveFinite(event.Notional) ||
			!isPositiveFinite(math.Abs(fill.Quantity)) || math.IsNaN(fill.Fee) || math.IsInf(fill.Fee, 0) || fill.Fee < 0 {
			return LedgerBalance{}, fmt.Errorf("invalid ledger fill at %d", index+1)
		}
		side := strategy.TradeSideLong
		if fill.Quantity < 0 {
			side = strategy.TradeSideShort
		}
		if fill.Side != side || (p.marketType == strategy.MarketTypeSpot && side == strategy.TradeSideShort) {
			return LedgerBalance{}, fmt.Errorf("invalid ledger side at %d", index+1)
		}
		if !near(event.Notional, math.Abs(fill.Quantity)*fill.Price) {
			return LedgerBalance{}, fmt.Errorf("invalid ledger notional at %d", index+1)
		}
		switch event.Action {
		case strategy.ActionClose:
			if p.currentSide() != fill.Side || math.Abs(fill.Quantity) > math.Abs(p.quantity)+1e-12 {
				return LedgerBalance{}, fmt.Errorf("invalid ledger close at %d", index+1)
			}
			expected := (fill.Price - p.avgPrice) * fill.Quantity
			if !near(expected, fill.RealizedPNL) {
				return LedgerBalance{}, fmt.Errorf("invalid ledger realized PNL at %d", index+1)
			}
		case strategy.ActionEnter, strategy.ActionAdd, strategy.ActionOpen:
			if (p.quantity != 0 && p.currentSide() != fill.Side) || fill.RealizedPNL != 0 {
				return LedgerBalance{}, fmt.Errorf("invalid ledger entry at %d", index+1)
			}
		default:
			return LedgerBalance{}, fmt.Errorf("invalid ledger action at %d", index+1)
		}
		applyMatchedFill(&p, matchedFill{quantity: fill.Quantity, notional: event.Notional,
			fee: fill.Fee, realized: fill.RealizedPNL}, event.Action, fill.Price)
		if p.cash != event.Cash || p.quantity != event.PositionQty || p.avgPrice != event.AveragePrice {
			return LedgerBalance{}, fmt.Errorf("ledger balance mismatch at %d", index+1)
		}
	}
	return LedgerBalance{Cash: p.cash, PositionQty: p.quantity, AveragePrice: p.avgPrice}, nil
}

func near(a, b float64) bool {
	return !math.IsNaN(a) && !math.IsNaN(b) && !math.IsInf(a, 0) && !math.IsInf(b, 0) &&
		math.Abs(a-b) <= 1e-10*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}
