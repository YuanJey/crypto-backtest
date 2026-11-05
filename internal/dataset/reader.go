package dataset

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/YuanJey/crypto-backtest/internal/market"
)

func ReadManifest(outputRoot string, datasetID string) (Manifest, string, error) {
	if err := ValidateDatasetID(datasetID); err != nil {
		return Manifest{}, "", err
	}
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

func ManifestSHA256(outputRoot string, datasetID string) (string, error) {
	_, root, err := ReadManifest(outputRoot, datasetID)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func ReadBars(outputRoot string, datasetID string, symbol string, timeframe string, startMS int64, endMS int64) ([]market.Bar, error) {
	snapshot, err := ReadSnapshot(context.Background(), outputRoot, datasetID, symbol, timeframe, startMS, endMS)
	return snapshot.Bars, err
}
