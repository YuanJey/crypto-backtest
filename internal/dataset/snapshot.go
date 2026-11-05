package dataset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/parquet-go/parquet-go"
)

type Snapshot struct {
	Manifest       Manifest
	ManifestSHA256 string
	Files          []FileEntry
	Bars           []market.Bar
	BarsSHA256     string
}

// ReadSnapshot parses and hashes the same manifest bytes, and hashes and
// decodes the same parquet bytes. A later replacement cannot change this run.
func ReadSnapshot(ctx context.Context, outputRoot, datasetID, symbol, timeframe string, startMS, endMS int64) (Snapshot, error) {
	if err := ValidateDatasetID(datasetID); err != nil {
		return Snapshot{}, err
	}
	root, err := filepath.Abs(filepath.Join(outputRoot, datasetID))
	if err != nil {
		return Snapshot{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Snapshot{}, err
	}
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
	if err := json.Unmarshal(raw, &snapshot.Manifest); err != nil {
		return Snapshot{}, err
	}
	if snapshot.Manifest.DatasetID != datasetID {
		return Snapshot{}, fmt.Errorf("manifest dataset_id mismatch")
	}
	prefix := "exchange=" + snapshot.Manifest.Exchange + "/symbol=" + symbol + "/timeframe=" + timeframe + "/"
	seen := map[string]bool{}
	for _, file := range snapshot.Manifest.Files {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		if !strings.HasPrefix(file.Path, prefix) {
			continue
		}
		if seen[file.Path] {
			return Snapshot{}, fmt.Errorf("duplicate manifest file %q", file.Path)
		}
		seen[file.Path] = true
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Snapshot{}, fmt.Errorf("invalid manifest path %q", file.Path)
		}
		path, err := filepath.EvalSymlinks(filepath.Join(root, clean))
		if err != nil {
			return Snapshot{}, err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return Snapshot{}, fmt.Errorf("manifest path escapes dataset root: %q", file.Path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return Snapshot{}, err
		}
		if file.SHA256 == "" || fmt.Sprintf("%x", sha256.Sum256(raw)) != file.SHA256 {
			return Snapshot{}, fmt.Errorf("dataset file hash mismatch: %s", file.Path)
		}
		rows, err := parquet.Read[market.Bar](bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return Snapshot{}, err
		}
		if len(rows) != file.RowCount {
			return Snapshot{}, fmt.Errorf("dataset file row_count mismatch: %s", file.Path)
		}
		snapshot.Files = append(snapshot.Files, file)
		for _, row := range rows {
			if row.OpenTimeMS >= startMS && row.OpenTimeMS < endMS {
				snapshot.Bars = append(snapshot.Bars, row)
			}
		}
	}
	sort.Slice(snapshot.Bars, func(i, j int) bool { return snapshot.Bars[i].OpenTimeMS < snapshot.Bars[j].OpenTimeMS })
	snapshot.BarsSHA256, err = BarsSHA256(snapshot.Bars)
	return snapshot, err
}

func BarsSHA256(bars []market.Bar) (string, error) {
	raw, err := json.Marshal(bars)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
