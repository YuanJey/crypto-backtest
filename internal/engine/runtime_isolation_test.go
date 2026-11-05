package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/indicator"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

func TestSharedCompiledPlanConcurrentRunsHaveIndependentIndicators(t *testing.T) {
	def := baseDefinition()
	def.Indicators[0].Params = json.RawMessage(`{"period":3}`)
	def.Rules = []strategy.RuleSpec{{
		ID: "enter", When: strategy.Bool(true),
		Action: strategy.ActionSpec{Type: strategy.ActionEnter, Side: strategy.TradeSideLong, SizePct: 0.1},
	}}
	plan, err := compiler.Compile(def, indicator.MustBuiltinRegistry())
	if err != nil {
		t.Fatal(err)
	}
	report := DefaultReportSpec()
	report.Indicators = []IndicatorOverlay{{ID: "ema_15m"}}
	requests := []RunRequest{
		{Bars: constantPriceBars(80, 100), Report: report},
		{Bars: testBars(80), Report: report},
	}
	want := make([]Result, len(requests))
	for index, request := range requests {
		want[index], err = Run(context.Background(), plan, request)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 24)
	for index := 0; index < cap(errors); index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			choice := index % len(requests)
			got, err := Run(context.Background(), plan, requests[choice])
			if err != nil {
				errors <- err
			} else if !reflect.DeepEqual(got, want[choice]) {
				errors <- fmt.Errorf("run %d shared another runtime's state", index)
			}
		}(index)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
