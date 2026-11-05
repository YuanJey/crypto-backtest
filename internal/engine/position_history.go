package engine

import (
	"github.com/YuanJey/crypto-backtest/internal/compiler"
	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

// positionHistory retains completed-bar portfolio values for one Run. Storing
// only prior snapshots (after rule evaluation) needs maxLag slots, not maxLag+1.
// The current bar always uses evalContext.portfolio.
type positionHistory struct {
	snapshots []positionSnapshot
}

type positionSnapshot struct {
	barIndex  int
	portfolio portfolio
}

func newPositionHistory(rules []compiler.RulePlan, barCount int) *positionHistory {
	maxLag := 0
	var visit func(strategy.Expression, int)
	visit = func(expr strategy.Expression, delta int) {
		if expr.Ref != nil && expr.Ref.Scope == strategy.ValueScopePosition {
			offset := expr.Ref.Offset
			// Ignore references that can never be available in this run. Compare
			// before negation so even the minimum int cannot overflow.
			if offset <= 0 && offset > -barCount {
				lag := -offset
				if lag <= barCount-1-delta {
					lag += delta
					if lag > maxLag {
						maxLag = lag
					}
				}
			}
		}
		if expr.Op == strategy.OperatorCrossUp || expr.Op == strategy.OperatorCrossDown ||
			expr.Op == strategy.OperatorTurnBullish || expr.Op == strategy.OperatorTurnBearish {
			delta++
		}
		for _, arg := range expr.Args {
			visit(arg, delta)
		}
	}
	for _, rule := range rules {
		visit(rule.When, 0)
	}
	if maxLag == 0 {
		return nil
	}
	history := &positionHistory{snapshots: make([]positionSnapshot, maxLag)}
	for index := range history.snapshots {
		history.snapshots[index].barIndex = -1
	}
	return history
}

func (h *positionHistory) record(index int, p portfolio) {
	if h == nil || len(h.snapshots) == 0 || index < 0 {
		return
	}
	h.snapshots[index%len(h.snapshots)] = positionSnapshot{barIndex: index, portfolio: p}
}

func (h *positionHistory) lookup(index int) (portfolio, bool) {
	if h == nil || len(h.snapshots) == 0 || index < 0 {
		return portfolio{}, false
	}
	snapshot := h.snapshots[index%len(h.snapshots)]
	return snapshot.portfolio, snapshot.barIndex == index
}
