package backtest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/dataset"
	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestDataSnapshotRejectsInvalidFileAndTimeline(t *testing.T) {
	for _, scenario := range []string{"row_count", "missing", "duplicate", "unclosed", "path_escape", "symlink_escape", "market_type"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			def := testDefinition()
			bars := []market.Bar{testBar(0, 100), testBar(900000, 101), testBar(1800000, 102)}
			switch scenario {
			case "missing":
				bars = bars[:2]
			case "duplicate":
				bars[1].OpenTimeMS = bars[0].OpenTimeMS
			case "unclosed":
				bars[1].State = market.BarStateForming
			}
			writer := dataset.Writer{OutputRoot: root, DatasetID: def.Data.DatasetID, Exchange: "binance", Timeframe: "15m", Overwrite: true}
			files, err := writer.WriteSymbolBars(def.Data.Symbols[0], bars)
			if err != nil {
				t.Fatal(err)
			}
			manifest := dataset.BuildManifest(def.Data.DatasetID, "binance", "perpetual", "15m", def.Data.Symbols, 0, 2700000, nil, files)
			switch scenario {
			case "row_count":
				manifest.Files[0].RowCount++
			case "path_escape":
				manifest.Files[0].Path = "exchange=binance/symbol=BTC-USDT/timeframe=15m/../../../../outside.parquet"
			case "market_type":
				manifest.MarketType = "spot"
			case "symlink_escape":
				file := filepath.Join(root, def.Data.DatasetID, filepath.FromSlash(files[0].Path))
				outside := filepath.Join(root, "outside.parquet")
				raw, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(outside, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, file); err != nil {
					t.Fatal(err)
				}
			}
			if err := dataset.WriteManifest(filepath.Join(root, def.Data.DatasetID), manifest); err != nil {
				t.Fatal(err)
			}
			_, config, err := strategy.AdaptV1(def)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := (FileDataProvider{Root: root}).Load(context.Background(), config); err == nil {
				t.Fatalf("accepted invalid %s", scenario)
			}
		})
	}
}

func TestDataProviderRejectsUnsupportedSelectionWithoutPanic(t *testing.T) {
	_, err := (FileDataProvider{}).Load(context.Background(), strategy.RunConfig{})
	if err == nil || !strings.Contains(err.Error(), "one symbol") {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FileDataProvider{}).Load(ctx, strategy.RunConfig{}); err != context.Canceled {
		t.Fatal(err)
	}
}
