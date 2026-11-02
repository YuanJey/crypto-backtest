package datafeed

import (
	"context"

	"github.com/bytedance/crypto-backtest/internal/market"
)

type FetchRequest struct {
	Symbol    string
	Timeframe string
	StartMS   int64
	EndMS     int64
}

type Client interface {
	FetchOHLCV(ctx context.Context, request FetchRequest) ([]market.Bar, error)
}
