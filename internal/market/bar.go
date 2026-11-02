package market

type BarState string

const (
	BarStateForming BarState = "forming"
	BarStateClosed  BarState = "closed"
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
