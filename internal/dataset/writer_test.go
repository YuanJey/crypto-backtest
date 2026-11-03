package dataset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/market"
)

func TestWriterWritesPartitionedParquet(t *testing.T) {
	root := t.TempDir()
	writer := Writer{
		OutputRoot: root,
		DatasetID:  "test-dataset",
		Exchange:   "binance",
		Timeframe:  "15m",
		Overwrite:  true,
	}

	entries, err := writer.WriteSymbolBars("BTC-USDT", []market.Bar{testBar(0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries)=%d, want 1", len(entries))
	}

	path := filepath.Join(root, "test-dataset", filepath.FromSlash(entries[0].Path))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected parquet part to exist: %v", err)
	}
	if entries[0].StartMS != 0 || entries[0].EndMS != 900_000 {
		t.Fatalf("unexpected time range: %+v", entries[0])
	}
	if entries[0].SHA256 == "" {
		t.Fatal("expected sha256 checksum")
	}
}

func TestReadBarsFromManifest(t *testing.T) {
	root := t.TempDir()
	writer := Writer{
		OutputRoot: root,
		DatasetID:  "test-dataset",
		Exchange:   "binance",
		Timeframe:  "15m",
		Overwrite:  true,
	}
	bars := []market.Bar{testBar(0), testBar(900_000)}
	entries, err := writer.WriteSymbolBars("BTC-USDT", bars)
	if err != nil {
		t.Fatal(err)
	}
	manifest := BuildManifest("test-dataset", "binance", "spot", "15m", []string{"BTC-USDT"}, 0, 2*900_000, []QualityReport{{
		Symbol:    "BTC-USDT",
		Timeframe: "15m",
		RowCount:  2,
	}}, entries)
	datasetRoot, err := writer.Root()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(datasetRoot, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadBars(root, "test-dataset", "BTC-USDT", "15m", 0, 2*900_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("len(loaded)=%d, want 2", len(loaded))
	}
}

func TestDatasetIDRejectsPathTraversal(t *testing.T) {
	writer := Writer{
		OutputRoot: t.TempDir(),
		DatasetID:  "../outside",
	}
	if _, err := writer.Root(); err == nil {
		t.Fatal("expected writer to reject path traversal dataset id")
	}
	if _, _, err := ReadManifest(t.TempDir(), "../outside"); err == nil {
		t.Fatal("expected reader to reject path traversal dataset id")
	}
}
