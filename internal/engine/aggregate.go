package engine

import "github.com/bytedance/crypto-backtest/internal/market"

type barAggregator struct {
	timeframeMS int64
	baseMS      int64
	current     market.Bar
	count       int64
	active      bool
}

func newBarAggregator(timeframeMS, baseMS int64) barAggregator {
	return barAggregator{timeframeMS: timeframeMS, baseMS: baseMS}
}

func (a *barAggregator) Update(bar market.Bar) (market.Bar, bool) {
	periodStart := (bar.OpenTimeMS / a.timeframeMS) * a.timeframeMS
	if !a.active || a.current.OpenTimeMS != periodStart {
		a.current = market.Bar{
			OpenTimeMS:  periodStart,
			Open:        bar.Open,
			High:        bar.High,
			Low:         bar.Low,
			Close:       bar.Close,
			Volume:      bar.Volume,
			QuoteVolume: bar.QuoteVolume,
			TradeCount:  bar.TradeCount,
		}
		a.count = 1
		a.active = true
	} else {
		if bar.High > a.current.High {
			a.current.High = bar.High
		}
		if bar.Low < a.current.Low {
			a.current.Low = bar.Low
		}
		a.current.Close = bar.Close
		a.current.Volume += bar.Volume
		a.current.QuoteVolume += bar.QuoteVolume
		a.current.TradeCount += bar.TradeCount
		a.count++
	}

	periodEnd := periodStart + a.timeframeMS
	if bar.OpenTimeMS+a.baseMS != periodEnd {
		return market.Bar{}, false
	}
	expectedBars := a.timeframeMS / a.baseMS
	completed := a.current
	complete := a.count == expectedBars
	a.current = market.Bar{}
	a.count = 0
	a.active = false
	if !complete {
		return market.Bar{}, false
	}
	return completed, true
}
