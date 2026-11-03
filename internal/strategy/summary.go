package strategy

import "encoding/json"

type Summary struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	DatasetID        string             `json:"dataset_id"`
	Symbols          []string           `json:"symbols"`
	Timeframe        string             `json:"timeframe"`
	MarketType       MarketType         `json:"market_type"`
	InitialCash      float64            `json:"initial_cash"`
	QuoteCurrency    string             `json:"quote_currency"`
	ExchangeLeverage float64            `json:"exchange_leverage"`
	PositionLeverage float64            `json:"position_leverage"`
	IndicatorCount   int                `json:"indicator_count"`
	Indicators       []IndicatorSummary `json:"indicators"`
	RuleCount        int                `json:"rule_count"`
	Rules            []RuleSummary      `json:"rules"`
}

type IndicatorSummary struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timeframe string          `json:"timeframe"`
	Params    json.RawMessage `json:"params"`
}

type RuleSummary struct {
	ID       string     `json:"id"`
	Disabled bool       `json:"disabled,omitempty"`
	Action   ActionSpec `json:"action"`
}

func Summarize(definition Definition) Summary {
	exchangeLeverage := definition.Account.ExchangeLeverage
	if exchangeLeverage == 0 {
		exchangeLeverage = 1
	}
	positionLeverage := definition.Position.PositionLeverage
	if positionLeverage == 0 {
		positionLeverage = definition.Position.TargetLeverage
	}
	if positionLeverage == 0 {
		positionLeverage = 1
	}
	indicators := make([]IndicatorSummary, 0, len(definition.Indicators))
	for _, spec := range definition.Indicators {
		timeframe := spec.Timeframe
		if timeframe == "" {
			timeframe = definition.Data.Timeframe
		}
		indicators = append(indicators, IndicatorSummary{
			ID:        spec.ID,
			Type:      spec.Type,
			Timeframe: timeframe,
			Params:    spec.Params,
		})
	}
	rules := make([]RuleSummary, 0, len(definition.Rules))
	for _, rule := range definition.Rules {
		rules = append(rules, RuleSummary{
			ID:       rule.ID,
			Disabled: rule.Disabled,
			Action:   rule.Action,
		})
	}
	return Summary{
		ID:               definition.ID,
		Name:             definition.Name,
		DatasetID:        definition.Data.DatasetID,
		Symbols:          definition.Data.Symbols,
		Timeframe:        definition.Data.Timeframe,
		MarketType:       definition.Account.MarketType,
		InitialCash:      definition.Account.InitialCash,
		QuoteCurrency:    definition.Account.QuoteCurrency,
		ExchangeLeverage: exchangeLeverage,
		PositionLeverage: positionLeverage,
		IndicatorCount:   len(definition.Indicators),
		Indicators:       indicators,
		RuleCount:        len(definition.Rules),
		Rules:            rules,
	}
}
