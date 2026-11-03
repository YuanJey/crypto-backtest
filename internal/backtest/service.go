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
	StrategyID       string            `json:"strategy_id"`
	StrategyRevision int               `json:"strategy_revision,omitempty"`
	DatasetID        string            `json:"dataset_id"`
	Symbol           string            `json:"symbol"`
	StartMS          int64             `json:"start_ms"`
	EndMS            int64             `json:"end_ms"`
	Report           engine.ReportSpec `json:"report"`
}

type Response struct {
	Status          string                    `json:"status"`
	Strategy        strategy.Summary          `json:"strategy"`
	StrategyVersion strategy.RevisionMetadata `json:"strategy_version"`
	Data            DataProvenance            `json:"data"`
	BarCount        int                       `json:"bar_count"`
	OrderCount      int                       `json:"order_count"`
	FillCount       int                       `json:"fill_count"`
	FinalEquity     float64                   `json:"final_equity"`
	Report          engine.Result             `json:"report"`
}

type DataProvenance struct {
	DatasetID      string `json:"dataset_id"`
	Symbol         string `json:"symbol"`
	Timeframe      string `json:"timeframe"`
	StartMS        int64  `json:"start_ms"`
	EndMS          int64  `json:"end_ms"`
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	Synthetic      bool   `json:"synthetic"`
}

func (s *Service) CompileStrategy(ctx context.Context, strategyID string) (strategy.Definition, compiler.Plan, error) {
	definition, plan, _, err := s.compileStrategyRevision(ctx, strategyID, 0)
	return definition, plan, err
}

func (s *Service) CompileStrategyRevision(
	ctx context.Context,
	strategyID string,
	revision int,
) (strategy.Definition, compiler.Plan, strategy.RevisionMetadata, error) {
	return s.compileStrategyRevision(ctx, strategyID, revision)
}

func (s *Service) compileStrategyRevision(
	ctx context.Context,
	strategyID string,
	revision int,
) (strategy.Definition, compiler.Plan, strategy.RevisionMetadata, error) {
	if strings.TrimSpace(strategyID) == "" {
		return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, fmt.Errorf("strategy_id is required")
	}
	if revision < 0 {
		return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, fmt.Errorf("strategy_revision must be >= 1 when provided")
	}
	var definition strategy.Definition
	var version strategy.RevisionMetadata
	var err error
	if repository, ok := s.repository.(strategy.RevisionRepository); ok {
		if revision > 0 {
			definition, version, err = repository.LoadRevision(ctx, strategyID, revision)
		} else {
			definition, err = repository.LoadByID(ctx, strategyID)
			if err == nil {
				version, err = repository.LatestRevision(ctx, strategyID)
			}
		}
	} else {
		if revision > 1 {
			return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, fmt.Errorf(
				"strategy repository does not support revision %d",
				revision,
			)
		}
		definition, err = s.repository.LoadByID(ctx, strategyID)
		if err == nil {
			version.Revision = 1
			version.DefinitionSHA256, err = strategy.DefinitionSHA256(definition)
		}
	}
	if err != nil {
		return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, err
	}
	plan, err := compiler.Compile(definition, s.registry)
	if err != nil {
		return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, err
	}
	return plan.Strategy, plan, version, nil
}

func (s *Service) Run(ctx context.Context, request Request) (Response, error) {
	definition, plan, version, err := s.compileStrategyRevision(
		ctx,
		request.StrategyID,
		request.StrategyRevision,
	)
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
	normalizedSymbol, err := market.NormalizeSymbol(symbol)
	if err != nil {
		return Response{}, err
	}
	symbol = normalizedSymbol

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
	timeframeMS, err := market.ParseTimeframeMS(timeframe)
	if err != nil {
		return Response{}, err
	}
	if err := market.RequireAlignedRange(startMS, endMS, timeframeMS); err != nil {
		return Response{}, fmt.Errorf("backtest range: %w", err)
	}

	var bars []market.Bar
	provenance := DataProvenance{
		DatasetID: datasetID,
		Symbol:    symbol,
		Timeframe: timeframe,
		StartMS:   startMS,
		EndMS:     endMS,
	}
	if datasetID == "__sample__" || strings.EqualFold(datasetID, "sample") {
		bars = market.GenerateSampleBars(symbol, startMS, endMS)
		expectedBars := (endMS - startMS) / timeframeMS
		if int64(len(bars)) != expectedBars {
			return Response{}, fmt.Errorf(
				"synthetic range requires %d bars; maximum supported is %d",
				expectedBars,
				market.MaxSyntheticBars,
			)
		}
		provenance.DatasetID = "__sample__"
		provenance.Synthetic = true
	} else {
		if datasetID == "" {
			return Response{}, fmt.Errorf("dataset_id is required; use __sample__ explicitly for synthetic data")
		}
		manifest, _, manifestErr := dataset.ReadManifest(s.dataRoot, datasetID)
		if manifestErr != nil {
			return Response{}, fmt.Errorf("read manifest for dataset %q: %w", datasetID, manifestErr)
		}
		if manifest.MarketType != string(definition.Account.MarketType) {
			return Response{}, fmt.Errorf(
				"dataset %q market_type %q does not match strategy account market_type %q",
				datasetID,
				manifest.MarketType,
				definition.Account.MarketType,
			)
		}
		if startMS < manifest.StartMS || endMS > manifest.EndMS {
			return Response{}, fmt.Errorf(
				"backtest range [%d, %d) is outside dataset range [%d, %d)",
				startMS,
				endMS,
				manifest.StartMS,
				manifest.EndMS,
			)
		}
		manifestHash, hashErr := dataset.ManifestSHA256(s.dataRoot, datasetID)
		if hashErr != nil {
			return Response{}, fmt.Errorf("hash manifest for dataset %q: %w", datasetID, hashErr)
		}
		provenance.ManifestSHA256 = manifestHash
		readBars, readErr := dataset.ReadBars(
			s.dataRoot,
			datasetID,
			symbol,
			timeframe,
			startMS,
			endMS,
		)
		if readErr != nil {
			return Response{}, fmt.Errorf("read bars for dataset %q symbol %q: %w", datasetID, symbol, readErr)
		}
		if len(readBars) == 0 {
			return Response{}, fmt.Errorf("no bars found in dataset %q for symbol %q", datasetID, symbol)
		}
		bars = readBars
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
	return BuildResponse(definition, version, provenance, len(bars), result), nil
}

func BuildResponse(
	definition strategy.Definition,
	version strategy.RevisionMetadata,
	data DataProvenance,
	barCount int,
	result engine.Result,
) Response {
	return Response{
		Status:          "backtest_completed",
		Strategy:        strategy.Summarize(definition),
		StrategyVersion: version,
		Data:            data,
		BarCount:        barCount,
		OrderCount:      result.TotalOrders,
		FillCount:       result.TotalFills,
		FinalEquity:     result.FinalEquity,
		Report:          result,
	}
}
