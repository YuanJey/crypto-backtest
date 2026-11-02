package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestServerCreatesAndListsStrategies(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), t.TempDir())

	raw := mustStrategyJSON(t)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/strategies", bytes.NewReader(raw))
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/strategies", nil)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	if !bytes.Contains(listRec.Body.Bytes(), []byte(`"id":"ema-boll-rsi"`)) {
		t.Fatalf("expected strategy id in list response: %s", listRec.Body.String())
	}
}

func TestServerRejectsMissingStrategyIDBacktest(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), t.TempDir())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func mustStrategyJSON(t *testing.T) []byte {
	t.Helper()
	definition := strategySummaryFixture()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func strategySummaryFixture() strategy.Definition {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	value := true
	return strategy.Definition{
		SchemaVersion: strategy.SchemaVersion,
		ID:            "ema-boll-rsi",
		Name:          "EMA + BOLL + RSI",
		Data: strategy.DataSelection{
			DatasetID: "binance-perpetual-15m-202501",
			Symbols:   []string{"BTC-USDT"},
			Timeframe: "15m",
			Start:     start,
			End:       end,
		},
		Account: strategy.AccountSpec{
			MarketType:    strategy.MarketTypePerpetual,
			InitialCash:   10_000,
			QuoteCurrency: "USDT",
		},
		Execution: strategy.ExecutionSpec{
			SignalTime:       strategy.SignalTimeClose,
			MarketFillTime:   strategy.MarketFillNextOpen,
			IntrabarConflict: strategy.IntrabarConflictConservative,
			Fee:              strategy.FeeSpec{MakerRate: 0.001, TakerRate: 0.001},
			Slippage:         strategy.SlippageSpec{Type: strategy.SlippageTypeNone},
		},
		Risk: strategy.RiskSpec{
			MaxSymbolEquityPct:  1,
			MaxTotalExposurePct: 1,
			MaxOpenPositions:    1,
			MaxDrawdownHaltPct:  0.25,
		},
		Indicators: []strategy.IndicatorSpec{{
			ID:        "ema_fast_15m",
			Type:      "ema",
			Timeframe: "15m",
			Inputs: []strategy.ValueRef{{
				Scope: strategy.ValueScopeMarket,
				Name:  "bar",
				Field: "close",
			}},
			Params: json.RawMessage(`{"period":20}`),
		}},
		Rules: []strategy.RuleSpec{{
			ID: "open_long",
			When: strategy.Expression{
				Kind: strategy.ExprBoolean,
				Bool: &value,
			},
			Action: strategy.ActionSpec{
				Type:    strategy.ActionOpen,
				Side:    strategy.TradeSideLong,
				SizePct: 0.2,
			},
		}},
	}
}

func TestServerMarketBars(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/bars?symbol=BTC-USDT&timeframe=15m", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Symbol    string       `json:"symbol"`
		Timeframe string       `json:"timeframe"`
		Bars      []market.Bar `json:"bars"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(response.Bars) == 0 {
		t.Fatal("expected non-empty bars")
	}
}

func TestServerMarketBarsMulti(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/bars?symbol=BTC-USDT&multi=true", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Symbol     string                  `json:"symbol"`
		Timeframes map[string][]market.Bar `json:"timeframes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	for _, tf := range []string{"15m", "1h", "4h", "1d"} {
		if len(response.Timeframes[tf]) == 0 {
			t.Fatalf("expected non-empty bars for %s", tf)
		}
	}
	if len(response.Timeframes["15m"]) <= len(response.Timeframes["1h"]) {
		t.Fatalf("expected 15m bar count > 1h bar count")
	}
}

func TestServerStaticWebFiles(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), t.TempDir())
	server.SetWebRoot("../../web") // relative to internal/api

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("QUANTIX")) {
		t.Fatalf("expected web/index.html to contain QUANTIX brand")
	}
}

func TestServerListDatasets(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	dataDir := t.TempDir()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), dataDir)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/market/datasets", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"datasets":[]`)) {
		t.Fatalf("expected empty datasets array: %s", rec.Body.String())
	}
}

func TestServerDownloadMarketDataValidation(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	dataDir := t.TempDir()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), dataDir)

	// Case 1: 缺少 symbol
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/market/download", bytes.NewReader([]byte(`{
		"exchange": "binance",
		"timeframe": "15m",
		"start": "2025-01-01T00:00:00Z",
		"end": "2025-01-02T00:00:00Z"
	}`)))
	rec1 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing symbol, got %d", rec1.Code)
	}

	// Case 2: 非 15m 周期
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/market/download", bytes.NewReader([]byte(`{
		"exchange": "binance",
		"symbol": "BTC-USDT",
		"timeframe": "1h",
		"start": "2025-01-01T00:00:00Z",
		"end": "2025-01-02T00:00:00Z"
	}`)))
	rec2 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-15m timeframe, got %d", rec2.Code)
	}

	// Case 3: 起始时间大于结束时间
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/market/download", bytes.NewReader([]byte(`{
		"exchange": "binance",
		"symbol": "BTC-USDT",
		"timeframe": "15m",
		"start": "2025-01-05T00:00:00Z",
		"end": "2025-01-02T00:00:00Z"
	}`)))
	rec3 := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid range, got %d", rec3.Code)
	}
}

func TestServerDownloadMarketDataStreamHeaders(t *testing.T) {
	repository, err := strategy.OpenSQLiteRepository(context.Background(), filepath.Join(t.TempDir(), "strategies.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	dataDir := t.TempDir()
	server := NewServer(repository, indicator.MustBuiltinRegistry(), dataDir)

	// 非法参数但是请求流式
	req := httptest.NewRequest(http.MethodPost, "/api/v1/market/download?stream=true", bytes.NewReader([]byte(`{
		"exchange": "binance",
		"symbol": "BTC-USDT",
		"timeframe": "invalid",
		"start": "2025-01-01T00:00:00Z",
		"end": "2025-01-02T00:00:00Z"
	}`)))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	// 校验失败仍应按 400 返回
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid timeframe, got %d", rec.Code)
	}
}


