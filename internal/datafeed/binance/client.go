package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/datafeed"
	"github.com/YuanJey/crypto-backtest/internal/market"
)

const (
	spotKlinesURL    = "https://api.binance.com/api/v3/klines"
	futuresKlinesURL = "https://fapi.binance.com/fapi/v1/klines"
)

type MarketType string

const (
	MarketTypeSpot      MarketType = "spot"
	MarketTypePerpetual MarketType = "perpetual"
)

type Client struct {
	MarketType     MarketType
	HTTPClient     *http.Client
	Limit          int
	MaxRetries     int
	RequestPause   time.Duration
	RetryBasePause time.Duration
}

func NewClient(marketType string) (*Client, error) {
	mt := MarketType(marketType)
	if mt != MarketTypeSpot && mt != MarketTypePerpetual {
		return nil, fmt.Errorf("unsupported binance market type %q", marketType)
	}
	return &Client{
		MarketType: mt,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Limit:          1000,
		MaxRetries:     3,
		RequestPause:   200 * time.Millisecond,
		RetryBasePause: 500 * time.Millisecond,
	}, nil
}

func (c *Client) FetchOHLCV(ctx context.Context, request datafeed.FetchRequest) ([]market.Bar, error) {
	timeframeMS, err := market.ParseTimeframeMS(request.Timeframe)
	if err != nil {
		return nil, err
	}
	if err := market.RequireAlignedRange(request.StartMS, request.EndMS, timeframeMS); err != nil {
		return nil, err
	}

	cursor := request.StartMS
	timeframeMSVal := timeframeMS
	totalEstimated := (request.EndMS - request.StartMS) / timeframeMSVal
	if totalEstimated <= 0 {
		totalEstimated = 1
	}
	if request.OnProgress != nil {
		request.OnProgress("fetch", 0, totalEstimated, 0.0, fmt.Sprintf("开始拉取 %s 15m K线行情...", request.Symbol))
	}

	bars := make([]market.Bar, 0)
	for cursor < request.EndMS {
		rows, err := c.requestKlines(ctx, request.Symbol, request.Timeframe, cursor, request.EndMS-1)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			bar, err := parseKline(row)
			if err != nil {
				return nil, err
			}
			if bar.OpenTimeMS >= request.EndMS {
				continue
			}
			bars = append(bars, bar)
		}

		lastOpenMS, err := parseInt64(rows[len(rows)-1][0])
		if err != nil {
			return nil, err
		}
		nextCursor := lastOpenMS + timeframeMS
		if nextCursor <= cursor {
			return nil, fmt.Errorf("binance pagination did not advance")
		}
		cursor = nextCursor

		if request.OnProgress != nil {
			currentCount := int64(len(bars))
			progressPct := (float64(currentCount) / float64(totalEstimated)) * 80.0
			if progressPct > 80.0 {
				progressPct = 80.0
			}
			msg := fmt.Sprintf("正在拉取行情数据: 已获取 %d/%d 根 K 线...", currentCount, totalEstimated)
			request.OnProgress("fetch", currentCount, totalEstimated, progressPct, msg)
		}

		if c.RequestPause > 0 {
			timer := time.NewTimer(c.RequestPause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}

	return bars, nil
}

func (c *Client) requestKlines(ctx context.Context, symbol, timeframe string, startMS, endMS int64) ([][]json.RawMessage, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	limit := c.Limit
	if limit <= 0 {
		limit = 1000
	}
	maxRetries := c.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	query := url.Values{}
	query.Set("symbol", market.ExchangeSymbol(symbol))
	query.Set("interval", timeframe)
	query.Set("startTime", strconv.FormatInt(startMS, 10))
	query.Set("endTime", strconv.FormatInt(endMS, 10))
	query.Set("limit", strconv.Itoa(limit))
	endpoint := c.klinesURL() + "?" + query.Encode()

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "crypto-backtest-market-data/0.1")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			var payload [][]json.RawMessage
			func() {
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					lastErr = fmt.Errorf("binance returned HTTP %d", resp.StatusCode)
					return
				}
				if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
					lastErr = err
					return
				}
				lastErr = nil
			}()
			if lastErr == nil {
				return payload, nil
			}
		}

		if attempt < maxRetries {
			pause := c.RetryBasePause
			if pause <= 0 {
				pause = 500 * time.Millisecond
			}
			timer := time.NewTimer(time.Duration(attempt) * pause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}

	return nil, fmt.Errorf("fetch binance klines failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) klinesURL() string {
	if c.MarketType == MarketTypePerpetual {
		return futuresKlinesURL
	}
	return spotKlinesURL
}

func parseKline(row []json.RawMessage) (market.Bar, error) {
	if len(row) < 9 {
		return market.Bar{}, fmt.Errorf("binance kline row has %d fields, want at least 9", len(row))
	}
	openTimeMS, err := parseInt64(row[0])
	if err != nil {
		return market.Bar{}, err
	}
	open, err := parseFloat(row[1])
	if err != nil {
		return market.Bar{}, err
	}
	high, err := parseFloat(row[2])
	if err != nil {
		return market.Bar{}, err
	}
	low, err := parseFloat(row[3])
	if err != nil {
		return market.Bar{}, err
	}
	closeValue, err := parseFloat(row[4])
	if err != nil {
		return market.Bar{}, err
	}
	volume, err := parseFloat(row[5])
	if err != nil {
		return market.Bar{}, err
	}
	quoteVolume, err := parseFloat(row[7])
	if err != nil {
		return market.Bar{}, err
	}
	tradeCount, err := parseInt64(row[8])
	if err != nil {
		return market.Bar{}, err
	}

	return market.Bar{
		OpenTimeMS:  openTimeMS,
		Open:        open,
		High:        high,
		Low:         low,
		Close:       closeValue,
		Volume:      volume,
		QuoteVolume: quoteVolume,
		TradeCount:  tradeCount,
		State:       market.BarStateClosed,
	}, nil
}

func parseFloat(raw json.RawMessage) (float64, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strconv.ParseFloat(text, 64)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func parseInt64(raw json.RawMessage) (int64, error) {
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strconv.ParseInt(text, 10, 64)
	}
	return 0, fmt.Errorf("cannot parse int64 from %s", string(raw))
}
