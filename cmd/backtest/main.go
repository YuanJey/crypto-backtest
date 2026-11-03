package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/backtest"
	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/engine"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("backtest", flag.ContinueOnError)
	strategyID := flags.String("strategy-id", "", "strategy id to load from strategies directory")
	strategyRevision := flags.Int("strategy-revision", 0, "immutable strategy revision; defaults to latest")
	strategiesDir := flags.String("strategies-dir", "strategies", "directory containing strategy JSON files")
	dbPath := flags.String("db", "", "SQLite database path; when set, load strategy from SQLite instead of strategies directory")
	runBacktest := flags.Bool("run", false, "run the compiled strategy against its dataset")
	dataRoot := flags.String("data-root", "data/market", "dataset output root")
	reportValue := flags.String("report", "summary", "comma-separated report sections: summary,equity,orders,fills,indicators")
	reportIndicators := flags.String("report-indicator", "", "comma-separated indicator fields, e.g. ema_fast_15m.value,rsi_14_4h.value")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *strategyID == "" {
		return fmt.Errorf("-strategy-id is required")
	}

	registry := indicator.MustBuiltinRegistry()
	repository, closeRepository, err := openRepository(context.Background(), *dbPath, *strategiesDir)
	if err != nil {
		return err
	}
	defer closeRepository()

	runner := backtest.NewService(repository, registry, *dataRoot)
	definition, plan, version, err := runner.CompileStrategyRevision(
		context.Background(),
		*strategyID,
		*strategyRevision,
	)
	if err != nil {
		return err
	}

	reportSpec, err := parseReportSpec(*reportValue, *reportIndicators, plan)
	if err != nil {
		return err
	}
	if !*runBacktest {
		payload := struct {
			Status          string                    `json:"status"`
			Strategy        strategy.Summary          `json:"strategy"`
			StrategyVersion strategy.RevisionMetadata `json:"strategy_version"`
		}{
			Status:          "strategy_compiled",
			Strategy:        strategy.Summarize(definition),
			StrategyVersion: version,
		}
		return printJSON(payload)
	}

	response, err := runner.Run(context.Background(), backtest.Request{
		StrategyID:       *strategyID,
		StrategyRevision: *strategyRevision,
		Report:           reportSpec,
	})
	if err != nil {
		return err
	}
	return printJSON(response)
}

func openRepository(ctx context.Context, dbPath string, strategiesDir string) (strategy.Repository, func(), error) {
	if strings.TrimSpace(dbPath) == "" {
		return strategy.FileRepository{Dir: strategiesDir}, func() {}, nil
	}
	repository, err := strategy.OpenSQLiteRepository(ctx, dbPath)
	if err != nil {
		return nil, nil, err
	}
	return repository, func() { _ = repository.Close() }, nil
}

func parseReportSpec(value string, indicatorValue string, plan compiler.Plan) (engine.ReportSpec, error) {
	spec := engine.ReportSpec{}
	includeIndicators := false
	for _, part := range strings.Split(value, ",") {
		section := strings.TrimSpace(part)
		if section == "" {
			continue
		}
		switch section {
		case "summary":
			spec.Metrics = engine.DefaultReportSpec().Metrics
		case "equity":
			spec.EquityCurve = true
		case "orders":
			spec.Orders = true
		case "fills":
			spec.Fills = true
		case "indicators":
			includeIndicators = true
		default:
			return engine.ReportSpec{}, fmt.Errorf("unknown report section %q", section)
		}
	}
	overlays, err := parseIndicatorOverlays(indicatorValue)
	if err != nil {
		return engine.ReportSpec{}, err
	}
	if includeIndicators && len(overlays) == 0 {
		overlays = make([]engine.IndicatorOverlay, 0, len(plan.Indicators))
		for _, item := range plan.Indicators {
			overlays = append(overlays, engine.IndicatorOverlay{ID: item.ID})
		}
	}
	spec.Indicators = overlays
	return spec, nil
}

func parseIndicatorOverlays(value string) ([]engine.IndicatorOverlay, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	byID := map[string]map[string]struct{}{}
	for _, part := range strings.Split(value, ",") {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		id, field, ok := strings.Cut(item, ".")
		if !ok || id == "" || field == "" {
			return nil, fmt.Errorf("invalid report indicator %q, expected indicator.field", item)
		}
		if byID[id] == nil {
			byID[id] = map[string]struct{}{}
		}
		byID[id][field] = struct{}{}
	}
	overlays := make([]engine.IndicatorOverlay, 0, len(byID))
	for id, fieldsMap := range byID {
		fields := make([]string, 0, len(fieldsMap))
		for field := range fieldsMap {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		overlays = append(overlays, engine.IndicatorOverlay{ID: id, Fields: fields})
	}
	sort.Slice(overlays, func(i, j int) bool {
		return overlays[i].ID < overlays[j].ID
	})
	return overlays, nil
}

func printJSON(value interface{}) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
