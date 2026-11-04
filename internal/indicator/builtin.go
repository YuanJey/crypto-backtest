package indicator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type PeriodParams struct {
	Period int `json:"period"`
}

type BollParams struct {
	Period int     `json:"period"`
	StdDev float64 `json:"stddev"`
}

type SupertrendParams struct {
	Period     int     `json:"period"`
	Multiplier float64 `json:"multiplier"`
}

func BuiltinRegistry() (*Registry, error) {
	registry := NewRegistry()
	for _, item := range []struct {
		definition Definition
		validator  ParamValidator
		builder    RuntimeBuilder
	}{
		{
			definition: periodIndicator(
				"ema",
				"EMA",
				"trend",
				"value",
				OutputSemanticPrice,
				"moving_average",
			),
			validator: validatePeriodParams,
			builder:   buildEMA,
		},
		{
			definition: periodIndicator(
				"sma",
				"SMA",
				"trend",
				"value",
				OutputSemanticPrice,
				"moving_average",
			),
			validator: validatePeriodParams,
			builder:   buildSMA,
		},
		{
			definition: periodIndicator(
				"rsi",
				"RSI",
				"momentum",
				"value",
				OutputSemanticOscillator,
				"rsi",
			),
			validator: validatePeriodParams,
			builder:   buildRSI,
		},
		{
			definition: Definition{
				Type:     "atr",
				Label:    "ATR",
				Category: "volatility",
				Inputs: []InputDefinition{
					{Name: "high", Type: ValueTypeFloat, Required: true},
					{Name: "low", Type: ValueTypeFloat, Required: true},
					{Name: "close", Type: ValueTypeFloat, Required: true},
				},
				Params: []ParamDefinition{
					{Name: "period", Type: ValueTypeInt, Required: true, Min: json.Number("1")},
				},
				Outputs: []OutputDefinition{
					{Name: "value", Type: ValueTypeFloat, Semantic: OutputSemanticVolatility},
				},
			},
			validator: validatePeriodParams,
			builder:   buildATR,
		},
		{
			definition: Definition{
				Type:     "extreme",
				Label:    "极值",
				Category: "trend",
				Inputs: []InputDefinition{
					{Name: "high", Type: ValueTypeFloat, Required: true},
					{Name: "low", Type: ValueTypeFloat, Required: true},
				},
				Params: []ParamDefinition{
					{Name: "period", Type: ValueTypeInt, Required: true, Min: json.Number("1")},
				},
				Outputs: []OutputDefinition{
					{Name: "highest", Type: ValueTypeFloat, Semantic: OutputSemanticPrice},
					{Name: "lowest", Type: ValueTypeFloat, Semantic: OutputSemanticPrice},
				},
			},
			validator: validatePeriodParams,
			builder:   buildExtreme,
		},
		{
			definition: Definition{
				Type:     "boll",
				Label:    "BOLL",
				Category: "volatility",
				Inputs: []InputDefinition{{
					Name:     "source",
					Type:     ValueTypeFloat,
					Required: true,
				}},
				Params: []ParamDefinition{
					{Name: "period", Type: ValueTypeInt, Required: true, Min: json.Number("1")},
					{Name: "stddev", Type: ValueTypeFloat, Required: true, Min: json.Number("0")},
				},
				Outputs: []OutputDefinition{
					{Name: "middle", Type: ValueTypeFloat, Semantic: OutputSemanticPrice},
					{Name: "upper", Type: ValueTypeFloat, Semantic: OutputSemanticPrice},
					{Name: "lower", Type: ValueTypeFloat, Semantic: OutputSemanticPrice},
				},
			},
			validator: validateBollParams,
			builder:   buildBOLL,
		},
		{
			definition: Definition{
				Type:     "supertrend",
				Label:    "Supertrend",
				Category: "trend",
				Inputs: []InputDefinition{
					{Name: "high", Type: ValueTypeFloat, Required: true},
					{Name: "low", Type: ValueTypeFloat, Required: true},
					{Name: "close", Type: ValueTypeFloat, Required: true},
				},
				Params: []ParamDefinition{
					{Name: "period", Type: ValueTypeInt, Required: true, Min: json.Number("1")},
					{Name: "multiplier", Type: ValueTypeFloat, Required: true, Min: json.Number("0")},
				},
				Outputs: []OutputDefinition{
					{Name: "value", Type: ValueTypeFloat, Semantic: OutputSemanticTrendLevel},
					{Name: "direction", Type: ValueTypeFloat, Semantic: OutputSemanticTrendDirection},
				},
			},
			validator: validateSupertrendParams,
			builder:   buildSupertrend,
		},
	} {
		if err := registry.RegisterRuntime(item.definition, item.validator, item.builder); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func MustBuiltinRegistry() *Registry {
	registry, err := BuiltinRegistry()
	if err != nil {
		panic(err)
	}
	return registry
}

func periodIndicator(
	indicatorType string,
	label string,
	category string,
	output string,
	semantic OutputSemantic,
	crossGroup string,
) Definition {
	return Definition{
		Type:     indicatorType,
		Label:    label,
		Category: category,
		Inputs: []InputDefinition{{
			Name:     "source",
			Type:     ValueTypeFloat,
			Required: true,
		}},
		Params: []ParamDefinition{
			{Name: "period", Type: ValueTypeInt, Required: true, Min: json.Number("1")},
		},
		Outputs: []OutputDefinition{
			{
				Name:       output,
				Type:       ValueTypeFloat,
				Semantic:   semantic,
				CrossGroup: crossGroup,
			},
		},
	}
}

func validatePeriodParams(raw json.RawMessage) error {
	var params PeriodParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return err
	}
	if params.Period < 1 {
		return fmt.Errorf("period must be >= 1")
	}
	return nil
}

func validateBollParams(raw json.RawMessage) error {
	var params BollParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return err
	}
	if params.Period < 1 {
		return fmt.Errorf("period must be >= 1")
	}
	if params.StdDev <= 0 {
		return fmt.Errorf("stddev must be > 0")
	}
	return nil
}

func validateSupertrendParams(raw json.RawMessage) error {
	var params SupertrendParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return err
	}
	if params.Period < 1 {
		return fmt.Errorf("period must be >= 1")
	}
	if params.Multiplier <= 0 {
		return fmt.Errorf("multiplier must be > 0")
	}
	return nil
}

func decodeStrictParams(raw json.RawMessage, target interface{}) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("params is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("params contains trailing data")
	}
	return nil
}
