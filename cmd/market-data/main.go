package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/datafeed/binance"
	"github.com/YuanJey/crypto-backtest/internal/dataset"
	"github.com/YuanJey/crypto-backtest/internal/market"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: market-data download [flags]")
	}
	switch args[0] {
	case "download":
		return runDownload(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runDownload(args []string) error {
	flags := flag.NewFlagSet("download", flag.ContinueOnError)
	exchange := flags.String("exchange", "binance", "exchange name")
	marketType := flags.String("market-type", "spot", "market type: spot or perpetual")
	symbolsValue := flags.String("symbols", "", "comma-separated symbols, e.g. BTC-USDT,ETH-USDT")
	timeframe := flags.String("timeframe", market.BaseTimeframe, "base timeframe; only 15m is accepted")
	startValue := flags.String("start", "", "inclusive UTC start, e.g. 2025-01-01T00:00:00Z")
	endValue := flags.String("end", "", "exclusive UTC end, e.g. 2025-02-01T00:00:00Z")
	outputRoot := flags.String("output-root", "data/market", "dataset output root")
	datasetID := flags.String("dataset-id", "", "dataset id; generated when empty")
	noOverwrite := flags.Bool("no-overwrite", false, "refuse to overwrite existing parquet parts")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *exchange != "binance" {
		return fmt.Errorf("unsupported exchange %q", *exchange)
	}
	startMS, err := market.ParseUTCMS(*startValue)
	if err != nil {
		return err
	}
	endMS, err := market.ParseUTCMS(*endValue)
	if err != nil {
		return err
	}
	symbols, err := parseSymbols(*symbolsValue)
	if err != nil {
		return err
	}
	id := *datasetID
	if id == "" {
		id = defaultDatasetID(*exchange, *marketType, *timeframe, *startValue, *endValue)
	}

	client, err := binance.NewClient(*marketType)
	if err != nil {
		return err
	}
	result, err := dataset.Build(context.Background(), client, dataset.BuildRequest{
		Exchange:   *exchange,
		MarketType: *marketType,
		Symbols:    symbols,
		Timeframe:  *timeframe,
		StartMS:    startMS,
		EndMS:      endMS,
		OutputRoot: *outputRoot,
		DatasetID:  id,
		Overwrite:  !*noOverwrite,
	})
	if err != nil {
		return err
	}

	summary := struct {
		DatasetRoot string `json:"dataset_root"`
		DatasetID   string `json:"dataset_id"`
		RowCount    int    `json:"row_count"`
		MissingBars int64  `json:"missing_bars"`
		Files       int    `json:"files"`
	}{
		DatasetRoot: result.DatasetRoot,
		DatasetID:   result.Manifest.DatasetID,
		RowCount:    result.Manifest.RowCount,
		MissingBars: result.Manifest.MissingBars,
		Files:       len(result.Manifest.Files),
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func parseSymbols(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("--symbols is required")
	}
	parts := strings.Split(value, ",")
	symbols := make([]string, 0, len(parts))
	for _, part := range parts {
		symbol, err := market.NormalizeSymbol(part)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, symbol)
	}
	return symbols, nil
}

func defaultDatasetID(exchange, marketType, timeframe, start, end string) string {
	clean := func(value string) string {
		replacer := strings.NewReplacer(":", "", "-", "", "T", "", "Z", "")
		return replacer.Replace(value)
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s", exchange, marketType, timeframe, clean(start), clean(end))
}
