package engine

import (
	"fmt"
	"math"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
)

func normalizeReportSpec(spec ReportSpec) ReportSpec {
	if spec.EquityCurve || spec.Orders || spec.Fills || spec.Ledger || len(spec.Metrics) > 0 || len(spec.Indicators) > 0 {
		return spec
	}
	return DefaultReportSpec()
}

func validateReportSpec(spec ReportSpec, plan compiler.ExecutionPlan) error {
	validMetrics := map[MetricType]struct{}{
		MetricFinalEquity:         {},
		MetricTotalReturnPct:      {},
		MetricMaxDrawdownPct:      {},
		MetricOrderCount:          {},
		MetricFillCount:           {},
		MetricMaxPositionLeverage: {},
	}
	for _, metric := range spec.Metrics {
		if _, ok := validMetrics[metric]; !ok {
			return fmt.Errorf("unknown report metric %q", metric)
		}
	}

	outputs := make(map[string]map[string]struct{}, len(plan.Indicators))
	for _, item := range plan.Indicators {
		fields := make(map[string]struct{}, len(item.Outputs))
		for _, field := range item.Outputs {
			fields[field] = struct{}{}
		}
		outputs[item.ID] = fields
	}
	for _, overlay := range spec.Indicators {
		fields, ok := outputs[overlay.ID]
		if !ok {
			return fmt.Errorf("report indicator %q does not exist", overlay.ID)
		}
		for _, field := range overlay.Fields {
			if _, ok := fields[field]; !ok {
				return fmt.Errorf("report indicator %q has no field %q", overlay.ID, field)
			}
		}
	}
	return nil
}

func applyReportSpec(result *Result, spec ReportSpec, indicatorSeries map[string]map[string][]float64, initialCash float64) error {
	spec = normalizeReportSpec(spec)
	if len(spec.Metrics) > 0 {
		result.Metrics = buildMetrics(*result, spec.Metrics, initialCash)
	}
	if len(spec.Indicators) > 0 {
		indicators, err := buildIndicatorReports(indicatorSeries, spec.Indicators)
		if err != nil {
			return err
		}
		result.Indicators = indicators
	}
	if !spec.EquityCurve {
		result.EquityCurve = nil
	}
	if !spec.Orders {
		result.Orders = nil
	}
	if !spec.Fills {
		result.Fills = nil
	}
	if !spec.Ledger {
		result.Ledger = nil
	}
	return nil
}

func buildMetrics(result Result, requested []MetricType, initialCash float64) map[MetricType]float64 {
	metrics := make(map[MetricType]float64, len(requested))
	for _, metric := range requested {
		switch metric {
		case MetricFinalEquity:
			metrics[metric] = result.FinalEquity
		case MetricTotalReturnPct:
			if initialCash > 0 {
				metrics[metric] = (result.FinalEquity - initialCash) / initialCash
			}
		case MetricMaxDrawdownPct:
			metrics[metric] = maxDrawdown(result.EquityCurve)
		case MetricOrderCount:
			metrics[metric] = float64(len(result.Orders))
		case MetricFillCount:
			metrics[metric] = float64(len(result.Fills))
		case MetricMaxPositionLeverage:
			metrics[metric] = maxPositionLeverage(result.EquityCurve)
		}
	}
	return metrics
}

func maxPositionLeverage(points []EquityPoint) float64 {
	var maximum float64
	for _, point := range points {
		if point.PositionLeverage > maximum {
			maximum = point.PositionLeverage
		}
	}
	return maximum
}

func maxDrawdown(points []EquityPoint) float64 {
	var peak float64
	var maxDD float64
	for _, point := range points {
		if point.Equity > peak {
			peak = point.Equity
		}
		if peak <= 0 {
			continue
		}
		drawdown := (peak - point.Equity) / peak
		if drawdown > maxDD {
			maxDD = drawdown
		}
	}
	return maxDD
}

func buildIndicatorReports(series map[string]map[string][]float64, overlays []IndicatorOverlay) (map[string]IndicatorReport, error) {
	reports := make(map[string]IndicatorReport, len(overlays))
	for _, overlay := range overlays {
		fields, ok := series[overlay.ID]
		if !ok {
			return nil, fmt.Errorf("report indicator %q does not exist", overlay.ID)
		}
		selected := map[string][]*float64{}
		if len(overlay.Fields) == 0 {
			for field, values := range fields {
				selected[field] = nullableSeries(values)
			}
		} else {
			for _, field := range overlay.Fields {
				values, ok := fields[field]
				if !ok {
					return nil, fmt.Errorf("report indicator %q has no field %q", overlay.ID, field)
				}
				selected[field] = nullableSeries(values)
			}
		}
		reports[overlay.ID] = IndicatorReport{Fields: selected}
	}
	return reports, nil
}

func nullableSeries(values []float64) []*float64 {
	result := make([]*float64, len(values))
	for index, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		valueCopy := value
		result[index] = &valueCopy
	}
	return result
}
