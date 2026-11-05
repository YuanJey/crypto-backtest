package backtest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/engine"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type Service struct {
	repository strategy.Repository
	registry   *indicator.Registry
	provider   DataProvider
}

func NewService(repository strategy.Repository, registry *indicator.Registry, dataRoot string) *Service {
	return &Service{
		repository: repository,
		registry:   registry,
		provider:   FileDataProvider{Root: dataRoot},
	}
}

type Request struct {
	StrategyID       string              `json:"strategy_id"`
	StrategyRevision int                 `json:"strategy_revision,omitempty"`
	DatasetID        string              `json:"dataset_id"`
	Symbol           string              `json:"symbol"`
	StartMS          int64               `json:"start_ms"`
	EndMS            int64               `json:"end_ms"`
	Report           engine.ReportSpec   `json:"report"`
	RunConfig        *strategy.RunConfig `json:"run_config,omitempty"`
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
	RunID           string                    `json:"run_id,omitempty"`
	RunConfig       strategy.RunConfig        `json:"run_config"`
}

type DataProvenance struct {
	DatasetID      string     `json:"dataset_id"`
	Symbol         string     `json:"symbol"`
	Timeframe      string     `json:"timeframe"`
	StartMS        int64      `json:"start_ms"`
	EndMS          int64      `json:"end_ms"`
	ManifestSHA256 string     `json:"manifest_sha256,omitempty"`
	BarsSHA256     string     `json:"bars_sha256"`
	Files          []DataFile `json:"files,omitempty"`
	Synthetic      bool       `json:"synthetic"`
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
			version, err = repository.LatestRevision(ctx, strategyID)
			if err == nil {
				definition, version, err = repository.LoadRevision(ctx, strategyID, version.Revision)
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
	hash, err := strategy.DefinitionSHA256(definition)
	if err != nil || hash != version.DefinitionSHA256 {
		return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, fmt.Errorf("strategy revision hash mismatch")
	}
	plan, err := compiler.Compile(definition, s.registry)
	if err != nil {
		return strategy.Definition{}, compiler.Plan{}, strategy.RevisionMetadata{}, err
	}
	return plan.Definition(), plan, version, nil
}

func (s *Service) Run(ctx context.Context, request Request) (Response, error) {
	prepared, err := s.Prepare(ctx, request)
	if err != nil {
		return Response{}, err
	}
	return s.Execute(ctx, prepared, nil)
}

type PreparedRun struct {
	Request    Request
	Definition strategy.Definition
	Version    strategy.RevisionMetadata
	Config     strategy.RunConfig
	Data       DataSnapshot
	Plan       compiler.Plan
}

func (s *Service) SetDataProvider(provider DataProvider) { s.provider = provider }

func (s *Service) Prepare(ctx context.Context, request Request) (PreparedRun, error) {
	definition, plan, version, err := s.compileStrategyRevision(
		ctx,
		request.StrategyID,
		request.StrategyRevision,
	)
	if err != nil {
		return PreparedRun{}, err
	}
	snapshot := plan.Snapshot()
	config := snapshot.Defaults
	if request.RunConfig != nil {
		if request.DatasetID != "" || request.Symbol != "" || request.StartMS != 0 || request.EndMS != 0 {
			return PreparedRun{}, fmt.Errorf("run_config cannot be combined with legacy data overrides")
		}
		config = *request.RunConfig
	} else {
		if request.DatasetID != "" {
			config.Data.DatasetID = strings.TrimSpace(request.DatasetID)
		}
		if request.Symbol != "" {
			config.Data.Symbols = []string{strings.TrimSpace(request.Symbol)}
		}
		if request.StartMS != 0 {
			config.Data.Start = time.UnixMilli(request.StartMS).UTC()
		}
		if request.EndMS != 0 {
			config.Data.End = time.UnixMilli(request.EndMS).UTC()
		}
	}
	if err := strategy.ValidateRunConfig(config, snapshot.Logic); err != nil {
		return PreparedRun{}, err
	}
	// Copy experiment slices and report overlays before crossing the run boundary.
	raw, err := json.Marshal(struct {
		Request Request
		Config  strategy.RunConfig
	}{request, config})
	if err != nil {
		return PreparedRun{}, err
	}
	var owned struct {
		Request Request
		Config  strategy.RunConfig
	}
	if err := json.Unmarshal(raw, &owned); err != nil {
		return PreparedRun{}, err
	}
	data, err := s.provider.Load(ctx, owned.Config)
	if err != nil {
		return PreparedRun{}, err
	}
	owned.Request.StrategyRevision = version.Revision
	owned.Request.RunConfig = &owned.Config
	owned.Request.DatasetID, owned.Request.Symbol = "", ""
	owned.Request.StartMS, owned.Request.EndMS = 0, 0
	return PreparedRun{Request: owned.Request, Definition: definition, Plan: plan,
		Version: version, Config: owned.Config, Data: data}, nil
}

func (s *Service) Execute(ctx context.Context, prepared PreparedRun, progress func(int, int)) (Response, error) {
	result, err := engine.Run(ctx, prepared.Plan, engine.RunRequest{
		Bars: prepared.Data.Bars, Config: &prepared.Config,
		Report: prepared.Request.Report, OnProgress: progress,
	})
	if err != nil {
		return Response{}, err
	}
	snapshot := prepared.Plan.Snapshot()
	response := BuildResponse(strategy.ComposeV1(snapshot.Logic, prepared.Config), prepared.Version, prepared.Data.Provenance, len(prepared.Data.Bars), result)
	response.RunConfig = prepared.Config
	return response, nil
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
