package backtest

import (
	"context"
	"fmt"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/dataset"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type DataFile = dataset.FileEntry

type DataSnapshot struct {
	Bars       []market.Bar   `json:"bars"`
	Provenance DataProvenance `json:"provenance"`
}

type DataProvider interface {
	Load(context.Context, strategy.RunConfig) (DataSnapshot, error)
}

type FileDataProvider struct{ Root string }

func (p FileDataProvider) Load(ctx context.Context, config strategy.RunConfig) (DataSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return DataSnapshot{}, err
	}
	data := config.Data
	if len(data.Symbols) != 1 {
		return DataSnapshot{}, fmt.Errorf("data provider requires exactly one symbol")
	}
	symbol, err := market.NormalizeSymbol(data.Symbols[0])
	if err != nil {
		return DataSnapshot{}, err
	}
	start, end := data.Start.UTC().UnixMilli(), data.End.UTC().UnixMilli()
	tf, err := market.ParseTimeframeMS(data.Timeframe)
	if err != nil {
		return DataSnapshot{}, err
	}
	provenance := DataProvenance{DatasetID: data.DatasetID, Symbol: symbol,
		Timeframe: data.Timeframe, StartMS: start, EndMS: end}
	var bars []market.Bar
	if data.DatasetID == "__sample__" || strings.EqualFold(data.DatasetID, "sample") {
		bars = market.GenerateSampleBars(symbol, start, end)
		provenance.DatasetID, provenance.Synthetic = "__sample__", true
	} else {
		snapshot, err := dataset.ReadSnapshot(ctx, p.Root, data.DatasetID, symbol, data.Timeframe, start, end)
		if err != nil {
			return DataSnapshot{}, fmt.Errorf("read dataset %q: %w", data.DatasetID, err)
		}
		if snapshot.Manifest.MarketType != string(config.Account.MarketType) {
			return DataSnapshot{}, fmt.Errorf("dataset market_type does not match run account")
		}
		if start < snapshot.Manifest.StartMS || end > snapshot.Manifest.EndMS {
			return DataSnapshot{}, fmt.Errorf("backtest range is outside dataset range")
		}
		bars = snapshot.Bars
		provenance.ManifestSHA256, provenance.Files = snapshot.ManifestSHA256, snapshot.Files
	}
	if int64(len(bars)) != (end-start)/tf {
		return DataSnapshot{}, fmt.Errorf("backtest range requires %d bars; found %d", (end-start)/tf, len(bars))
	}
	for i, bar := range bars {
		if bar.OpenTimeMS != start+int64(i)*tf || !bar.IsClosed() {
			return DataSnapshot{}, fmt.Errorf("missing, duplicate or unclosed bar at index %d", i)
		}
	}
	provenance.BarsSHA256, err = dataset.BarsSHA256(bars)
	if err != nil {
		return DataSnapshot{}, err
	}
	return DataSnapshot{Bars: bars, Provenance: provenance}, nil
}
