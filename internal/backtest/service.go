package backtest

import (
	"context"
	"fmt"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/dataset"
	"github.com/YuanJey/crypto-backtest/internal/engine"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type Service struct {
	repository strategy.Repository
	registry   *indicator.Registry
	dataRoot   string
}

func NewService(repository strategy.Repository, registry *indicator.Registry, dataRoot string) *Service {
	return &Service{
		repository: repository,
		registry:   registry,
		dataRoot:   dataRoot,
	}
}

type Request struct {
	StrategyID string            `json:"strategy_id"`
	DatasetID  string            `json:"dataset_id"`
	Symbol     string            `json:"symbol"`
	StartMS    int64             `json:"start_ms"`
	EndMS      int64             `json:"end_ms"`
	Report     engine.ReportSpec `json:"report"`
}

type Response struct {
	Status      string           `json:"status"`
	Strategy    strategy.Summary `json:"strategy"`
	BarCount    int              `json:"bar_count"`
	OrderCount  int              `json:"order_count"`
	FillCount   int              `json:"fill_count"`
	FinalEquity float64          `json:"final_equity"`
	Report      engine.Result    `json:"report"`
}

func (s *Service) CompileStrategy(ctx context.Context, strategyID string) (strategy.Definition, compiler.Plan, error) {
	if strings.TrimSpace(strategyID) == "" {
		return strategy.Definition{}, compiler.Plan{}, fmt.Errorf("strategy_id is required")
	}
	definition, err := s.repository.LoadByID(ctx, strategyID)
	if err != nil {
		return strategy.Definition{}, compiler.Plan{}, err
	}
	plan, err := compiler.Compile(definition, s.registry)
	if err != nil {
		return strategy.Definition{}, compiler.Plan{}, err
	}
	return definition, plan, nil
}

func (s *Service) Run(ctx context.Context, request Request) (Response, error) {
	definition, plan, err := s.CompileStrategy(ctx, request.StrategyID)
	if err != nil {
		return Response{}, err
	}

	datasetID := definition.Data.DatasetID
	if strings.TrimSpace(request.DatasetID) != "" {
		datasetID = strings.TrimSpace(request.DatasetID)
	}

	symbol := ""
	if len(definition.Data.Symbols) > 0 {
		symbol = definition.Data.Symbols[0]
	}
	if strings.TrimSpace(request.Symbol) != "" {
		symbol = strings.TrimSpace(request.Symbol)
	}
	if symbol == "" {
		return Response{}, fmt.Errorf("initial engine supports exactly one symbol")
	}

	timeframe := definition.Data.Timeframe
	if timeframe == "" {
		timeframe = "15m"
	}

	startMS := definition.Data.Start.UTC().UnixMilli()
	if request.StartMS > 0 {
		startMS = request.StartMS
	}
	endMS := definition.Data.End.UTC().UnixMilli()
	if request.EndMS > 0 {
		endMS = request.EndMS
	}

	var bars []market.Bar
	if datasetID == "__sample__" || strings.EqualFold(datasetID, "sample") {
		bars = market.GenerateSampleBars(symbol, startMS, endMS)
	} else if datasetID != "" {
		readBars, readErr := dataset.ReadBars(
			s.dataRoot,
			datasetID,
			symbol,
			timeframe,
			startMS,
			endMS,
		)
		if readErr == nil && len(readBars) > 0 {
			bars = readBars
		} else {
			if strings.TrimSpace(request.DatasetID) == "" {
				bars = market.GenerateSampleBars(symbol, startMS, endMS)
			} else {
				if readErr != nil {
					return Response{}, fmt.Errorf("read bars for dataset %q symbol %q: %w", datasetID, symbol, readErr)
				}
				return Response{}, fmt.Errorf("no bars found in dataset %q for symbol %q", datasetID, symbol)
			}
		}
	} else {
		bars = market.GenerateSampleBars(symbol, startMS, endMS)
	}

	if len(bars) == 0 {
		return Response{}, fmt.Errorf("no bars available for backtest")
	}

	result, err := engine.Run(ctx, plan, engine.BacktestRequest{
		Bars:   bars,
		Report: request.Report,
	})
	if err != nil {
		return Response{}, err
	}
	return BuildResponse(definition, len(bars), result), nil
}

func BuildResponse(definition strategy.Definition, barCount int, result engine.Result) Response {
	orderCount := len(result.Orders)
	if value, ok := result.Metrics[engine.MetricOrderCount]; ok {
		orderCount = int(value)
	}
	fillCount := len(result.Fills)
	if value, ok := result.Metrics[engine.MetricFillCount]; ok {
		fillCount = int(value)
	}
	return Response{
		Status:      "backtest_completed",
		Strategy:    strategy.Summarize(definition),
		BarCount:    barCount,
		OrderCount:  orderCount,
		FillCount:   fillCount,
		FinalEquity: result.FinalEquity,
		Report:      result,
	}
}
