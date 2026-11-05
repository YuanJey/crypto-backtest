package strategy

import (
	"encoding/json"
	"fmt"
)

// LogicDefinition contains only trading logic. Definition remains the V1
// transport/storage contract; its environment is a default run configuration.
type LogicDefinition struct {
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Behavior      BehaviorSpec     `json:"behavior"`
	Position      PositionSpec     `json:"position"`
	ExitPolicies  []ExitPolicySpec `json:"exit_policies,omitempty"`
	Indicators    []IndicatorSpec  `json:"indicators"`
	Rules         []RuleSpec       `json:"rules,omitempty"`
}

type RunConfig struct {
	Data      DataSelection `json:"data"`
	Account   AccountSpec   `json:"account"`
	Execution ExecutionSpec `json:"execution"`
	Risk      RiskSpec      `json:"risk"`
}

// AdaptV1 owns its input before normalization, including nested expressions
// and policy slices. Legacy exposure aliases apply only at this boundary.
func AdaptV1(definition Definition) (LogicDefinition, RunConfig, error) {
	raw, err := json.Marshal(definition)
	if err != nil {
		return LogicDefinition{}, RunConfig{}, err
	}
	if err := json.Unmarshal(raw, &definition); err != nil {
		return LogicDefinition{}, RunConfig{}, err
	}
	definition, err = Normalize(definition)
	if err != nil {
		return LogicDefinition{}, RunConfig{}, err
	}
	return LogicDefinition{
			SchemaVersion: definition.SchemaVersion, ID: definition.ID,
			Name: definition.Name, Description: definition.Description,
			Behavior: definition.Behavior, Position: definition.Position,
			ExitPolicies: definition.ExitPolicies, Indicators: definition.Indicators,
			Rules: definition.Rules,
		}, RunConfig{
			Data: definition.Data, Account: definition.Account,
			Execution: definition.Execution, Risk: definition.Risk,
		}, nil
}

func ComposeV1(logic LogicDefinition, config RunConfig) Definition {
	return Definition{
		SchemaVersion: logic.SchemaVersion, ID: logic.ID, Name: logic.Name,
		Description: logic.Description, Behavior: logic.Behavior, Position: logic.Position,
		ExitPolicies: logic.ExitPolicies, Indicators: logic.Indicators, Rules: logic.Rules,
		Data: config.Data, Account: config.Account, Execution: config.Execution, Risk: config.Risk,
	}
}

// ValidateRunConfig deliberately does not Normalize: explicit experiment
// risk limits must not be replaced by compatibility defaults.
func ValidateRunConfig(config RunConfig, logic LogicDefinition) error {
	if err := validateData(config.Data); err != nil {
		return err
	}
	if err := validateAccount(config.Account); err != nil {
		return err
	}
	if err := validateExecution(config.Execution); err != nil {
		return err
	}
	if err := validateRisk(config.Risk); err != nil {
		return err
	}
	if err := validatePosition(logic.Position, logic.Behavior, config.Account); err != nil {
		return err
	}
	if config.Account.MarketType == MarketTypeSpot {
		for _, rule := range logic.Rules {
			if !rule.Disabled && rule.Action.Side == TradeSideShort {
				return fmt.Errorf("rule %q: short side requires perpetual market", rule.ID)
			}
		}
		for _, policy := range logic.ExitPolicies {
			if !policy.Disabled && policy.Side == TradeSideShort {
				return fmt.Errorf("exit policy %q: short side requires perpetual market", policy.ID)
			}
		}
	}
	return nil
}
