package strategy

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

func DefinitionSHA256(definition Definition) (string, error) {
	normalized, err := Normalize(definition)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}

	var canonical any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return "", err
	}
	encoded, err = json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func validateRawDefinition(definition Definition, raw []byte) error {
	decoded, err := Decode(raw)
	if err != nil {
		return err
	}
	expectedHash, err := DefinitionSHA256(definition)
	if err != nil {
		return err
	}
	rawHash, err := DefinitionSHA256(decoded)
	if err != nil {
		return err
	}
	if expectedHash != rawHash {
		return fmt.Errorf("raw strategy does not match definition")
	}
	return nil
}
