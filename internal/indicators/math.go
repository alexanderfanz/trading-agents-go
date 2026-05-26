package indicators

import (
	"errors"
	"math"
)

// Zero-allocation indicators write their outputs directly to the pre-allocated slice.
// If 'out' is smaller than the input slice, an error is returned.

// SMA calculates the Simple Moving Average in-place.
func SMA(prices []float64, period int, out []float64) error {
	if len(prices) < period {
		return errors.New("insufficient data for SMA period")
	}
	if len(out) < len(prices) {
		return errors.New("output slice capacity is smaller than input prices")
	}

	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	out[period-1] = sum / float64(period)

	for i := period; i < len(prices); i++ {
		sum = sum - prices[i-period] + prices[i]
		out[i] = sum / float64(period)
	}
	return nil
}

// EMA calculates the Exponential Moving Average in-place.
func EMA(prices []float64, period int, out []float64) error {
	if len(prices) < period {
		return errors.New("insufficient data for EMA period")
	}
	if len(out) < len(prices) {
		return errors.New("output slice capacity is smaller than input prices")
	}

	k := 2.0 / (float64(period) + 1.0)

	// Seed with SMA
	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	out[period-1] = sum / float64(period)

	for i := period; i < len(prices); i++ {
		out[i] = (prices[i] * k) + (out[i-1] * (1.0 - k))
	}
	return nil
}

// RSI calculates the Relative Strength Index in-place.
func RSI(prices []float64, period int, out []float64) error {
	if len(prices) <= period {
		return errors.New("insufficient data for RSI period")
	}
	if len(out) < len(prices) {
		return errors.New("output slice capacity is smaller than input prices")
	}

	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			avgGain += change
		} else {
			avgLoss -= change
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	if avgLoss == 0 {
		out[period] = 100
	} else {
		out[period] = 100 - (100 / (1 + avgGain/avgLoss))
	}

	for i := period + 1; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}

		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			out[i] = 100
		} else {
			out[i] = 100 - (100 / (1 + avgGain/avgLoss))
		}
	}
	return nil
}

// MACD calculates standard MACD parameters in-place.
func MACD(prices []float64, fastP, slowP, sigP int, fastBuf, slowBuf, macdOut, sigOut, histOut []float64) error {
	if err := EMA(prices, fastP, fastBuf); err != nil {
		return err
	}
	if err := EMA(prices, slowP, slowBuf); err != nil {
		return err
	}
	for i := slowP - 1; i < len(prices); i++ {
		macdOut[i] = fastBuf[i] - slowBuf[i]
	}
	// Calculate signal line by performing EMA on the calculated MACD line
	if err := EMA(macdOut[slowP-1:], sigP, sigOut[slowP-1:]); err != nil {
		return err
	}
	for i := slowP + sigP - 2; i < len(prices); i++ {
		histOut[i] = macdOut[i] - sigOut[i]
	}
	return nil
}

// MFI calculates the Money Flow Index in-place.
func MFI(high, low, close, volume []float64, period int, pmfBuf, nmfBuf, out []float64) error {
	n := len(close)
	if period <= 0 {
		return errors.New("MFI period must be greater than zero")
	}
	if n < period+1 {
		return errors.New("insufficient data points for MFI period")
	}
	if len(high) != n || len(low) != n || len(volume) != n {
		return errors.New("input slice lengths mismatch")
	}
	if len(out) < n || len(pmfBuf) < n || len(nmfBuf) < n {
		return errors.New("buffer or output slice capacities are too small")
	}

	// Bounds Check Elimination (BCE) Hint
	_ = high[n-1]
	_ = low[n-1]
	_ = close[n-1]
	_ = volume[n-1]
	_ = pmfBuf[n-1]
	_ = nmfBuf[n-1]
	_ = out[n-1]

	// Compute Typical Price and raw flow comparisons
	tpPrev := (high[0] + low[0] + close[0]) / 3.0
	pmfBuf[0] = 0.0
	nmfBuf[0] = 0.0

	for i := 1; i < n; i++ {
		tp := (high[i] + low[i] + close[i]) / 3.0
		rmf := tp * volume[i]
		switch {
		case tp > tpPrev:
			pmfBuf[i] = rmf
			nmfBuf[i] = 0.0
		case tp < tpPrev:
			pmfBuf[i] = 0.0
			nmfBuf[i] = rmf
		default:
			pmfBuf[i] = 0.0
			nmfBuf[i] = 0.0
		}
		tpPrev = tp
	}

	// Compute initial window sums
	var sumPMF, sumNMF float64
	for i := 1; i <= period; i++ {
		sumPMF += pmfBuf[i]
		sumNMF += nmfBuf[i]
	}

	if sumNMF == 0 {
		out[period] = 100.0
	} else {
		out[period] = 100.0 - (100.0 / (1.0 + (sumPMF / sumNMF)))
	}

	// Maintain sliding window for O(N) execution
	for i := period + 1; i < n; i++ {
		sumPMF = sumPMF - pmfBuf[i-period] + pmfBuf[i]
		sumNMF = sumNMF - nmfBuf[i-period] + nmfBuf[i]
		if sumNMF == 0 {
			out[i] = 100.0
		} else {
			out[i] = 100.0 - (100.0 / (1.0 + (sumPMF / sumNMF)))
		}
	}
	return nil
}

// ATR calculates the Average True Range in-place.
func ATR(high, low, close []float64, period int, trBuf, out []float64) error {
	n := len(close)
	if period <= 0 {
		return errors.New("ATR period must be greater than zero")
	}
	if n < period+1 {
		return errors.New("insufficient data points for ATR period")
	}
	if len(high) != n || len(low) != n {
		return errors.New("input slice lengths mismatch")
	}
	if len(trBuf) < n || len(out) < n {
		return errors.New("buffer or output slice capacities are too small")
	}

	// Bounds Check Elimination
	_ = high[n-1]
	_ = low[n-1]
	_ = close[n-1]
	_ = trBuf[n-1]
	_ = out[n-1]

	// Seed TR at index 0 (no previous Close)
	trBuf[0] = high[0] - low[0]

	// Compute subsequent True Ranges
	for i := 1; i < n; i++ {
		tr1 := high[i] - low[i]
		tr2 := math.Abs(high[i] - close[i-1])
		tr3 := math.Abs(low[i] - close[i-1])
		trBuf[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// Seed ATR with SMA of TR over first 'period' elements
	var sumTR float64
	for i := 1; i <= period; i++ {
		sumTR += trBuf[i]
	}
	out[period] = sumTR / float64(period)

	// Wilder's smoothing for subsequent ATRs
	pSub1 := float64(period - 1)
	pDiv := float64(period)
	for i := period + 1; i < n; i++ {
		out[i] = (out[i-1]*pSub1 + trBuf[i]) / pDiv
	}
	return nil
}

// BollingerBands calculates Mid, Upper, and Lower Bands in-place.
func BollingerBands(close []float64, period int, k float64, midBand, upperBand, lowerBand []float64) error {
	n := len(close)
	if period <= 0 {
		return errors.New("bollinger bands period must be greater than zero")
	}
	if n < period {
		return errors.New("insufficient data points for Bollinger Bands period")
	}
	if len(midBand) < n || len(upperBand) < n || len(lowerBand) < n {
		return errors.New("output slice capacities are too small")
	}

	// Bounds Check Elimination
	_ = close[n-1]
	_ = midBand[n-1]
	_ = upperBand[n-1]
	_ = lowerBand[n-1]

	// Compute initial window Middle Band (SMA)
	var sum float64
	for i := 0; i < period; i++ {
		sum += close[i]
	}
	midBand[period-1] = sum / float64(period)

	// Compute initial Standard Deviation
	var varSum float64
	mean := midBand[period-1]
	for i := 0; i < period; i++ {
		diff := close[i] - mean
		varSum += diff * diff
	}
	stdDev := math.Sqrt(varSum / float64(period))
	upperBand[period-1] = mean + k*stdDev
	lowerBand[period-1] = mean - k*stdDev

	// Compute sliding windows
	for i := period; i < n; i++ {
		sum = sum - close[i-period] + close[i]
		currentMean := sum / float64(period)
		midBand[i] = currentMean

		// Numerical stability: two-pass within local window to prevent float cancellations
		var currentVarSum float64
		for j := i - period + 1; j <= i; j++ {
			diff := close[j] - currentMean
			currentVarSum += diff * diff
		}
		currentStdDev := math.Sqrt(currentVarSum / float64(period))
		upperBand[i] = currentMean + k*currentStdDev
		lowerBand[i] = currentMean - k*currentStdDev
	}
	return nil
}
