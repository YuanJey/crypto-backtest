package engine

import (
	"github.com/bytedance/crypto-backtest/internal/market"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

type Result struct {
	Orders      []Order                    `json:"orders,omitempty"`
	Fills       []Fill                     `json:"fills,omitempty"`
	EquityCurve []EquityPoint              `json:"equity_curve,omitempty"`
	Indicators  map[string]IndicatorReport `json:"indicators,omitempty"`
	Metrics     map[MetricType]float64     `json:"metrics,omitempty"`
	FinalEquity float64                    `json:"final_equity"`
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
}

type EquityPoint struct {
	TimeMS      int64   `json:"time_ms"`
	Equity      float64 `json:"equity"`
	Cash        float64 `json:"cash"`
	PositionQty float64 `json:"position_qty"`
	Close       float64 `json:"close"`
}

type RunRequest struct {
	Bars   []market.Bar
	Report ReportSpec
}

type BacktestRequest = RunRequest

type ReportSpec struct {
	EquityCurve bool               `json:"equity_curve,omitempty"`
	Orders      bool               `json:"orders,omitempty"`
	Fills       bool               `json:"fills,omitempty"`
	Metrics     []MetricType       `json:"metrics,omitempty"`
	Indicators  []IndicatorOverlay `json:"indicators,omitempty"`
}

type MetricType string

const (
	MetricFinalEquity    MetricType = "final_equity"
	MetricTotalReturnPct MetricType = "total_return_pct"
	MetricMaxDrawdownPct MetricType = "max_drawdown_pct"
	MetricOrderCount     MetricType = "order_count"
	MetricFillCount      MetricType = "fill_count"
)

type IndicatorOverlay struct {
	ID     string   `json:"id"`
	Fields []string `json:"fields,omitempty"`
}

type IndicatorReport struct {
	Fields map[string][]float64 `json:"fields"`
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
		},
	}
}
