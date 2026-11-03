package indicator

import (
	"encoding/json"
	"math"
)

func buildEMA(raw json.RawMessage) (RuntimeIndicator, error) {
	var params PeriodParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return nil, err
	}
	return &emaRuntime{period: params.Period, alpha: 2.0 / float64(params.Period+1)}, nil
}

type emaRuntime struct {
	period int
	alpha  float64
	count  int
	value  float64
}

func (r *emaRuntime) Warmup() int { return r.period }

func (r *emaRuntime) Reset() { *r = emaRuntime{period: r.period, alpha: r.alpha} }

func (r *emaRuntime) Update(inputs []float64) []float64 {
	value := firstInput(inputs)
	if math.IsNaN(value) {
		return []float64{math.NaN()}
	}
	r.count++
	if r.count == 1 {
		r.value = value
	} else {
		r.value = r.alpha*value + (1-r.alpha)*r.value
	}
	if r.count < r.period {
		return []float64{math.NaN()}
	}
	return []float64{r.value}
}

func buildSMA(raw json.RawMessage) (RuntimeIndicator, error) {
	var params PeriodParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return nil, err
	}
	return &smaRuntime{period: params.Period}, nil
}

type smaRuntime struct {
	period int
	window []float64
	sum    float64
}

func (r *smaRuntime) Warmup() int { return r.period }

func (r *smaRuntime) Reset() { *r = smaRuntime{period: r.period} }

func (r *smaRuntime) Update(inputs []float64) []float64 {
	value := firstInput(inputs)
	if math.IsNaN(value) {
		return []float64{math.NaN()}
	}
	r.window = append(r.window, value)
	r.sum += value
	if len(r.window) > r.period {
		r.sum -= r.window[0]
		r.window = r.window[1:]
	}
	if len(r.window) < r.period {
		return []float64{math.NaN()}
	}
	return []float64{r.sum / float64(r.period)}
}

func buildBOLL(raw json.RawMessage) (RuntimeIndicator, error) {
	var params BollParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return nil, err
	}
	return &bollRuntime{period: params.Period, stddev: params.StdDev}, nil
}

type bollRuntime struct {
	period int
	stddev float64
	window []float64
}

func (r *bollRuntime) Warmup() int { return r.period }

func (r *bollRuntime) Reset() { *r = bollRuntime{period: r.period, stddev: r.stddev} }

func (r *bollRuntime) Update(inputs []float64) []float64 {
	value := firstInput(inputs)
	if math.IsNaN(value) {
		return []float64{math.NaN(), math.NaN(), math.NaN()}
	}
	r.window = append(r.window, value)
	if len(r.window) > r.period {
		r.window = r.window[1:]
	}
	if len(r.window) < r.period {
		return []float64{math.NaN(), math.NaN(), math.NaN()}
	}
	var sum float64
	for _, item := range r.window {
		sum += item
	}
	mean := sum / float64(r.period)
	var variance float64
	for _, item := range r.window {
		diff := item - mean
		variance += diff * diff
	}
	deviation := math.Sqrt(variance / float64(r.period))
	return []float64{mean, mean + r.stddev*deviation, mean - r.stddev*deviation}
}

func buildRSI(raw json.RawMessage) (RuntimeIndicator, error) {
	var params PeriodParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return nil, err
	}
	return &rsiRuntime{period: params.Period}, nil
}

type rsiRuntime struct {
	period      int
	count       int
	prev        float64
	hasPrev     bool
	avgGain     float64
	avgLoss     float64
	initialGain float64
	initialLoss float64
}

func (r *rsiRuntime) Warmup() int { return r.period + 1 }

func (r *rsiRuntime) Reset() { *r = rsiRuntime{period: r.period} }

func (r *rsiRuntime) Update(inputs []float64) []float64 {
	value := firstInput(inputs)
	if math.IsNaN(value) {
		return []float64{math.NaN()}
	}
	if !r.hasPrev {
		r.prev = value
		r.hasPrev = true
		return []float64{math.NaN()}
	}
	change := value - r.prev
	r.prev = value
	gain := math.Max(change, 0)
	loss := math.Max(-change, 0)
	r.count++
	if r.count <= r.period {
		r.initialGain += gain
		r.initialLoss += loss
		if r.count < r.period {
			return []float64{math.NaN()}
		}
		r.avgGain = r.initialGain / float64(r.period)
		r.avgLoss = r.initialLoss / float64(r.period)
		return []float64{rsiValue(r.avgGain, r.avgLoss)}
	}
	r.avgGain = (r.avgGain*float64(r.period-1) + gain) / float64(r.period)
	r.avgLoss = (r.avgLoss*float64(r.period-1) + loss) / float64(r.period)
	return []float64{rsiValue(r.avgGain, r.avgLoss)}
}

func buildATR(raw json.RawMessage) (RuntimeIndicator, error) {
	var params PeriodParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return nil, err
	}
	return &atrRuntime{period: params.Period}, nil
}

type atrRuntime struct {
	period  int
	count   int
	prev    float64
	hasPrev bool
	value   float64
	sum     float64
}

func (r *atrRuntime) Warmup() int { return r.period + 1 }

func (r *atrRuntime) Reset() { *r = atrRuntime{period: r.period} }

func (r *atrRuntime) Update(inputs []float64) []float64 {
	if len(inputs) < 3 {
		return []float64{math.NaN()}
	}
	high, low, closeValue := inputs[0], inputs[1], inputs[2]
	if math.IsNaN(high) || math.IsNaN(low) || math.IsNaN(closeValue) {
		return []float64{math.NaN()}
	}
	if !r.hasPrev {
		r.prev = closeValue
		r.hasPrev = true
		return []float64{math.NaN()}
	}
	tr := math.Max(high-low, math.Max(math.Abs(high-r.prev), math.Abs(low-r.prev)))
	r.prev = closeValue
	r.count++
	if r.count <= r.period {
		r.sum += tr
		if r.count < r.period {
			return []float64{math.NaN()}
		}
		r.value = r.sum / float64(r.period)
		return []float64{r.value}
	}
	r.value = (r.value*float64(r.period-1) + tr) / float64(r.period)
	return []float64{r.value}
}

func buildSupertrend(raw json.RawMessage) (RuntimeIndicator, error) {
	var params SupertrendParams
	if err := decodeStrictParams(raw, &params); err != nil {
		return nil, err
	}
	return &supertrendRuntime{
		period:     params.Period,
		multiplier: params.Multiplier,
	}, nil
}

type supertrendRuntime struct {
	period        int
	multiplier    float64
	atrCount      int
	atrSum        float64
	atr           float64
	prevClose     float64
	hasPrevClose  bool
	finalUpper    float64
	finalLower    float64
	value         float64
	direction     float64
	hasSupertrend bool
}

func (r *supertrendRuntime) Warmup() int { return r.period + 1 }

func (r *supertrendRuntime) Reset() {
	*r = supertrendRuntime{
		period:     r.period,
		multiplier: r.multiplier,
	}
}

func (r *supertrendRuntime) Update(inputs []float64) []float64 {
	if len(inputs) < 3 {
		return []float64{math.NaN(), math.NaN()}
	}
	high, low, closeValue := inputs[0], inputs[1], inputs[2]
	if math.IsNaN(high) || math.IsNaN(low) || math.IsNaN(closeValue) {
		return []float64{math.NaN(), math.NaN()}
	}
	if !r.hasPrevClose {
		r.prevClose = closeValue
		r.hasPrevClose = true
		return []float64{math.NaN(), math.NaN()}
	}

	previousClose := r.prevClose
	trueRange := math.Max(
		high-low,
		math.Max(math.Abs(high-previousClose), math.Abs(low-previousClose)),
	)
	r.prevClose = closeValue
	r.atrCount++
	if r.atrCount <= r.period {
		r.atrSum += trueRange
		if r.atrCount < r.period {
			return []float64{math.NaN(), math.NaN()}
		}
		r.atr = r.atrSum / float64(r.period)
	} else {
		r.atr = (r.atr*float64(r.period-1) + trueRange) / float64(r.period)
	}

	midpoint := (high + low) / 2
	basicUpper := midpoint + r.multiplier*r.atr
	basicLower := midpoint - r.multiplier*r.atr
	if !r.hasSupertrend {
		r.finalUpper = basicUpper
		r.finalLower = basicLower
		if closeValue >= midpoint {
			r.value = r.finalLower
			r.direction = 1
		} else {
			r.value = r.finalUpper
			r.direction = -1
		}
		r.hasSupertrend = true
		return []float64{r.value, r.direction}
	}

	previousUpper := r.finalUpper
	previousLower := r.finalLower
	previousValue := r.value
	if basicUpper < previousUpper || previousClose > previousUpper {
		r.finalUpper = basicUpper
	}
	if basicLower > previousLower || previousClose < previousLower {
		r.finalLower = basicLower
	}

	if previousValue == previousUpper {
		if closeValue <= r.finalUpper {
			r.value = r.finalUpper
			r.direction = -1
		} else {
			r.value = r.finalLower
			r.direction = 1
		}
	} else if closeValue >= r.finalLower {
		r.value = r.finalLower
		r.direction = 1
	} else {
		r.value = r.finalUpper
		r.direction = -1
	}
	return []float64{r.value, r.direction}
}

func firstInput(inputs []float64) float64 {
	if len(inputs) == 0 {
		return math.NaN()
	}
	return inputs[0]
}

func rsiValue(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}
