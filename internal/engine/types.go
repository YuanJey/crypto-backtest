package engine

import (
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type Result struct {
	Orders      []Order                    `json:"orders,omitempty"`
	Fills       []Fill                     `json:"fills,omitempty"`
	EquityCurve []EquityPoint              `json:"equity_curve,omitempty"`
	Indicators  map[string]IndicatorReport `json:"indicators,omitempty"`
	Metrics     map[MetricType]float64     `json:"metrics,omitempty"`
	Ledger      []LedgerEvent              `json:"ledger,omitempty"`
	Margin      *MarginReport              `json:"margin,omitempty"`
	FinalEquity float64                    `json:"final_equity"`
	TotalOrders int                        `json:"-"`
	TotalFills  int                        `json:"-"`
}

type OrderStatus string

const (
	OrderStatusPending OrderStatus = "pending"
	OrderStatusFilled  OrderStatus = "filled"
	OrderStatusSkipped OrderStatus = "skipped"
)

type Order struct {
	ID           string              `json:"id"`
	RuleID       string              `json:"rule_id"`
	SignalTimeMS int64               `json:"signal_time_ms"`
	Action       strategy.ActionSpec `json:"action"`
	Status       OrderStatus         `json:"status"`
	Reason       string              `json:"reason,omitempty"`
}

type Fill struct {
	OrderID     string             `json:"order_id"`
	RuleID      string             `json:"rule_id"`
	TimeMS      int64              `json:"time_ms"`
	Price       float64            `json:"price"`
	Quantity    float64            `json:"quantity"`
	Fee         float64            `json:"fee"`
	RealizedPNL float64            `json:"realized_pnl"`
	Side        strategy.TradeSide `json:"side"`
	Reason      string             `json:"reason,omitempty"`
}

type EquityPoint struct {
	TimeMS            int64    `json:"time_ms"`
	Equity            float64  `json:"equity"`
	Cash              float64  `json:"cash"`
	PositionQty       float64  `json:"position_qty"`
	PositionLeverage  float64  `json:"position_leverage"`
	Close             float64  `json:"close"`
	MaintenanceMargin *float64 `json:"maintenance_margin,omitempty"`
	MMR               *float64 `json:"mmr,omitempty"`
	// Legacy equity / notional ratio. Use MMR for account maintenance coverage.
	MarginRatio      *float64 `json:"margin_ratio,omitempty"`
	LiquidationPrice *float64 `json:"liquidation_price,omitempty"`
}

type RunRequest struct {
	Bars       []market.Bar
	Report     ReportSpec
	Config     *strategy.RunConfig
	OnProgress func(completed, total int)
}

type BacktestRequest = RunRequest

type ReportSpec struct {
	EquityCurve bool               `json:"equity_curve,omitempty"`
	Orders      bool               `json:"orders,omitempty"`
	Fills       bool               `json:"fills,omitempty"`
	Ledger      bool               `json:"ledger,omitempty"`
	Metrics     []MetricType       `json:"metrics,omitempty"`
	Indicators  []IndicatorOverlay `json:"indicators,omitempty"`
}

type MetricType string

const (
	MetricFinalEquity         MetricType = "final_equity"
	MetricTotalReturnPct      MetricType = "total_return_pct"
	MetricMaxDrawdownPct      MetricType = "max_drawdown_pct"
	MetricOrderCount          MetricType = "order_count"
	MetricFillCount           MetricType = "fill_count"
	MetricMaxPositionLeverage MetricType = "max_position_leverage"
	MetricMinimumMMR          MetricType = "minimum_mmr"
)

type IndicatorOverlay struct {
	ID     string   `json:"id"`
	Fields []string `json:"fields,omitempty"`
}

type IndicatorReport struct {
	Fields map[string][]*float64 `json:"fields"`
}

func DefaultReportSpec() ReportSpec {
	return ReportSpec{
		EquityCurve: true,
		Orders:      true,
		Fills:       true,
		Metrics: []MetricType{
			MetricFinalEquity,
			MetricTotalReturnPct,
			MetricMaxDrawdownPct,
			MetricOrderCount,
			MetricFillCount,
			MetricMaxPositionLeverage,
		},
	}
}
