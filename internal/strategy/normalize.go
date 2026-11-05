package strategy

import "strings"

func Normalize(definition Definition) (Definition, error) {
	if definition.Account.ExchangeLeverage == 0 {
		definition.Account.ExchangeLeverage = 1
	}
	if definition.Position.PositionLeverage == 0 {
		if definition.Position.TargetLeverage != 0 {
			definition.Position.PositionLeverage = definition.Position.TargetLeverage
		} else {
			definition.Position.PositionLeverage = 1
		}
	}
	if definition.Position.LongCapitalDecayPower == 0 {
		definition.Position.LongCapitalDecayPower = DefaultCapitalDecayPower
	}
	if definition.Position.ShortCapitalDecayPower == 0 {
		definition.Position.ShortCapitalDecayPower = DefaultCapitalDecayPower
	}
	definition.Position.TargetLeverage = 0
	definition.Position.MaxPositionPct = 0
	// Exposure caps are legacy configuration. The exchange leverage is the
	// single runtime ceiling for total position notional.
	definition.Risk.MaxSymbolEquityPct = definition.Account.ExchangeLeverage
	definition.Risk.MaxTotalExposurePct = definition.Account.ExchangeLeverage
	if definition.Position.AllowPyramiding {
		definition.Behavior.AllowPyramiding = true
	}
	definition.Position.AllowPyramiding = false
	for index := range definition.ExitPolicies {
		policy := &definition.ExitPolicies[index]
		if policy.Anchor.Type == "" {
			policy.Anchor.Type = ExitAnchorAverageEntry
		}
		if policy.ClosePct == 0 {
			policy.ClosePct = 1
		}
		if policy.Execution == "" {
			policy.Execution = ExitExecutionNextOpen
		}
		if policy.TriggerPrice == "" {
			policy.TriggerPrice = defaultExitTriggerPrice(policy.Side, policy.Purpose)
		}
		if policy.Distance.Type == ExitDistanceATRMultiple {
			if policy.Distance.Field == "" {
				policy.Distance.Field = "value"
			}
			if policy.Distance.Sampling == "" {
				policy.Distance.Sampling = ExitSamplingLive
			}
		}
	}

	for index, rule := range definition.Rules {
		if rule.AdditionTrigger != nil {
			trigger := *rule.AdditionTrigger
			if trigger.Anchor.Type == "" {
				trigger.Anchor.Type = ExitAnchorAverageEntry
			}
			if trigger.Direction == "" {
				trigger.Direction = AdditionAdverse
			}
			if trigger.TriggerPrice == "" {
				trigger.TriggerPrice = ExitTriggerClose
			}
			if trigger.Distance.Type == ExitDistanceATRMultiple {
				if trigger.Distance.Field == "" {
					trigger.Distance.Field = "value"
				}
				if trigger.Distance.Sampling == "" {
					trigger.Distance.Sampling = ExitSamplingLive
				}
			}
			definition.Rules[index].AdditionTrigger = &trigger
		}
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
			if indicatorType == "atr" || indicatorType == "supertrend" {
				definition.Indicators[index].Inputs = []ValueRef{
					{Scope: ValueScopeMarket, Name: "bar", Field: "high"},
					{Scope: ValueScopeMarket, Name: "bar", Field: "low"},
					{Scope: ValueScopeMarket, Name: "bar", Field: "close"},
				}
			} else if indicatorType == "extreme" {
				definition.Indicators[index].Inputs = []ValueRef{
					{Scope: ValueScopeMarket, Name: "bar", Field: "high"},
					{Scope: ValueScopeMarket, Name: "bar", Field: "low"},
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

func defaultExitTriggerPrice(side TradeSide, purpose ExitPurpose) ExitTriggerPrice {
	if (side == TradeSideLong && purpose == ExitPurposeTakeProfit) ||
		(side == TradeSideShort && purpose == ExitPurposeStopLoss) {
		return ExitTriggerHigh
	}
	return ExitTriggerLow
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
