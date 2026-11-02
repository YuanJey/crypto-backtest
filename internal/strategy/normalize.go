package strategy

import "strings"

func Normalize(definition Definition) (Definition, error) {
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
