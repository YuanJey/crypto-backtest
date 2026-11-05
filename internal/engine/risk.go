package engine

import (
	"math"

	"github.com/YuanJey/crypto-backtest/internal/strategy"
)

type riskController struct {
	spec   strategy.RiskSpec
	peak   float64
	halted bool
}

func (r *riskController) observe(equity float64, closing bool) bool {
	if closing && equity > r.peak {
		r.peak = equity
	}
	r.halted = r.halted || !isPositiveFinite(equity) || drawdownHalted(equity, r.peak, r.spec.MaxDrawdownHaltPct)
	return r.halted
}

func drawdownHalted(equity, peak, limit float64) bool {
	if limit <= 0 || peak <= 0 {
		return false
	}
	return (peak-equity)/peak >= limit
}

// Risk is evaluated separately from strategy eligibility and broker quoting.
// Compatibility defaults equal exchange leverage; explicit run limits may
// reduce that ceiling without rewriting a strategy revision.
func capRiskNotional(p portfolio, requestedPct, price float64, risk strategy.RiskSpec) float64 {
	equity := p.equity(price)
	limit := math.Min(risk.MaxSymbolEquityPct, risk.MaxTotalExposurePct)
	available := equity*limit - p.positionNotional(price)
	if available <= 0 || equity <= 0 {
		return 0
	}
	if equity*requestedPct > available {
		return available / equity
	}
	return requestedPct
}
