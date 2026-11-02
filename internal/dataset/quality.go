package dataset

type Gap struct {
	Symbol      string `json:"symbol"`
	Timeframe   string `json:"timeframe"`
	StartMS     int64  `json:"start_ms"`
	EndMS       int64  `json:"end_ms"`
	MissingBars int64  `json:"missing_bars"`
}

type QualityIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type QualityReport struct {
	Symbol        string         `json:"symbol"`
	Timeframe     string         `json:"timeframe"`
	RowCount      int            `json:"row_count"`
	DuplicateRows int            `json:"duplicate_rows"`
	MissingBars   int64          `json:"missing_bars"`
	Gaps          []Gap          `json:"gaps"`
	Issues        []QualityIssue `json:"issues"`
}
