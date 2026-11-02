package dataset

import (
	"fmt"
	"math"
	"sort"

	"github.com/YuanJey/crypto-backtest/internal/market"
)

type CleanOptions struct {
	FailOnInvalidRows bool
}

func DefaultCleanOptions() CleanOptions {
	return CleanOptions{FailOnInvalidRows: true}
}

func CleanBars(bars []market.Bar, symbol, timeframe string, startMS, endMS int64, options CleanOptions) ([]market.Bar, QualityReport, error) {
	timeframeMS, err := market.ParseTimeframeMS(timeframe)
	if err != nil {
		return nil, QualityReport{}, err
	}
	if err := market.RequireAlignedRange(startMS, endMS, timeframeMS); err != nil {
		return nil, QualityReport{}, err
	}

	report := QualityReport{Symbol: symbol, Timeframe: timeframe}
	if len(bars) == 0 {
		report.Gaps = findGaps(nil, symbol, timeframe, startMS, endMS, timeframeMS)
		report.MissingBars = sumMissing(report.Gaps)
		return nil, report, nil
	}

	byOpenTime := make(map[int64]market.Bar, len(bars))
	for _, bar := range bars {
		if bar.OpenTimeMS < startMS || bar.OpenTimeMS >= endMS {
			continue
		}
		bar = bar.WithDefaultClosedState()
		if _, exists := byOpenTime[bar.OpenTimeMS]; exists {
			report.DuplicateRows++
		}
		byOpenTime[bar.OpenTimeMS] = bar
	}

	cleaned := make([]market.Bar, 0, len(byOpenTime))
	for _, bar := range byOpenTime {
		cleaned = append(cleaned, bar)
	}
	sort.Slice(cleaned, func(i, j int) bool {
		return cleaned[i].OpenTimeMS < cleaned[j].OpenTimeMS
	})

	valid := cleaned[:0]
	for _, bar := range cleaned {
		code, message := validateBar(bar, timeframeMS)
		if code == "" {
			valid = append(valid, bar)
			continue
		}
		addIssue(&report, code, message)
		if options.FailOnInvalidRows {
			return nil, report, fmt.Errorf("%s: %s at open_time_ms=%d", code, message, bar.OpenTimeMS)
		}
	}
	cleaned = valid

	observed := make([]int64, 0, len(cleaned))
	for _, bar := range cleaned {
		observed = append(observed, bar.OpenTimeMS)
	}
	report.RowCount = len(cleaned)
	report.Gaps = findGaps(observed, symbol, timeframe, startMS, endMS, timeframeMS)
	report.MissingBars = sumMissing(report.Gaps)
	return cleaned, report, nil
}

func validateBar(bar market.Bar, timeframeMS int64) (string, string) {
	if bar.OpenTimeMS%timeframeMS != 0 {
		return "misaligned_timestamp", "bar open time is not aligned to timeframe boundary"
	}
	if bar.State != market.BarStateClosed {
		return "unclosed_bar", "historical dataset bars must be closed"
	}
	values := []float64{bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.QuoteVolume}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "invalid_numeric", "bar contains NaN or infinity"
		}
	}
	if bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 {
		return "invalid_ohlc", "OHLC prices must be positive"
	}
	if bar.Volume < 0 || bar.QuoteVolume < 0 || bar.TradeCount < 0 {
		return "invalid_volume", "volume and trade_count must be non-negative"
	}
	if bar.High < max(bar.Open, bar.Close) || bar.Low > min(bar.Open, bar.Close) || bar.High < bar.Low {
		return "invalid_ohlc", "OHLC relationship is invalid"
	}
	return "", ""
}

func findGaps(observed []int64, symbol, timeframe string, startMS, endMS, timeframeMS int64) []Gap {
	gaps := make([]Gap, 0)
	expected := startMS
	for _, openTimeMS := range observed {
		if openTimeMS < expected {
			continue
		}
		if openTimeMS > expected {
			gaps = append(gaps, Gap{
				Symbol:      symbol,
				Timeframe:   timeframe,
				StartMS:     expected,
				EndMS:       openTimeMS,
				MissingBars: (openTimeMS - expected) / timeframeMS,
			})
		}
		expected = openTimeMS + timeframeMS
	}
	if expected < endMS {
		gaps = append(gaps, Gap{
			Symbol:      symbol,
			Timeframe:   timeframe,
			StartMS:     expected,
			EndMS:       endMS,
			MissingBars: (endMS - expected) / timeframeMS,
		})
	}
	return gaps
}

func addIssue(report *QualityReport, code, message string) {
	for i := range report.Issues {
		if report.Issues[i].Code == code && report.Issues[i].Message == message {
			report.Issues[i].Count++
			return
		}
	}
	report.Issues = append(report.Issues, QualityIssue{Code: code, Message: message, Count: 1})
}

func sumMissing(gaps []Gap) int64 {
	var total int64
	for _, gap := range gaps {
		total += gap.MissingBars
	}
	return total
}
