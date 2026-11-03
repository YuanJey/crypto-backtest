package strategy

import "strings"

func Normalize(definition Definition) (Definition, error) {
	if definition.Position.AllowPyramiding {
		definition.Behavior.AllowPyramiding = true
	}
	definition.Position.AllowPyramiding = false

	for index, rule := range definition.Rules {
		condition := strings.TrimSpace(rule.Condition)
		if condition == "" {
			continue
		}
		if rule.When.Kind != "" {
			return Definition{}, errRuleConditionConflict(rule.ID)
		}
		expression, err := ParseCondition(condition)
		if err != nil {
			return Definition{}, err
		}
		definition.Rules[index].When = expression
		definition.Rules[index].Condition = ""
	}

	// 自动补齐未显式声明 inputs 的技术指标默认 K 线字段
	for index, spec := range definition.Indicators {
		if len(spec.Inputs) == 0 {
			indicatorType := strings.ToLower(strings.TrimSpace(spec.Type))
			if indicatorType == "atr" {
				definition.Indicators[index].Inputs = []ValueRef{
					{Scope: ValueScopeMarket, Name: "bar", Field: "high"},
					{Scope: ValueScopeMarket, Name: "bar", Field: "low"},
					{Scope: ValueScopeMarket, Name: "bar", Field: "close"},
				}
			} else {
				definition.Indicators[index].Inputs = []ValueRef{
					{Scope: ValueScopeMarket, Name: "bar", Field: "close"},
				}
			}
		}
	}

	return definition, nil
}

func errRuleConditionConflict(ruleID string) error {
	return &conditionConflictError{ruleID: ruleID}
}

type conditionConflictError struct {
	ruleID string
}

func (e *conditionConflictError) Error() string {
	return "rule " + e.ruleID + " cannot define both condition and when"
}
