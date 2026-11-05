package engine

import (
	"math"

	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

// matchedFill is the broker's immutable output. It retains the exact
// notional used for fees and accounting (quantity * price can round differently).
type matchedFill struct {
	quantity float64
	notional float64
	fee      float64
	realized float64
}

// Quoting never mutates the portfolio. Both next-open and intrabar exits
// use these functions and commit through the same ledger.
func quoteOpen(p portfolio, side strategy.TradeSide, price, notionalPct, feeRate float64, allowPyramiding bool) (matchedFill, bool) {
	if !isPositiveFinite(p.cash) || !isPositiveFinite(price) {
		return matchedFill{}, false
	}
	if p.quantity != 0 && (p.currentSide() != side || !allowPyramiding) {
		return matchedFill{}, false
	}
	notional := p.equity(price) * notionalPct
	if p.marketType == strategy.MarketTypeSpot {
		notional = math.Min(notional, p.cash/(1+feeRate))
	}
	if !isPositiveFinite(notional) {
		return matchedFill{}, false
	}
	quantity := notional / price
	if side == strategy.TradeSideShort {
		quantity = -quantity
	}
	return matchedFill{quantity: quantity, notional: notional, fee: notional * feeRate}, true
}

func quoteClose(p portfolio, side strategy.TradeSide, price, sizePct, feeRate float64) (matchedFill, bool) {
	if p.quantity == 0 || p.currentSide() != side || !isPositiveFinite(price) {
		return matchedFill{}, false
	}
	closeAbs := math.Abs(p.quantity) * sizePct
	if !isPositiveFinite(closeAbs) || sizePct > 1 {
		return matchedFill{}, false
	}
	notional := closeAbs * price
	match := matchedFill{quantity: closeAbs, notional: notional, fee: notional * feeRate}
	if p.quantity > 0 {
		match.realized = (price - p.avgPrice) * closeAbs
	} else {
		match.realized = (p.avgPrice - price) * closeAbs
		match.quantity = -closeAbs
	}
	return match, true
}
