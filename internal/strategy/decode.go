package strategy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func Decode(raw []byte) (Definition, error) {
	var definition Definition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return Definition{}, fmt.Errorf("decode strategy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Definition{}, fmt.Errorf("decode strategy: trailing data")
	}
	return definition, nil
}

func DecodeRunConfig(raw []byte) (RunConfig, error) {
	var config RunConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return RunConfig{}, fmt.Errorf("decode run config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RunConfig{}, fmt.Errorf("decode run config: trailing data")
	}
	return config, nil
}
