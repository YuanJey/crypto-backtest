package engine

import (
	"math"

	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type portfolio struct {
	marketType        strategy.MarketType
	cash              float64
	quantity          float64
	avgPrice          float64
	entryCount        int
	lastEntryPrice    float64
	firstNotional     float64
	firstEntryPrice   float64
	highestSinceEntry float64
	lowestSinceEntry  float64
}

func newPortfolio(marketType strategy.MarketType, initialCash float64) portfolio {
	return portfolio{marketType: marketType, cash: initialCash}
}

func (p portfolio) equity(markPrice float64) float64 {
	if p.marketType == strategy.MarketTypeSpot {
		return p.cash + math.Max(p.quantity, 0)*markPrice
	}
	if p.quantity > 0 {
		return p.cash + (markPrice-p.avgPrice)*p.quantity
	}
	if p.quantity < 0 {
		return p.cash + (p.avgPrice-markPrice)*math.Abs(p.quantity)
	}
	return p.cash
}

func (p portfolio) currentSide() strategy.TradeSide {
	if p.quantity > 0 {
		return strategy.TradeSideLong
	}
	if p.quantity < 0 {
		return strategy.TradeSideShort
	}
	return ""
}

func (p *portfolio) open(side strategy.TradeSide, price float64, notionalPct float64, feeRate float64, allowPyramiding bool) (float64, float64, bool) {
	match, ok := quoteOpen(*p, side, price, notionalPct, feeRate, allowPyramiding)
	if ok {
		applyMatchedFill(p, match, strategy.ActionOpen, price)
	}
	return match.quantity, match.fee, ok
}

func isPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (p *portfolio) updateExtremes(high, low float64) {
	if p.quantity == 0 {
		return
	}
	if p.highestSinceEntry == 0 || high > p.highestSinceEntry {
		p.highestSinceEntry = high
	}
	if p.lowestSinceEntry == 0 || low < p.lowestSinceEntry {
		p.lowestSinceEntry = low
	}
}

func (p portfolio) positionNotional(markPrice float64) float64 {
	return math.Abs(p.quantity) * markPrice
}

func (p portfolio) positionLeverage(markPrice float64) float64 {
	equity := p.equity(markPrice)
	if equity <= 0 {
		return 0
	}
	return p.positionNotional(markPrice) / equity
}

func (p *portfolio) close(side strategy.TradeSide, price float64, sizePct float64, feeRate float64) (float64, float64, float64, bool) {
	match, ok := quoteClose(*p, side, price, sizePct, feeRate)
	if ok {
		applyMatchedFill(p, match, strategy.ActionClose, price)
	}
	return match.quantity, match.fee, match.realized, ok
}

func (p *portfolio) addQuantity(quantity float64, price float64) {
	if p.quantity == 0 {
		p.quantity = quantity
		p.avgPrice = price
		return
	}
	oldAbs := math.Abs(p.quantity)
	newAbs := math.Abs(quantity)
	p.avgPrice = (p.avgPrice*oldAbs + price*newAbs) / (oldAbs + newAbs)
	p.quantity += quantity
}

func (p *portfolio) resetPosition() {
	p.quantity = 0
	p.avgPrice = 0
	p.entryCount = 0
	p.lastEntryPrice = 0
	p.firstNotional = 0
	p.firstEntryPrice = 0
	p.highestSinceEntry = 0
	p.lowestSinceEntry = 0
}
