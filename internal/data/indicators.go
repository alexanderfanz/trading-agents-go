package data

import (
	"fmt"
	"math"
)

// CalculateSMA computes a Simple Moving Average.
func CalculateSMA(data []OHLCV, period int) []float64 {
	results := make([]float64, len(data))
	if len(data) < period {
		for i := range results {
			results[i] = math.NaN()
		}
		return results
	}

	// Pad with NaNs for warm-up period
	for i := 0; i < period-1; i++ {
		results[i] = math.NaN()
	}

	var sum float64
	for i := 0; i < period; i++ {
		sum += data[i].Close
	}
	results[period-1] = sum / float64(period)

	for i := period; i < len(data); i++ {
		sum = sum - data[i-period].Close + data[i].Close
		results[i] = sum / float64(period)
	}
	return results
}

// CalculateEMA computes an Exponential Moving Average.
func CalculateEMA(data []OHLCV, period int) []float64 {
	results := make([]float64, len(data))
	if len(data) < period {
		for i := range results {
			results[i] = math.NaN()
		}
		return results
	}

	// Pad with NaNs
	for i := 0; i < period-1; i++ {
		results[i] = math.NaN()
	}

	// First value is SMA
	var sum float64
	for i := 0; i < period; i++ {
		sum += data[i].Close
	}
	results[period-1] = sum / float64(period)

	k := 2.0 / float64(period+1)
	for i := period; i < len(data); i++ {
		results[i] = (data[i].Close * k) + (results[i-1] * (1.0 - k))
	}
	return results
}

// CalculateRSI computes the Relative Strength Index.
func CalculateRSI(data []OHLCV, period int) []float64 {
	results := make([]float64, len(data))
	if len(data) < period+1 {
		for i := range results {
			results[i] = math.NaN()
		}
		return results
	}

	// Pad with NaNs
	for i := 0; i < period; i++ {
		results[i] = math.NaN()
	}

	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		change := data[i].Close - data[i-1].Close
		if change > 0 {
			avgGain += change
		} else {
			avgLoss += math.Abs(change)
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	if avgLoss == 0 {
		results[period] = 100
	} else {
		rs := avgGain / avgLoss
		results[period] = 100 - (100 / (1 + rs))
	}

	for i := period + 1; i < len(data); i++ {
		change := data[i].Close - data[i-1].Close
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = math.Abs(change)
		}

		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)

		if avgLoss == 0 {
			results[i] = 100
		} else {
			rs := avgGain / avgLoss
			results[i] = 100 - (100 / (1 + rs))
		}
	}
	return results
}

type MACDResult struct {
	MACD      []float64
	Signal    []float64
	Histogram []float64
}

// CalculateMACD computes MACD, MACD Signal, and MACD Histogram.
func CalculateMACD(data []OHLCV, fastPeriod, slowPeriod, signalPeriod int) MACDResult {
	n := len(data)
	res := MACDResult{
		MACD:      make([]float64, n),
		Signal:    make([]float64, n),
		Histogram: make([]float64, n),
	}

	for i := 0; i < n; i++ {
		res.MACD[i] = math.NaN()
		res.Signal[i] = math.NaN()
		res.Histogram[i] = math.NaN()
	}

	if n < slowPeriod {
		return res
	}

	fastEMA := CalculateEMA(data, fastPeriod)
	slowEMA := CalculateEMA(data, slowPeriod)

	for i := slowPeriod - 1; i < n; i++ {
		if !math.IsNaN(fastEMA[i]) && !math.IsNaN(slowEMA[i]) {
			res.MACD[i] = fastEMA[i] - slowEMA[i]
		}
	}

	// Compute Signal Line (EMA of MACD)
	// We extract MACD values as virtual Close values to use CalculateEMA
	dummyOHLCV := make([]OHLCV, n)
	for i := 0; i < n; i++ {
		if math.IsNaN(res.MACD[i]) {
			dummyOHLCV[i] = OHLCV{Close: 0} // Placeholder, won't be used since we shift index
		} else {
			dummyOHLCV[i] = OHLCV{Close: res.MACD[i]}
		}
	}

	// We compute EMA over valid MACD entries
	startIdx := slowPeriod - 1
	validMACD := dummyOHLCV[startIdx:]
	signalSub := CalculateEMA(validMACD, signalPeriod)

	for i := 0; i < len(signalSub); i++ {
		res.Signal[startIdx+i] = signalSub[i]
	}

	for i := 0; i < n; i++ {
		if !math.IsNaN(res.MACD[i]) && !math.IsNaN(res.Signal[i]) {
			res.Histogram[i] = res.MACD[i] - res.Signal[i]
		}
	}

	return res
}

type BollingerResult struct {
	Middle []float64
	Upper  []float64
	Lower  []float64
}

// CalculateBollingerBands computes Bollinger Bands (Middle, Upper, Lower) over Close prices.
func CalculateBollingerBands(data []OHLCV, period int, stdDevMultiplier float64) BollingerResult {
	n := len(data)
	res := BollingerResult{
		Middle: make([]float64, n),
		Upper:  make([]float64, n),
		Lower:  make([]float64, n),
	}

	for i := 0; i < n; i++ {
		res.Middle[i] = math.NaN()
		res.Upper[i] = math.NaN()
		res.Lower[i] = math.NaN()
	}

	if n < period {
		return res
	}

	sma := CalculateSMA(data, period)

	for i := period - 1; i < n; i++ {
		res.Middle[i] = sma[i]
		
		// Calculate standard deviation of Close prices in window [i-period+1, i]
		var sumSqDiff float64
		mean := sma[i]
		for j := i - period + 1; j <= i; j++ {
			diff := data[j].Close - mean
			sumSqDiff += diff * diff
		}
		variance := sumSqDiff / float64(period)
		stdDev := math.Sqrt(variance)

		res.Upper[i] = mean + (stdDevMultiplier * stdDev)
		res.Lower[i] = mean - (stdDevMultiplier * stdDev)
	}

	return res
}

// CalculateATR computes the Average True Range.
func CalculateATR(data []OHLCV, period int) []float64 {
	results := make([]float64, len(data))
	if len(data) < period {
		for i := range results {
			results[i] = math.NaN()
		}
		return results
	}

	// Pad with NaNs
	for i := 0; i < period-1; i++ {
		results[i] = math.NaN()
	}

	// Calculate True Ranges (TR)
	tr := make([]float64, len(data))
	tr[0] = data[0].High - data[0].Low

	for i := 1; i < len(data); i++ {
		hMinusL := data[i].High - data[i].Low
		hMinusPrevC := math.Abs(data[i].High - data[i-1].Close)
		lMinusPrevC := math.Abs(data[i].Low - data[i-1].Close)
		tr[i] = math.Max(hMinusL, math.Max(hMinusPrevC, lMinusPrevC))
	}

	// First ATR is SMA of TR
	var sum float64
	for i := 0; i < period; i++ {
		sum += tr[i]
	}
	results[period-1] = sum / float64(period)

	// Wilder's smoothing technique for ATR
	for i := period; i < len(data); i++ {
		results[i] = (results[i-1]*float64(period-1) + tr[i]) / float64(period)
	}
	return results
}

// CalculateVWMA computes the Volume Weighted Moving Average.
func CalculateVWMA(data []OHLCV, period int) []float64 {
	results := make([]float64, len(data))
	if len(data) < period {
		for i := range results {
			results[i] = math.NaN()
		}
		return results
	}

	// Pad with NaNs
	for i := 0; i < period-1; i++ {
		results[i] = math.NaN()
	}

	var sumPV, sumV float64
	for i := 0; i < period; i++ {
		sumPV += data[i].Close * data[i].Volume
		sumV += data[i].Volume
	}
	
	if sumV > 0 {
		results[period-1] = sumPV / sumV
	} else {
		results[period-1] = data[period-1].Close
	}

	for i := period; i < len(data); i++ {
		dropIdx := i - period
		sumPV = sumPV - (data[dropIdx].Close * data[dropIdx].Volume) + (data[i].Close * data[i].Volume)
		sumV = sumV - data[dropIdx].Volume + data[i].Volume
		
		if sumV > 0 {
			results[i] = sumPV / sumV
		} else {
			results[i] = data[i].Close
		}
	}
	return results
}

// FormatIndicatorValue converts a float64 indicator value to a string, handling NaN.
func FormatIndicatorValue(val float64) string {
	if math.IsNaN(val) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", val)
}

// CalculateMFI computes the Money Flow Index (MFI) over a sliding period.
func CalculateMFI(data []OHLCV, period int) []float64 {
	results := make([]float64, len(data))
	if len(data) < period {
		for i := range results {
			results[i] = math.NaN()
		}
		return results
	}

	for i := 0; i < period; i++ {
		results[i] = math.NaN()
	}

	tp := make([]float64, len(data))
	for i := range data {
		tp[i] = (data[i].High + data[i].Low + data[i].Close) / 3.0
	}

	for i := period; i < len(data); i++ {
		var posFlow, negFlow float64
		for j := i - period + 1; j <= i; j++ {
			flow := tp[j] * data[j].Volume
			if tp[j] > tp[j-1] {
				posFlow += flow
			} else if tp[j] < tp[j-1] {
				negFlow += flow
			}
		}

		if negFlow == 0 {
			results[i] = 100
		} else {
			ratio := posFlow / negFlow
			results[i] = 100 - (100 / (1 + ratio))
		}
	}
	return results
}
