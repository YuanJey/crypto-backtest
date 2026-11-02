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

	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/strategy"
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
