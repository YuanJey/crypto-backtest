package dataset

import (
	"context"
	"fmt"
	"os"

	"github.com/bytedance/crypto-backtest/internal/datafeed"
	"github.com/bytedance/crypto-backtest/internal/market"
)

type BuildRequest struct {
	Exchange   string
	MarketType string
	Symbols    []string
	Timeframe  string
	StartMS    int64
	EndMS      int64
	OutputRoot string
	DatasetID  string
	Overwrite  bool
}

type BuildResult struct {
	DatasetRoot string          `json:"dataset_root"`
	Manifest    Manifest        `json:"manifest"`
	Reports     []QualityReport `json:"reports"`
}

func Build(ctx context.Context, client datafeed.Client, request BuildRequest) (BuildResult, error) {
	if err := market.RequireBaseTimeframe(request.Timeframe); err != nil {
		return BuildResult{}, err
	}
	timeframeMS, err := market.ParseTimeframeMS(request.Timeframe)
	if err != nil {
		return BuildResult{}, err
	}
	if err := market.RequireAlignedRange(request.StartMS, request.EndMS, timeframeMS); err != nil {
		return BuildResult{}, err
	}
	if len(request.Symbols) == 0 {
		return BuildResult{}, fmt.Errorf("at least one symbol is required")
	}

	writer := Writer{
		OutputRoot: request.OutputRoot,
		DatasetID:  request.DatasetID,
		Exchange:   request.Exchange,
		Timeframe:  request.Timeframe,
		Overwrite:  request.Overwrite,
	}
	root, err := writer.Root()
	if err != nil {
		return BuildResult{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return BuildResult{}, err
	}

	reports := make([]QualityReport, 0, len(request.Symbols))
	files := make([]FileEntry, 0)
	for _, symbol := range request.Symbols {
		rawBars, err := client.FetchOHLCV(ctx, datafeed.FetchRequest{
			Symbol:    symbol,
			Timeframe: request.Timeframe,
			StartMS:   request.StartMS,
			EndMS:     request.EndMS,
		})
		if err != nil {
			return BuildResult{}, err
		}

		cleaned, report, err := CleanBars(rawBars, symbol, request.Timeframe, request.StartMS, request.EndMS, DefaultCleanOptions())
		if err != nil {
			return BuildResult{}, err
		}
		reports = append(reports, report)

		entries, err := writer.WriteSymbolBars(symbol, cleaned)
		if err != nil {
			return BuildResult{}, err
		}
		files = append(files, entries...)
	}

	manifest := BuildManifest(request.DatasetID, request.Exchange, request.MarketType, request.Timeframe, request.Symbols, request.StartMS, request.EndMS, reports, files)
	if err := WriteManifest(root, manifest); err != nil {
		return BuildResult{}, err
	}
	if err := WriteGapReport(root, reports); err != nil {
		return BuildResult{}, err
	}

	return BuildResult{
		DatasetRoot: root,
		Manifest:    manifest,
		Reports:     reports,
	}, nil
}
