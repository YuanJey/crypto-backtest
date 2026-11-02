package datafeed

import (
	"context"

	"github.com/YuanJey/crypto-backtest/internal/market"
)

type ProgressCallback func(stage string, current, total int64, percent float64, message string)

type FetchRequest struct {
	Symbol     string
	Timeframe  string
	StartMS    int64
	EndMS      int64
	OnProgress ProgressCallback
}

type Client interface {
	FetchOHLCV(ctx context.Context, request FetchRequest) ([]market.Bar, error)
}
