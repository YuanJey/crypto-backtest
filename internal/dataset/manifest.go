package dataset

type FileEntry struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	RowCount int    `json:"row_count"`
	StartMS  int64  `json:"start_ms"`
	EndMS    int64  `json:"end_ms"`
}

type Manifest struct {
	SchemaVersion int                    `json:"schema_version"`
	DatasetID     string                 `json:"dataset_id"`
	Exchange      string                 `json:"exchange"`
	MarketType    string                 `json:"market_type"`
	Symbols       []string               `json:"symbols"`
	Timeframes    []string               `json:"timeframes"`
	StartMS       int64                  `json:"start_ms"`
	EndMS         int64                  `json:"end_ms"`
	RowCount      int                    `json:"row_count"`
	MissingBars   int64                  `json:"missing_bars"`
	GeneratedAt   string                 `json:"generated_at"`
	MarketRules   map[string]interface{} `json:"market_rules"`
	Files         []FileEntry            `json:"files"`
}
