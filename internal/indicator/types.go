package indicator

import "encoding/json"

type ValueType string
type OutputSemantic string

const (
	ValueTypeFloat ValueType = "float"
	ValueTypeInt   ValueType = "int"

	OutputSemanticPrice          OutputSemantic = "price_series"
	OutputSemanticOscillator     OutputSemantic = "oscillator_series"
	OutputSemanticVolatility     OutputSemantic = "volatility_series"
	OutputSemanticTrendLevel     OutputSemantic = "trend_level"
	OutputSemanticTrendDirection OutputSemantic = "trend_direction"
)

type Definition struct {
	Type     string             `json:"type"`
	Label    string             `json:"label"`
	Category string             `json:"category"`
	Inputs   []InputDefinition  `json:"inputs"`
	Params   []ParamDefinition  `json:"params"`
	Outputs  []OutputDefinition `json:"outputs"`
}

type InputDefinition struct {
	Name     string    `json:"name"`
	Type     ValueType `json:"type"`
	Required bool      `json:"required"`
}

type ParamDefinition struct {
	Name     string      `json:"name"`
	Type     ValueType   `json:"type"`
	Required bool        `json:"required"`
	Min      json.Number `json:"min,omitempty"`
}

type OutputDefinition struct {
	Name       string         `json:"name"`
	Type       ValueType      `json:"type"`
	Semantic   OutputSemantic `json:"semantic"`
	CrossGroup string         `json:"cross_group,omitempty"`
}

type ParamValidator func(json.RawMessage) error

type RuntimeBuilder func(json.RawMessage) (RuntimeIndicator, error)

type RuntimeIndicator interface {
	Warmup() int
	Update(inputs []float64) []float64
	Reset()
}
