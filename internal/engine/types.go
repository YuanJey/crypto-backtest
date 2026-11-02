package engine

import (
	"github.com/bytedance/crypto-backtest/internal/market"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

type Result struct {
	Orders      []Order       `json:"orders"`
	Fills       []Fill        `json:"fills"`
	EquityCurve []EquityPoint `json:"equity_curve"`
	FinalEquity float64       `json:"final_equity"`
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
	Bars []market.Bar
}
