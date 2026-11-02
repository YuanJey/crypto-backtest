package dataset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/parquet-go/parquet-go"

	"github.com/bytedance/crypto-backtest/internal/market"
)

func ReadManifest(outputRoot string, datasetID string) (Manifest, string, error) {
	root, err := filepath.Abs(filepath.Join(outputRoot, datasetID))
	if err != nil {
		return Manifest{}, "", err
	}
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return Manifest{}, "", err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, "", err
	}
	if manifest.DatasetID != datasetID {
		return Manifest{}, "", fmt.Errorf("manifest dataset_id %q does not match %q", manifest.DatasetID, datasetID)
	}
	return manifest, root, nil
}

func ReadBars(outputRoot string, datasetID string, symbol string, timeframe string, startMS int64, endMS int64) ([]market.Bar, error) {
	manifest, root, err := ReadManifest(outputRoot, datasetID)
	if err != nil {
		return nil, err
	}
	partPrefix := "exchange=" + manifest.Exchange + "/symbol=" + symbol + "/timeframe=" + timeframe + "/"
	bars := make([]market.Bar, 0)
	for _, file := range manifest.Files {
		if !strings.HasPrefix(file.Path, partPrefix) {
			continue
		}
		rows, err := parquet.ReadFile[market.Bar](filepath.Join(root, filepath.FromSlash(file.Path)))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.OpenTimeMS >= startMS && row.OpenTimeMS < endMS {
				bars = append(bars, row)
			}
		}
	}
	sort.Slice(bars, func(i, j int) bool {
		return bars[i].OpenTimeMS < bars[j].OpenTimeMS
	})
	return bars, nil
}
