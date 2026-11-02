package engine

import (
	"math"

	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type portfolio struct {
	marketType     strategy.MarketType
	cash           float64
	quantity       float64
	avgPrice       float64
	entryCount     int
	lastEntryPrice float64
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

func (p *portfolio) open(side strategy.TradeSide, price float64, sizePct float64, feeRate float64, allowPyramiding bool) (float64, float64, bool) {
	if p.quantity != 0 {
		if p.currentSide() != side || !allowPyramiding {
			return 0, 0, false
		}
	}
	notional := p.equity(price) * sizePct
	if notional <= 0 {
		return 0, 0, false
	}
	quantity := notional / price
	fee := notional * feeRate
	if side == strategy.TradeSideShort {
		quantity = -quantity
	}
	if p.marketType == strategy.MarketTypeSpot {
		p.cash -= notional + fee
	} else {
		p.cash -= fee
	}
	p.addQuantity(quantity, price)
	p.entryCount++
	p.lastEntryPrice = price
	return quantity, fee, true
}

func (p portfolio) positionNotional(markPrice float64) float64 {
	return math.Abs(p.quantity) * markPrice
}

func (p *portfolio) close(side strategy.TradeSide, price float64, sizePct float64, feeRate float64) (float64, float64, float64, bool) {
	if p.quantity == 0 || p.currentSide() != side {
		return 0, 0, 0, false
	}
	closeAbs := math.Abs(p.quantity) * sizePct
	if closeAbs <= 0 {
		return 0, 0, 0, false
	}
	notional := closeAbs * price
	fee := notional * feeRate
	var realized float64
	var signedQty float64
	if p.quantity > 0 {
		realized = (price - p.avgPrice) * closeAbs
		signedQty = closeAbs
		if p.marketType == strategy.MarketTypeSpot {
			p.cash += notional - fee
		} else {
			p.cash += realized - fee
		}
		p.quantity -= closeAbs
	} else {
		realized = (p.avgPrice - price) * closeAbs
		signedQty = -closeAbs
		p.cash += realized - fee
		p.quantity += closeAbs
	}
	if math.Abs(p.quantity) < 1e-12 {
		p.resetPosition()
	}
	return signedQty, fee, realized, true
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
}
