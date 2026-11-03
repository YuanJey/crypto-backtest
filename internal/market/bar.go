package market

import "math"

type BarState string

const (
	BarStateForming  BarState = "forming"
	BarStateClosed   BarState = "closed"
	MaxSyntheticBars          = 100_000
)

// Bar is the canonical OHLCV contract shared by data download, cleaning and
// the future Go backtest engine. OpenTimeMS is the UTC bar open timestamp.
type Bar struct {
	OpenTimeMS  int64    `json:"open_time_ms" parquet:"open_time_ms"`
	Open        float64  `json:"open" parquet:"open"`
	High        float64  `json:"high" parquet:"high"`
	Low         float64  `json:"low" parquet:"low"`
	Close       float64  `json:"close" parquet:"close"`
	Volume      float64  `json:"volume" parquet:"volume"`
	QuoteVolume float64  `json:"quote_volume" parquet:"quote_volume"`
	TradeCount  int64    `json:"trade_count" parquet:"trade_count"`
	State       BarState `json:"state" parquet:"state"`
}

func (b Bar) IsClosed() bool {
	return b.State == "" || b.State == BarStateClosed
}

func (b Bar) WithDefaultClosedState() Bar {
	if b.State == "" {
		b.State = BarStateClosed
	}
	return b
}

// GenerateSampleBars returns deterministic 15m synthetic bars for explicit demos.
func GenerateSampleBars(symbol string, startMS, endMS int64) []Bar {
	stepMS := int64(15 * 60 * 1000)
	totalBars := 672 // 默认生成 7 天 15m K 线

	baseTime := int64(1735689600000) // 2025-01-01 00:00:00 UTC
	if startMS > 0 || (endMS > 0 && endMS != math.MaxInt64) {
		baseTime = (startMS / stepMS) * stepMS
	}

	if endMS > baseTime && endMS != math.MaxInt64 {
		barsInRange := (endMS - baseTime) / stepMS
		if barsInRange > 0 {
			totalBars = int(min(barsInRange, MaxSyntheticBars))
		}
	}

	bars := make([]Bar, 0, totalBars)
	price := 93500.0

	for i := 0; i < totalBars; i++ {
		t := baseTime + int64(i)*stepMS
		// Deterministic sinusoidal wave + trend oscillation

		phase := float64(i) * 0.08
		harmonic := math.Sin(phase)*180.0 + math.Cos(phase*0.35)*350.0 + math.Sin(float64(i)*0.015)*600.0
		drift := float64(i) * 6.5

		open := price
		noise := math.Sin(float64(i*13))*45.0 + math.Cos(float64(i*7))*30.0
		closePrice := 93500.0 + harmonic + drift + noise
		high := math.Max(open, closePrice) + math.Abs(math.Sin(float64(i*17)))*75.0 + 20.0
		low := math.Min(open, closePrice) - math.Abs(math.Cos(float64(i*23)))*75.0 - 20.0
		volume := 120.0 + math.Abs(math.Sin(float64(i)))*250.0 + math.Abs(closePrice-open)*0.8

		bar := Bar{
			OpenTimeMS:  t,
			Open:        math.Round(open*100) / 100,
			High:        math.Round(high*100) / 100,
			Low:         math.Round(low*100) / 100,
			Close:       math.Round(closePrice*100) / 100,
			Volume:      math.Round(volume*100) / 100,
			QuoteVolume: math.Round(volume*closePrice*100) / 100,
			TradeCount:  int64(1500 + (i%50)*30),
			State:       BarStateClosed,
		}
		bars = append(bars, bar)
		price = closePrice
	}
	return bars
}
