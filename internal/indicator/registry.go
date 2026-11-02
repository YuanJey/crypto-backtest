package indicator

import (
	"encoding/json"
	"fmt"
	"sort"
)

type Registry struct {
	definitions map[string]Definition
	validators  map[string]ParamValidator
	builders    map[string]RuntimeBuilder
}

func NewRegistry() *Registry {
	return &Registry{
		definitions: map[string]Definition{},
		validators:  map[string]ParamValidator{},
		builders:    map[string]RuntimeBuilder{},
	}
}

func (r *Registry) Register(definition Definition, validator ParamValidator) error {
	return r.RegisterRuntime(definition, validator, nil)
}

func (r *Registry) RegisterRuntime(definition Definition, validator ParamValidator, builder RuntimeBuilder) error {
	if definition.Type == "" {
		return fmt.Errorf("indicator type is required")
	}
	if validator == nil {
		return fmt.Errorf("validator is required for indicator %q", definition.Type)
	}
	if _, exists := r.definitions[definition.Type]; exists {
		return fmt.Errorf("duplicate indicator type %q", definition.Type)
	}
	r.definitions[definition.Type] = definition
	r.validators[definition.Type] = validator
	if builder != nil {
		r.builders[definition.Type] = builder
	}
	return nil
}

func (r *Registry) Definition(indicatorType string) (Definition, bool) {
	definition, ok := r.definitions[indicatorType]
	return definition, ok
}

func (r *Registry) Definitions() []Definition {
	definitions := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Type < definitions[j].Type
	})
	return definitions
}

func (r *Registry) ValidateParams(indicatorType string, params json.RawMessage) error {
	validator, ok := r.validators[indicatorType]
	if !ok {
		return fmt.Errorf("unknown indicator type %q", indicatorType)
	}
	return validator(params)
}

func (r *Registry) BuildRuntime(indicatorType string, params json.RawMessage) (RuntimeIndicator, error) {
	builder, ok := r.builders[indicatorType]
	if !ok {
		return nil, fmt.Errorf("indicator %q has no runtime builder", indicatorType)
	}
	return builder(params)
}
