package engine

import (
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/market"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestEvalTrendDirectionTransitions(t *testing.T) {
	ctx := evalContext{
		bars:     []market.Bar{{}, {}},
		barIndex: 1,
		indicators: map[string]map[string][]float64{
			"supertrend": {
				"direction": {-1, 1},
			},
		},
	}
	bullish, err := evalBool(
		strategy.IndicatorField("supertrend", "direction").TurnsBullish(),
		ctx,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bullish {
		t.Fatal("expected -1 to 1 to be a bullish transition")
	}

	ctx.indicators["supertrend"]["direction"] = []float64{1, -1}
	bearish, err := evalBool(
		strategy.IndicatorField("supertrend", "direction").TurnsBearish(),
		ctx,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bearish {
		t.Fatal("expected 1 to -1 to be a bearish transition")
	}

	ctx.barIndex = 0
	firstBar, err := evalBool(
		strategy.IndicatorField("supertrend", "direction").TurnsBearish(),
		ctx,
	)
	if err != nil {
		t.Fatal(err)
	}
	if firstBar {
		t.Fatal("expected transition to be false without a previous value")
	}
}
