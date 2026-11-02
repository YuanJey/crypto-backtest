package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/bytedance/crypto-backtest/internal/compiler"
	"github.com/bytedance/crypto-backtest/internal/dataset"
	"github.com/bytedance/crypto-backtest/internal/engine"
	"github.com/bytedance/crypto-backtest/internal/indicator"
	"github.com/bytedance/crypto-backtest/internal/strategy"
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
	strategiesDir := flags.String("strategies-dir", "strategies", "directory containing strategy JSON files")
	runBacktest := flags.Bool("run", false, "run the compiled strategy against its dataset")
	dataRoot := flags.String("data-root", "data/market", "dataset output root")
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
	repository := strategy.FileRepository{Dir: *strategiesDir}
	definition, err := repository.LoadByID(context.Background(), *strategyID)
	if err != nil {
		return err
	}
	if err := strategy.Validate(definition, registry); err != nil {
		return err
	}
	plan, err := compiler.Compile(definition, registry)
	if err != nil {
		return err
	}

	if !*runBacktest {
		payload := struct {
			Status   string           `json:"status"`
			Strategy strategy.Summary `json:"strategy"`
		}{
			Status:   "strategy_compiled",
			Strategy: strategy.Summarize(definition),
		}
		return printJSON(payload)
	}

	if len(definition.Data.Symbols) != 1 {
		return fmt.Errorf("initial engine CLI supports exactly one symbol")
	}
	bars, err := dataset.ReadBars(
		*dataRoot,
		definition.Data.DatasetID,
		definition.Data.Symbols[0],
		definition.Data.Timeframe,
		definition.Data.Start.UTC().UnixMilli(),
		definition.Data.End.UTC().UnixMilli(),
	)
	if err != nil {
		return err
	}
	result, err := engine.Run(context.Background(), plan, engine.RunRequest{Bars: bars})
	if err != nil {
		return err
	}
	payload := struct {
		Status      string           `json:"status"`
		Strategy    strategy.Summary `json:"strategy"`
		BarCount    int              `json:"bar_count"`
		OrderCount  int              `json:"order_count"`
		FillCount   int              `json:"fill_count"`
		FinalEquity float64          `json:"final_equity"`
	}{
		Status:      "backtest_completed",
		Strategy:    strategy.Summarize(definition),
		BarCount:    len(bars),
		OrderCount:  len(result.Orders),
		FillCount:   len(result.Fills),
		FinalEquity: result.FinalEquity,
	}
	return printJSON(payload)
}

func printJSON(value interface{}) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
