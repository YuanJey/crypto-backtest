package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/YuanJey/crypto-backtest/internal/market"
)

type Writer struct {
	OutputRoot string
	DatasetID  string
	Exchange   string
	Timeframe  string
	Overwrite  bool
}

func (w Writer) Root() (string, error) {
	if w.OutputRoot == "" {
		return "", fmt.Errorf("output root is required")
	}
	if err := ValidateDatasetID(w.DatasetID); err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(w.OutputRoot, w.DatasetID))
}

func (w Writer) WriteSymbolBars(symbol string, bars []market.Bar) ([]FileEntry, error) {
	root, err := w.Root()
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, nil
	}
	timeframeMS, err := market.ParseTimeframeMS(w.Timeframe)
	if err != nil {
		return nil, err
	}

	groups := groupByMonth(bars)
	entries := make([]FileEntry, 0, len(groups))
	for _, group := range groups {
		partDir := filepath.Join(
			root,
			"exchange="+w.Exchange,
			"symbol="+symbol,
			"timeframe="+w.Timeframe,
			fmt.Sprintf("year=%04d", group.Year),
			fmt.Sprintf("month=%02d", group.Month),
		)
		if err := os.MkdirAll(partDir, 0o755); err != nil {
			return nil, err
		}

		partPath := filepath.Join(partDir, "part-000.parquet")
		if _, err := os.Stat(partPath); err == nil && !w.Overwrite {
			return nil, fmt.Errorf("refusing to overwrite existing file: %s", partPath)
		}
		if err := parquet.WriteFile(partPath, group.Bars); err != nil {
			return nil, err
		}

		checksum, err := sha256File(partPath)
		if err != nil {
			return nil, err
		}
		relPath, err := filepath.Rel(root, partPath)
		if err != nil {
			return nil, err
		}
		entries = append(entries, FileEntry{
			Path:     filepath.ToSlash(relPath),
			SHA256:   checksum,
			RowCount: len(group.Bars),
			StartMS:  group.Bars[0].OpenTimeMS,
			EndMS:    group.Bars[len(group.Bars)-1].OpenTimeMS + timeframeMS,
		})
	}
	return entries, nil
}

func WriteManifest(root string, manifest Manifest) error {
	return writeJSON(filepath.Join(root, "manifest.json"), manifest)
}

func WriteGapReport(root string, reports []QualityReport) error {
	return writeJSON(filepath.Join(root, "gap_report.json"), struct {
		Reports []QualityReport `json:"reports"`
	}{Reports: reports})
}

func BuildManifest(datasetID, exchange, marketType, timeframe string, symbols []string, startMS, endMS int64, reports []QualityReport, files []FileEntry) Manifest {
	manifest := Manifest{
		SchemaVersion: 1,
		DatasetID:     datasetID,
		Exchange:      exchange,
		MarketType:    marketType,
		Symbols:       symbols,
		Timeframes:    []string{timeframe},
		StartMS:       startMS,
		EndMS:         endMS,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		MarketRules:   map[string]interface{}{},
		Files:         files,
	}
	for _, symbol := range symbols {
		manifest.MarketRules[symbol] = map[string]interface{}{}
	}
	for _, report := range reports {
		manifest.RowCount += report.RowCount
		manifest.MissingBars += report.MissingBars
	}
	return manifest
}

type monthGroup struct {
	Year  int
	Month int
	Bars  []market.Bar
}

func groupByMonth(bars []market.Bar) []monthGroup {
	groups := make([]monthGroup, 0)
	groupIndex := map[string]int{}
	for _, bar := range bars {
		t := time.UnixMilli(bar.OpenTimeMS).UTC()
		key := fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
		idx, ok := groupIndex[key]
		if !ok {
			idx = len(groups)
			groupIndex[key] = idx
			groups = append(groups, monthGroup{Year: t.Year(), Month: int(t.Month())})
		}
		groups[idx].Bars = append(groups[idx].Bars, bar)
	}
	return groups
}

func writeJSON(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
