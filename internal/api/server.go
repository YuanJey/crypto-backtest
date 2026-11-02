package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/crypto-backtest/internal/backtest"
	"github.com/bytedance/crypto-backtest/internal/datafeed/binance"
	"github.com/bytedance/crypto-backtest/internal/dataset"
	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/market"
	"github.com/bytedance/crypto-backtest/internal/strategy"
)

type Server struct {
	repository strategy.Repository
	registry   *indicator.Registry
	dataRoot   string
	webRoot    string
	runner     *backtest.Service
}

func NewServer(repository strategy.Repository, registry *indicator.Registry, dataRoot string) *Server {
	return &Server{
		repository: repository,
		registry:   registry,
		dataRoot:   dataRoot,
		webRoot:    "web",
		runner:     backtest.NewService(repository, registry, dataRoot),
	}
}

func (s *Server) SetWebRoot(webRoot string) {
	s.webRoot = webRoot
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/v1/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/v1/strategies", s.handleListStrategies)
	mux.HandleFunc("POST /api/v1/strategies", s.handleCreateStrategy)
	mux.HandleFunc("GET /api/v1/strategies/{id}", s.handleGetStrategy)
	mux.HandleFunc("POST /api/v1/backtests", s.handleRunBacktest)
	mux.HandleFunc("GET /api/v1/market/bars", s.handleGetMarketBars)
	mux.HandleFunc("GET /api/v1/market/datasets", s.handleListDatasets)
	mux.HandleFunc("POST /api/v1/market/download", s.handleDownloadMarketData)

	if s.webRoot != "" {
		if info, err := os.Stat(s.webRoot); err == nil && info.IsDir() {
			mux.Handle("GET /", http.FileServer(http.Dir(s.webRoot)))
		}
	}
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"indicators": s.registry.Definitions(),
	})
}

func (s *Server) handleListStrategies(w http.ResponseWriter, r *http.Request) {
	ids, err := s.repository.ListIDs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	summaries := make([]strategy.Summary, 0, len(ids))
	for _, id := range ids {
		definition, err := s.repository.LoadByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		summaries = append(summaries, strategy.Summarize(definition))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"strategies": summaries})
}

func (s *Server) handleCreateStrategy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	definition, err := strategy.Decode(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := strategy.Validate(definition, s.registry); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.repository.Save(r.Context(), definition, raw); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"strategy": strategy.Summarize(definition),
	})
}

func (s *Server) handleGetStrategy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	definition, err := s.repository.LoadByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, definition)
}

func (s *Server) handleRunBacktest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request backtest.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(request.StrategyID) == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("strategy_id is required"))
		return
	}
	response, err := s.runner.Run(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *Server) handleGetMarketBars(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	datasetID := strings.TrimSpace(query.Get("dataset_id"))
	symbol := strings.TrimSpace(query.Get("symbol"))
	if symbol == "" {
		symbol = "BTC-USDT"
	}
	timeframe := strings.TrimSpace(query.Get("timeframe"))
	if timeframe == "" {
		timeframe = market.BaseTimeframe
	}
	startStr := query.Get("start_ms")
	endStr := query.Get("end_ms")
	var startMS, endMS int64
	if startStr != "" {
		startMS, _ = strconv.ParseInt(startStr, 10, 64)
	}
	if endStr != "" {
		endMS, _ = strconv.ParseInt(endStr, 10, 64)
	}
	if endMS == 0 {
		endMS = math.MaxInt64
	}

	var bars []market.Bar
	var err error

	if datasetID != "" {
		bars, err = dataset.ReadBars(s.dataRoot, datasetID, symbol, market.BaseTimeframe, startMS, endMS)
	}

	if err != nil || len(bars) == 0 {
		bars = generateSampleBars(symbol, startMS, endMS)
	}

	if query.Get("multi") == "true" {
		tf1h, _ := market.ParseTimeframeMS("1h")
		tf4h, _ := market.ParseTimeframeMS("4h")
		tf1d, _ := market.ParseTimeframeMS("1d")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"symbol":         symbol,
			"dataset_id":     datasetID,
			"base_timeframe": market.BaseTimeframe,
			"timeframes": map[string][]market.Bar{
				"15m": bars,
				"1h":  aggregateBars(bars, tf1h),
				"4h":  aggregateBars(bars, tf4h),
				"1d":  aggregateBars(bars, tf1d),
			},
		})
		return
	}

	if timeframe != market.BaseTimeframe {
		targetMS, tfErr := market.ParseTimeframeMS(timeframe)
		if tfErr == nil && targetMS > 0 {
			bars = aggregateBars(bars, targetMS)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"symbol":    symbol,
		"timeframe": timeframe,
		"bars":      bars,
	})
}

func aggregateBars(bars []market.Bar, targetMS int64) []market.Bar {
	if len(bars) == 0 || targetMS <= 0 {
		return nil
	}
	var aggregated []market.Bar
	var current market.Bar
	var active bool

	for _, b := range bars {
		periodStart := (b.OpenTimeMS / targetMS) * targetMS
		if !active || current.OpenTimeMS != periodStart {
			if active {
				current.State = market.BarStateClosed
				aggregated = append(aggregated, current)
			}
			current = market.Bar{
				OpenTimeMS:  periodStart,
				Open:        b.Open,
				High:        b.High,
				Low:         b.Low,
				Close:       b.Close,
				Volume:      b.Volume,
				QuoteVolume: b.QuoteVolume,
				TradeCount:  b.TradeCount,
				State:       market.BarStateForming,
			}
			active = true
		} else {
			if b.High > current.High {
				current.High = b.High
			}
			if b.Low < current.Low {
				current.Low = b.Low
			}
			current.Close = b.Close
			current.Volume += b.Volume
			current.QuoteVolume += b.QuoteVolume
			current.TradeCount += b.TradeCount
		}
	}
	if active {
		aggregated = append(aggregated, current)
	}
	return aggregated
}

func generateSampleBars(symbol string, startMS, endMS int64) []market.Bar {
	baseTime := int64(1735689600000) // 2025-01-01 00:00:00 UTC
	if startMS > 0 && startMS > baseTime {
		baseTime = (startMS / (15 * 60 * 1000)) * (15 * 60 * 1000)
	}
	totalBars := 672 // 7 days of 15m bars
	stepMS := int64(15 * 60 * 1000)

	bars := make([]market.Bar, 0, totalBars)
	price := 93500.0

	for i := 0; i < totalBars; i++ {
		t := baseTime + int64(i)*stepMS
		if t < startMS {
			continue
		}
		if t >= endMS {
			break
		}
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

		bar := market.Bar{
			OpenTimeMS:  t,
			Open:        math.Round(open*100) / 100,
			High:        math.Round(high*100) / 100,
			Low:         math.Round(low*100) / 100,
			Close:       math.Round(closePrice*100) / 100,
			Volume:      math.Round(volume*100) / 100,
			QuoteVolume: math.Round(volume*closePrice*100) / 100,
			TradeCount:  int64(1500 + (i%50)*30),
			State:       market.BarStateClosed,
		}
		bars = append(bars, bar)
		price = closePrice
	}
	return bars
}

func (s *Server) handleListDatasets(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.dataRoot)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]interface{}{"datasets": []interface{}{}})
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	datasets := make([]dataset.Manifest, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, _, err := dataset.ReadManifest(s.dataRoot, entry.Name())
		if err != nil {
			continue
		}
		datasets = append(datasets, manifest)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"datasets": datasets})
}

type downloadRequest struct {
	Exchange   string `json:"exchange"`
	MarketType string `json:"market_type"`
	Symbol     string `json:"symbol"`
	Symbols    string `json:"symbols"`
	Timeframe  string `json:"timeframe"`
	Start      string `json:"start"`
	End        string `json:"end"`
	StartMS    int64  `json:"start_ms"`
	EndMS      int64  `json:"end_ms"`
	DatasetID  string `json:"dataset_id"`
	Stream     bool   `json:"stream"`
}

func (s *Server) handleDownloadMarketData(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req downloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid json body: %w", err))
		return
	}

	isStream := req.Stream || r.URL.Query().Get("stream") == "true" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")

	exchange := strings.ToLower(strings.TrimSpace(req.Exchange))
	if exchange == "" {
		exchange = "binance"
	}
	if exchange != "binance" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported exchange %q, only 'binance' is supported", exchange))
		return
	}

	marketType := strings.ToLower(strings.TrimSpace(req.MarketType))
	if marketType == "" {
		marketType = "spot"
	}
	if marketType != "spot" && marketType != "perpetual" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported market_type %q, must be 'spot' or 'perpetual'", marketType))
		return
	}

	timeframe := strings.TrimSpace(req.Timeframe)
	if timeframe == "" {
		timeframe = market.BaseTimeframe
	}
	if err := market.RequireBaseTimeframe(timeframe); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unsupported timeframe %q: %w", timeframe, err))
		return
	}

	// 提取并清洗交易标的
	rawSymbols := strings.TrimSpace(req.Symbols)
	if rawSymbols == "" {
		rawSymbols = strings.TrimSpace(req.Symbol)
	}
	if rawSymbols == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("symbol is required"))
		return
	}
	parts := strings.Split(rawSymbols, ",")
	symbols := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		sym, err := market.NormalizeSymbol(p)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid symbol %q: %w", p, err))
			return
		}
		symbols = append(symbols, sym)
	}
	if len(symbols) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("at least one valid symbol is required"))
		return
	}

	// 解析时间戳
	startMS := req.StartMS
	if startMS <= 0 {
		var err error
		startMS, err = parseFlexibleTime(req.Start)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid start time: %w", err))
			return
		}
	}
	endMS := req.EndMS
	if endMS <= 0 {
		var err error
		endMS, err = parseFlexibleTime(req.End)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid end time: %w", err))
			return
		}
	}

	tfMS, _ := market.ParseTimeframeMS(timeframe)
	startMS = (startMS / tfMS) * tfMS
	endMS = ((endMS + tfMS - 1) / tfMS) * tfMS
	if startMS >= endMS {
		writeError(w, http.StatusBadRequest, fmt.Errorf("start time (%d) must be before end time (%d)", startMS, endMS))
		return
	}

	datasetID := strings.TrimSpace(req.DatasetID)
	if datasetID == "" {
		clean := func(v string) string {
			return strings.NewReplacer(":", "", "-", "", "T", "", "Z", "").Replace(v)
		}
		sStr := time.UnixMilli(startMS).UTC().Format("20060102T150405Z")
		eStr := time.UnixMilli(endMS).UTC().Format("20060102T150405Z")
		datasetID = fmt.Sprintf("%s-%s-%s-%s-%s", exchange, marketType, timeframe, clean(sStr), clean(eStr))
	}

	client, err := binance.NewClient(marketType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("init binance client: %w", err))
		return
	}

	var flusher http.Flusher
	var sendSSE func(stage string, current, total int64, percent float64, message string, extra ...interface{})
	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, _ = w.(http.Flusher)

		sendSSE = func(stage string, current, total int64, percent float64, message string, extra ...interface{}) {
			payload := map[string]interface{}{
				"stage":   stage,
				"current": current,
				"total":   total,
				"percent": math.Round(percent*10) / 10,
				"message": message,
			}
			if len(extra) > 0 && extra[0] != nil {
				payload["result"] = extra[0]
			}
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
		sendSSE("init", 0, 100, 0.0, "初始化下载流水线...")
	}

	var onProgress func(stage string, current, total int64, percent float64, message string)
	if isStream {
		onProgress = func(stage string, current, total int64, percent float64, message string) {
			sendSSE(stage, current, total, percent, message)
		}
	}

	result, err := dataset.Build(r.Context(), client, dataset.BuildRequest{
		Exchange:   exchange,
		MarketType: marketType,
		Symbols:    symbols,
		Timeframe:  timeframe,
		StartMS:    startMS,
		EndMS:      endMS,
		OutputRoot: s.dataRoot,
		DatasetID:  datasetID,
		Overwrite:  true,
		OnProgress: onProgress,
	})
	if err != nil {
		if isStream {
			sendSSE("error", 0, 0, 0.0, fmt.Sprintf("构建失败: %v", err))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Errorf("build dataset failed: %w", err))
		return
	}

	if isStream {
		sendSSE("done", int64(result.Manifest.RowCount), int64(result.Manifest.RowCount), 100.0, "数据构建完成！", map[string]interface{}{
			"dataset_root": result.DatasetRoot,
			"manifest":     result.Manifest,
			"reports":      result.Reports,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"dataset_root": result.DatasetRoot,
		"manifest":     result.Manifest,
		"reports":      result.Reports,
	})
}

func parseFlexibleTime(val string) (int64, error) {
	s := strings.TrimSpace(val)
	if s == "" {
		return 0, fmt.Errorf("time value is empty")
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return n, nil
	}
	if len(s) == len("2006-01-02T15:04") && strings.Contains(s, "T") {
		s += ":00Z"
	} else if len(s) == len("2006-01-02T15:04:05") && strings.Contains(s, "T") {
		s += "Z"
	}
	return market.ParseUTCMS(s)
}
