package strategy

import (
	"fmt"
	"math"

	"github.com/YuanJey/crypto-backtest/internal/indicator"
)

// DistanceAt returns the percentage, price amount or ATR multiplier at the
// action's addition index. A linear increment uses the same unit as Distance.
func (spec AdditionTriggerSpec) DistanceAt(index int) (float64, bool) {
	if index < 1 {
		return 0, false
	}
	value := spec.Distance.Value
	if spec.Spacing != nil {
		switch spec.Spacing.Mode {
		case AdditionSpacingFixed:
		case AdditionSpacingLinear:
			value += float64(index-1) * spec.Spacing.Value
		case AdditionSpacingGeometric:
			value *= math.Pow(spec.Spacing.Value, float64(index-1))
		default:
			return 0, false
		}
	}
	return value, isFinite(value) && value > 0 &&
		(spec.Distance.Type != ExitDistancePercent || value < 1)
}

func validateAdditionTrigger(spec AdditionTriggerSpec, index int, outputs indicatorOutputs) error {
	switch spec.Anchor.Type {
	case ExitAnchorAverageEntry, ExitAnchorFirstEntry, ExitAnchorLastEntry:
	default:
		return fmt.Errorf("anchor must be avg_entry_price, first_entry_price or last_entry_price")
	}
	if spec.Anchor.Indicator != "" || spec.Anchor.Field != "" {
		return fmt.Errorf("position anchor cannot configure an indicator")
	}
	if spec.Direction != AdditionAdverse && spec.Direction != AdditionFavorable {
		return fmt.Errorf("direction must be adverse or favorable")
	}
	switch spec.TriggerPrice {
	case ExitTriggerClose, ExitTriggerHigh, ExitTriggerLow:
	default:
		return fmt.Errorf("trigger_price must be close, high or low")
	}
	if !isFinite(spec.Distance.Value) || spec.Distance.Value <= 0 {
		return fmt.Errorf("distance.value must be finite and > 0")
	}
	switch spec.Distance.Type {
	case ExitDistancePercent, ExitDistanceFixedAmount:
		if spec.Distance.Indicator != "" || spec.Distance.Field != "" || spec.Distance.Sampling != "" {
			return fmt.Errorf("non-ATR distance cannot configure indicator sampling")
		}
	case ExitDistanceATRMultiple:
		fields, exists := outputs[spec.Distance.Indicator]
		output, ok := fields[spec.Distance.Field]
		if !exists || !ok || output.Semantic != indicator.OutputSemanticVolatility {
			return fmt.Errorf("ATR distance requires a volatility output from a known indicator")
		}
		switch spec.Distance.Sampling {
		case ExitSamplingEntry, ExitSamplingEachEntry, ExitSamplingLive:
		default:
			return fmt.Errorf("ATR sampling must be entry, each_entry or live")
		}
	default:
		return fmt.Errorf("distance.type must be percent, fixed_amount or atr_multiple")
	}
	if spec.Spacing != nil {
		if !isFinite(spec.Spacing.Value) {
			return fmt.Errorf("spacing.value must be finite")
		}
		switch spec.Spacing.Mode {
		case AdditionSpacingFixed:
			if spec.Spacing.Value != 0 {
				return fmt.Errorf("fixed spacing cannot configure a value")
			}
		case AdditionSpacingLinear:
			if spec.Spacing.Value <= 0 {
				return fmt.Errorf("linear spacing increment must be > 0")
			}
		case AdditionSpacingGeometric:
			if spec.Spacing.Value < 1 {
				return fmt.Errorf("geometric spacing multiplier must be >= 1")
			}
		default:
			return fmt.Errorf("spacing.mode must be fixed, linear or geometric")
		}
	}
	if _, ok := spec.DistanceAt(index); !ok {
		return fmt.Errorf("effective distance must be finite and > 0; percent distance must be < 1")
	}
	return nil
}
