# Component 1: Zero-Dependency Core Data Flows & Dynamic Indicator Engine

## 1. Technical Architecture & Data Flows

The Python implementation dynamically maps indicator request strings (e.g., `close_50_sma`, `macd`, `rsi`) into a Pandas DataFrame using `stockstats`. This introduces significant overhead, dynamic memory allocations, look-ahead bias vulnerabilities, and substantial platform packaging friction.

In the Go implementation, we replace the Pandas layer with a lightweight, concurrency-safe dataflow system. This component is structured as a pipeline of typed structs representing stock OHLCV historical records, rate-limited and cached vendor fetching, look-ahead bias filtering, and zero-allocation in-place mathematical indicators.

```
[Raw CSV Stream (YFinance / Alpha Vantage)]
                 │
                 ▼
[HTTP Client with Token Bucket Rate Limiter (with Full Jitter)]
                 │
                 ▼
[Look-Ahead Bias Filter (prunes timestamps > tradeDate)]
                 │
                 ▼
       [Candle Slice ([]Candle)]
                 │
                 ├──────────────────────────────┐
                 ▼ (Dynamic Indicator Resolver)  ▼ (Thread-Safe Memoization Cache)
        [String Parser]                  [RWMutex Lookup]
                 │                              │
                 ▼                              │
        [Explicit Go Logic]                     │
                 │                              │
                 ▼                              ▼
      [In-place Indicator Computations] ────► [Cache Hit / Write-Back]
```

### Data Pipeline Details
1. **Data Sourcing**: The `DataProvider` acts as the unified repository interface. It delegates to Yahoo Finance (parsing standard HTTP CSV streams dynamically) or Alpha Vantage.
2. **Rate Limiting**: Custom decorator mapping standard `net/http` calls to a thread-safe Token-Bucket rate-limiter, preventing API blocks on concurrent agent queries.
3. **Look-Ahead Bias Filter**: All retrieved historical lines are parsed sequentially. If a record's date exceeds the current running `tradeDate`, it is immediately discarded. This guarantees agents only observe prices prior to or on the decision date.
4. **Indicator Computations**: Technical analysis calculations run directly on float slices in-place. By avoiding repeated slice creation, the engine operates in sub-microseconds with zero heap allocations on the hot path.
5. **Dynamic Indicator Resolver**: Strings like `"close_50_sma"` or `"volume_14_ema"` are parsed at runtime using explicit split logic and mapped to compiler-optimized routines without reflection.

---

## 2. Go Interfaces & Struct Definitions

```go
package dataflow

import (
	"context"
	"time"
)

// Candle represents an individual OHLCV candlestick bar.
type Candle struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

// DataProvider represents the concurrency-safe contract for historical and fundamental data.
type DataProvider interface {
	// FetchOHLCV retrieves historical price bars for the given ticker,
	// filtering out any records with timestamps strictly greater than the tradeDate.
	FetchOHLCV(ctx context.Context, ticker string, start, end time.Time, tradeDate time.Time) ([]Candle, error)

	// FetchFundamentals retrieves structured company overview statements.
	FetchFundamentals(ctx context.Context, ticker string, tradeDate time.Time) (string, error)
}
```

```go
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

// MACD calculates standard MACD parameters in-place, avoiding dynamic slice allocation inside execution paths.
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
```

```go
package indicators

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"dataflow"
)

// IndicatorCache is a thread-safe caching/memoization layer for calculated metrics.
type IndicatorCache struct {
	mu    sync.RWMutex
	store map[string]float64
}

func NewIndicatorCache() *IndicatorCache {
	return &IndicatorCache{
		store: make(map[string]float64),
	}
}

func (c *IndicatorCache) Get(ticker, indicator string, date time.Time) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", ticker, indicator, date.Format("2006-01-02"))
	val, ok := c.store[key]
	return val, ok
}

func (c *IndicatorCache) Put(ticker, indicator string, date time.Time, val float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", ticker, indicator, date.Format("2006-01-02"))
	c.store[key] = val
}

// DynamicIndicatorResolver translates string indicator tool arguments into direct execution paths.
type DynamicIndicatorResolver struct {
	cache *IndicatorCache
}

func NewDynamicIndicatorResolver(cache *IndicatorCache) *DynamicIndicatorResolver {
	return &DynamicIndicatorResolver{cache: cache}
}

// Resolve matches and parses requested indicator string (e.g. "close_50_sma") at runtime without reflection.
func (r *DynamicIndicatorResolver) Resolve(ctx context.Context, candles []dataflow.Candle, ticker, indicatorStr string, tradeDate time.Time) (float64, error) {
	if val, ok := r.cache.Get(ticker, indicatorStr, tradeDate); ok {
		return val, nil
	}

	parts := strings.Split(indicatorStr, "_")
	if len(parts) < 3 {
		return 0, fmt.Errorf("invalid indicator configuration format: %s", indicatorStr)
	}

	targetMetric := parts[0] // e.g. "close", "volume"
	periodStr := parts[1]    // e.g. "50"
	indType := parts[2]      // e.g. "sma", "ema", "rsi"

	period, err := strconv.Atoi(periodStr)
	if err != nil {
		return 0, fmt.Errorf("invalid indicator period parsing: %s", periodStr)
	}

	prices := make([]float64, len(candles))
	for i, c := range candles {
		switch targetMetric {
		case "close":
			prices[i] = c.Close
		case "open":
			prices[i] = c.Open
		case "high":
			prices[i] = c.High
		case "low":
			prices[i] = c.Low
		case "volume":
			prices[i] = c.Volume
		default:
			return 0, fmt.Errorf("unsupported target price metric: %s", targetMetric)
		}
	}

	out := make([]float64, len(prices))
	switch indType {
	case "sma":
		if err := SMA(prices, period, out); err != nil {
			return 0, err
		}
	case "ema":
		if err := EMA(prices, period, out); err != nil {
			return 0, err
		}
	case "rsi":
		if err := RSI(prices, period, out); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unsupported indicator algorithm type: %s", indType)
	}

	result := out[len(out)-1]
	r.cache.Put(ticker, indicatorStr, tradeDate, result)
	return result, nil
}
```

---

## 3. Dynamic Indicators Math Derivations

To support complex technical trading agents, we specify three mathematical indicators with strict mathematical derivations, step boundaries, error checking, and zero-allocation slice mutation signatures.

### 3.1. Money Flow Index (MFI)

The Money Flow Index (MFI) is a momentum indicator that measures the inflow and outflow of money into an asset over a specific period, factoring in both volume and price changes.

#### Mathematical Derivations
1. **Typical Price ($\text{TP}_t$)** for a given day $t$ is calculated as the arithmetic mean of High, Low, and Close:
   $$\text{TP}_t = \frac{\text{High}_t + \text{Low}_t + \text{Close}_t}{3}$$
2. **Raw Money Flow ($\text{RMF}_t$)** scales Typical Price by the Volume of trading:
   $$\text{RMF}_t = \text{Typical Price}_t \times \text{Volume}_t$$
3. **Positive Money Flow ($\text{PMF}_t$)** and **Negative Money Flow ($\text{NMF}_t$)** are calculated based on price movements relative to the previous day:
   $$\text{PMF}_t = \begin{cases} \text{RMF}_t & \text{if } \text{TP}_t > \text{TP}_{t-1} \\ 0 & \text{otherwise} \end{cases}$$
   $$\text{NMF}_t = \begin{cases} \text{RMF}_t & \text{if } \text{TP}_t < \text{TP}_{t-1} \\ 0 & \text{otherwise} \end{cases}$$
4. **Money Flow Ratio ($\text{MFR}_t$)** is the sum of Positive Money Flow over the selected lookback period $P$ divided by the sum of Negative Money Flow over $P$:
   $$\text{MFR}_t = \frac{\sum_{i=0}^{P-1} \text{PMF}_{t-i}}{\sum_{i=0}^{P-1} \text{NMF}_{t-i}}$$
5. **Money Flow Index ($\text{MFI}_t$)** scales the ratio to a 0–100 boundary:
   $$\text{MFI}_t = 100 - \frac{100}{1 + \text{MFR}_t}$$

#### Loop Step Boundaries & In-Place Implementation
* **Valid Boundaries**: First valid Typical Price starts at index $0$. Money flow dynamics comparison can start at index $1$. The cumulative period sums $\text{PMF}$ and $\text{NMF}$ require $P$ intervals, which requires $P+1$ bars (indices $0$ to $P$). Thus, the first valid MFI output resides at index $P$.
* **Zero Allocation Signature**: To eliminate heap allocations, the caller must supply two pre-allocated scratch slices `pmfBuf` and `nmfBuf` of size equal to the candle history.

```go
// MFI calculates the Money Flow Index in-place with zero heap allocations on the hot path.
// high, low, close, and volume must be of equal length.
// out, pmfBuf, and nmfBuf must have capacities at least equal to len(close).
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
		if tp > tpPrev {
			pmfBuf[i] = rmf
			nmfBuf[i] = 0.0
		} else if tp < tpPrev {
			pmfBuf[i] = 0.0
			nmfBuf[i] = rmf
		} else {
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
```

### 3.2. Average True Range (ATR)

The Average True Range (ATR) is a volatility metric showing the average trading range of an asset over a moving period.

#### Mathematical Derivations
1. **True Range ($\text{TR}_t$)** is the greatest of:
   - Current High minus current Low: $\text{High}_t - \text{Low}_t$
   - Absolute value of current High minus previous Close: $\left|\text{High}_t - \text{Close}_{t-1}\right|$
   - Absolute value of current Low minus previous Close: $\left|\text{Low}_t - \text{Close}_{t-1}\right|$
   $$\text{TR}_t = \max\left(\text{High}_t - \text{Low}_t, \left|\text{High}_t - \text{Close}_{t-1}\right|, \left|\text{Low}_t - \text{Close}_{t-1}\right|\right)$$
2. **Average True Range ($\text{ATR}_t$)** is initialized as the simple average of $\text{TR}$ over the first $P$ bars:
   $$\text{ATR}_P = \frac{1}{P} \sum_{i=1}^{P} \text{TR}_i$$
3. Subsequent periods use Wilder’s smoothing method (essentially an EMA with $\alpha = 1/P$):
   $$\text{ATR}_t = \frac{\text{ATR}_{t-1} \times (P - 1) + \text{TR}_t}{P}$$

#### Loop Step Boundaries & In-Place Implementation
* **Valid Boundaries**: True Range compares current highs/lows with the previous day's close price, meaning index calculation starts at $t = 1$. The first ATR seeding completes at index $P$ (requires $P$ true range points). The remaining series calculations execute from index $P+1$ to the final element.
* **Zero Allocation Signature**: Requires a pre-allocated true range buffer `trBuf`.

```go
// ATR calculates the Average True Range in-place.
// high, low, close must be of equal length.
// trBuf and out must have capacity at least equal to len(close).
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
```

### 3.3. Bollinger Bands

Bollinger Bands consist of a middle band with two accompanying outer bands, measuring relative highness or lowness of price levels.

#### Mathematical Derivations
1. **Middle Band ($\text{MB}_t$)** is the Simple Moving Average of the Close prices over period $P$:
   $$\text{MB}_t = \text{SMA}(\text{Close}, P)_t = \frac{1}{P} \sum_{i=0}^{P-1} \text{Close}_{t-i}$$
2. **Standard Deviation ($\sigma_t$)** is computed on the trailing Close prices relative to the current Middle Band:
   $$\sigma_t = \sqrt{\frac{1}{P} \sum_{i=0}^{P-1} \left(\text{Close}_{t-i} - \text{MB}_t\right)^2}$$
3. **Upper Band ($\text{UB}_t$)** and **Lower Band ($\text{LB}_t$)** scale by standard deviation multiplier $K$:
   $$\text{UB}_t = \text{MB}_t + (K \times \sigma_t)$$
   $$\text{LB}_t = \text{MB}_t - (K \times \sigma_t)$$

#### Loop Step Boundaries & In-Place Implementation
* **Valid Boundaries**: SMA cannot be computed until $t = P-1$. Thus, the first valid Bollinger Band outputs start at index $P-1$.
* **Numerical Stability**: Standard sliding variance equations ($\sum x^2 - \frac{(\sum x)^2}{N}$) are susceptible to floating-point drift and catastrophic cancellation. We implement a local two-pass rolling slice to guarantee absolute numerical safety and correctness.

```go
// BollingerBands calculates Mid, Upper, and Lower Bands in-place with strict numerical stability.
// close must be of at least period length.
// midBand, upperBand, and lowerBand must have capacity of at least len(close).
func BollingerBands(close []float64, period int, k float64, midBand, upperBand, lowerBand []float64) error {
	n := len(close)
	if period <= 0 {
		return errors.New("Bollinger Bands period must be greater than zero")
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
```

---

## 4. SIMD-Friendly Optimization Strategies

High-Performance mathematical operations in Go rely heavily on how the CPU architecture processes blocks of numerical elements. In financial microservices, performance is dominated by L1/L2 cache spatial locality and vectorization capabilities.

### 4.1. Struct of Arrays (SoA) vs. Array of Structs (AoS)

To maximize memory alignment and cache efficiency, we deliberately layout our tick-level indicators using **Struct of Arrays (SoA)** instead of the traditional **Array of Structs (AoS)**.

| Layout Pattern | Representation | Memory Alignment | L1/L2 Cache Spatial Locality |
| :--- | :--- | :--- | :--- |
| **AoS (Traditional)** | `[]Candle` where `Candle` houses OHLCV fields. | Interleaved. High, Low, Close are interspersed with timestamps. | **Poor.** Iterating close prices loads unnecessary high/low/volume data into cache lines. |
| **SoA (SIMD Optimized)** | `CandleSeries` with distinct flat `[]float64` slices. | Flat and Contiguous. | **Outstanding.** Loading `Close[i]` loads consecutive close prices. A 64-byte cache line holds exactly 8 sequential `float64` elements. |

```go
// CandleSeries uses the Struct of Arrays (SoA) layout.
// Flat float slices are strictly contiguous in memory, aligning perfectly with CPU cache lines.
type CandleSeries struct {
	Time   []time.Time
	Open   []float64
	High   []float64
	Low    []float64
	Close  []float64
	Volume []float64
}
```

### 4.2. Compiler Bounds Check Elimination (BCE)

By default, the Go Compiler (`gc`) emits bounds-checking instructions (`runtime.panicIndex`) for every indexing slice access to guarantee safety. In tight loop operations (such as moving indicators), these branching statements break CPU pipelining and block loop autovectorization.

#### Go BCE Pattern Rules:
1. **Slice Capacity Anchoring**: By verifying the maximum bounds of a slice using a single dummy read *before* entering the loop, the compiler proves that any indexing access within the loop boundaries (`0 <= i < len`) is physically safe.
2. **Explicit Slice Matching**: Sub-slicing input data to equal bounds before loop entry ensures the compiler doesn't generate branch conditions inside.

#### Non-BCE vs BCE Compilation Profiles:

```go
// WITHOUT BCE (Compiles with index bounds panic branches in the loop body)
func ComputeSMA_Standard(prices []float64, out []float64, period int) {
	for i := period; i < len(prices); i++ {
		// Compiler generates bounds checks for both prices[i] and out[i] on every iteration!
		out[i] = prices[i] * 1.5
	}
}

// WITH BCE (Optimized - Bounds checks completely removed from loop body)
func ComputeSMA_Optimized(prices []float64, out []float64, period int) {
	n := len(prices)
	if n == 0 || len(out) < n {
		return
	}
	// BCE Hint: Single terminal check proves safety for all index accesses < n
	_ = prices[n-1]
	_ = out[n-1]

	for i := period; i < n; i++ {
		// Absolutely zero bounds-check instructions inside. Enjoys clean register allocation and pipelining!
		out[i] = prices[i] * 1.5
	}
}
```

#### Vectorization Hints:
To encourage Go's SSA compiler to autovectorize or unroll loops:
- Maintain simple indexing loops: use `for i := 0; i < len; i++` rather than complex pointer math or iterator mutations.
- Avoid branching operations (`if`/`else`) in the hottest indicator loops.
- Use basic constants or compile-time variables to simplify the loop step invariant calculations.

---

## 5. Robust CSV Stream Tokenization

Standard Python implementations often ingest an entire historical CSV file into memory as a string, then parse it into a large DataFrame. For long histories or continuous symbol sweeps, this creates severe garbage collector pressure and triggers heap allocation spikes.

In Go, we stream raw data from the Yahoo Finance HTTP socket directly and tokenize columns in a single pass with **zero heap allocations on the hot path**.

```
[TCP Socket Input] ────► [bufio.Reader (64KB Segment)] ────► [Line Buffer (ReadSlice)]
                                                                  │
                                                                  ▼
                                                      [Zero-Alloc Column Hunter]
                                                                  │ (Scan index of commas)
                                                                  ▼
                                                      [Look-Ahead Bias Filter]
                                                                  │ (If timestamp <= tradeDate)
                                                                  ▼
                                                      [Contiguous []Candle Append]
```

### Stream Tokenizer Implementation

```go
package dataflow

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// YahooFinanceCSVReader handles zero-allocation-on-hot-path CSV ingestion.
type YahooFinanceCSVReader struct {
	client *http.Client
}

func NewYahooFinanceCSVReader(client *http.Client) *YahooFinanceCSVReader {
	return &YahooFinanceCSVReader{client: client}
}

// FetchAndStreamOHLCV retrieves historical prices dynamically without loading the full raw file into memory.
func (r *YahooFinanceCSVReader) FetchAndStreamOHLCV(ctx context.Context, url string, tradeDate time.Time) ([]Candle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d", resp.StatusCode)
	}

	// Pre-allocate slice capacity to prevent multiple array re-allocations
	candles := make([]Candle, 0, 512)
	reader := bufio.NewReaderSize(resp.Body, 64*1024) // 64KB buffer for efficient socket reads

	// Skip standard Yahoo header line ("Date,Open,High,Low,Close,Adj Close,Volume")
	header, err := reader.ReadSlice('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV header: %w", err)
	}
	_ = header

	var lineCount int
	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					break
				}
			} else {
				return nil, fmt.Errorf("failed reading stream line %d: %w", lineCount, err)
			}
		}
		lineCount++

		// Clean carriage return bytes
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}

		if len(line) == 0 {
			continue
		}

		// Tokenize columns using zero-allocation comma index scans
		candle, valid, parseErr := parseCSVRowFast(line, tradeDate)
		if parseErr != nil {
			// In production, log warning and continue so a malformed row doesn't crash the pipeline
			continue
		}
		if !valid {
			// Safely pruned by the look-ahead bias filter
			continue
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

// parseCSVRowFast parses fields without allocating strings or string slices on the heap.
func parseCSVRowFast(line []byte, tradeDate time.Time) (Candle, bool, error) {
	var candle Candle
	var colIdx int
	start := 0
	n := len(line)

	var dateVal time.Time
	var oVal, hVal, lVal, cVal, vVal float64

	for i := 0; i <= n; i++ {
		if i == n || line[i] == ',' {
			field := line[start:i]
			start = i + 1

			switch colIdx {
			case 0: // Date Column ("YYYY-MM-DD")
				if len(field) != 10 {
					return candle, false, fmt.Errorf("invalid date field length: %s", string(field))
				}
				d, err := time.Parse("2006-01-02", string(field))
				if err != nil {
					return candle, false, fmt.Errorf("failed to parse date: %w", err)
				}
				dateVal = d
				// CRITICAL LOOK-AHEAD BIAS FILTER: Compare immediately and reject row.
				if d.After(tradeDate) {
					return candle, false, nil // Row discarded; completely prevents future data leak.
				}
			case 1: // Open
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("open parse error: %w", err)
				}
				oVal = val
			case 2: // High
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("high parse error: %w", err)
				}
				hVal = val
			case 3: // Low
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("low parse error: %w", err)
				}
				lVal = val
			case 4: // Close
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("close parse error: %w", err)
				}
				cVal = val
			case 5: // Adj Close (Skipped)
				// We skip Adj Close as standard closes are utilized for math logic.
			case 6: // Volume
				val, err := strconv.ParseFloat(string(field), 64)
				if err != nil {
					return candle, false, fmt.Errorf("volume parse error: %w", err)
				}
				vVal = val
			}
			colIdx++
		}
	}

	if colIdx < 7 {
		return candle, false, fmt.Errorf("incomplete columns: expected 7, parsed %d", colIdx)
	}

	candle = Candle{
		Time:   dateVal,
		Open:   oVal,
		High:   hVal,
		Low:    lVal,
		Close:  cVal,
		Volume: vVal,
	}

	return candle, true, nil
}
```

---

## 6. Token Bucket Rate Limiter with Jitter

Financial APIs enforce strict rate-limiting caps. When executing multi-symbol sweeps, concurrent requests will trigger immediate rate drop penalties. To resolve this, we implement a highly efficient, thread-safe **Token Bucket Rate Limiter** coupled with a resilient HTTP client configured with **Exponential Backoff and Full Jitter**.

```
                           [Resilient HTTP Client]
                                     │
                                     ▼
                        [Token Bucket Rate Limiter]
                         /                       \
        Tokens Available?                         No Tokens?
               /                                       \
             YES                                        NO
             /                                           \
    [Execute HTTP Call]                        [Context Active Wait]
             │                                     (Poll every 100ms)
    Response Status?
    ┌────────┴────────┐
 200 OK          429 / 503
    │                 │
 [Return]      [Calculate Jitter Backoff]
                      │
            Sleep = U(0, Min(Max, Base * 2^attempt))
                      │
                   [Retry]
```

### Thread-Safe Rate Limiter Implementation

```go
package rate

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// TokenBucket represents a concurrency-safe rate limiter.
type TokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64 // Tokens generated per second
	lastRefill time.Time
}

// NewTokenBucket instantiates an active bucket.
func NewTokenBucket(capacity, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow evaluates and consumes a token if available.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// refill calculates incremental tokens based on time elapsed since last check.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	tb.tokens = math.Min(tb.capacity, tb.tokens+(elapsed*tb.refillRate))
}

// ResilientHTTPClient is a robust decorator wrapper around a standard net/http client.
type ResilientHTTPClient struct {
	client      *http.Client
	limiter     *TokenBucket
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	rng         *rand.Rand
	rngMu       sync.Mutex
}

func NewResilientHTTPClient(client *http.Client, limiter *TokenBucket, maxRetries int, base, max time.Duration) *ResilientHTTPClient {
	return &ResilientHTTPClient{
		client:      client,
		limiter:     limiter,
		maxRetries:  maxRetries,
		baseBackoff: base,
		maxBackoff:  max,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Do executes request execution under token restriction and handles HTTP 429/503 limits dynamically.
func (c *ResilientHTTPClient) Do(req *http.Request) (*http.Response, error) {
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// Acquire rate limiter permission
		for {
			if c.limiter.Allow() {
				break
			}
			// Active wait block
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(100 * time.Millisecond):
			}
		}

		resp, err := c.client.Do(req)
		if err != nil {
			if attempt == c.maxRetries {
				return nil, err
			}
			c.sleepJitter(attempt, req.Context())
			continue
		}

		// Handle server throttling or service drops
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close() // Immediately close body to conserve connections
			if attempt == c.maxRetries {
				return nil, fmt.Errorf("rate limits exceeded after %d retries: status %d", c.maxRetries, resp.StatusCode)
			}
			c.sleepJitter(attempt, req.Context())
			continue
		}

		return resp, nil
	}
	return nil, errors.New("unexpected error in HTTP retry matrix")
}

// sleepJitter sleeps for a duration defined by Full Jitter algorithm:
// Sleep = Uniform(0, Min(maxBackoff, baseBackoff * 2^attempt))
func (c *ResilientHTTPClient) sleepJitter(attempt int, ctx context.Context) {
	// Exponential scaling factor: 2^attempt
	factor := int64(1) << uint(attempt)
	temp := float64(c.baseBackoff) * float64(factor)
	if temp > float64(c.maxBackoff) {
		temp = float64(c.maxBackoff)
	}

	c.rngMu.Lock()
	jitter := c.rng.Float64() * temp
	c.rngMu.Unlock()

	sleepDur := time.Duration(jitter)

	select {
	case <-ctx.Done():
	case <-time.After(sleepDur):
	}
}
```

---

## 7. Step-by-Step Implementation Sub-plan

- [x] **1. Scaffolding & Types**: Define core struct entities (`Candle`) and SoA structural representations in `internal/data/provider.go`.
- [ ] **2. Low-Overhead Yahoo Finance CSV Stream Client**:
  - Construct the zero-allocation CSV reader in `internal/data/yfinance_reader.go` utilising the pre-allocated, dynamic single-pass `parseCSVRowFast` function.
  - Implement direct look-ahead bias filter execution by parsing the date column immediately at index zero and pruning.
- [ ] **3. Resilient Token Bucket Rate Limiter**:
  - Create the concurrency-safe `TokenBucket` in `internal/rate/limiter.go` using a lock-based lazy-refilling approach.
  - Write the `ResilientHTTPClient` decorator implementing exponential backoff with full jitter to automatically retry requests upon receipt of 429/503 status codes.
- [ ] **4. Pure-Go Mathematical Indicators**:
  - Code zero-allocation, in-place slice mutation functions for `SMA`, `EMA`, `RSI`, `MACD`, `MFI`, `ATR`, and `BollingerBands` in `internal/data/indicators.go`.
  - Add standard unit tests validating indicator calculations against exact results from Python `stockstats` and ensure compiler autovectorization by checking with `-gcflags="-m -g"`.
- [ ] **5. Dynamic Resolution**:
  - Implement the `DynamicIndicatorResolver` using string parsing trees inside `internal/data/resolver.go`.
  - Create the `IndicatorCache` with read/write mutex memoization.

---

## 8. Idiomatic Trade-offs

### High Performance In-Place Mutation over GC Allocation
* **Python Pattern**: Pandas dataframes recreate entire columns and sub-frames on every function query, stressing the garbage collector.
* **Go Pattern**: Passing `out []float64` slices pre-allocated at the function stack boundary completely avoids allocating heap memory. Calculations execute in nanoseconds, maintaining high instruction-cache efficiency.

### Struct of Arrays (SoA) over Array of Structs (AoS)
* **AoS Pattern**: Easy to conceptualize and map (a list of candle objects), but memory is scattered, generating low L1 cache density.
* **SoA Pattern**: Introduces minor overhead when organizing fields, but provides absolute speed in vector loops, fitting sequential floats perfectly into CPU registers.

### Explicit Parser over Attribute Reflection
* **Python Pattern**: The Python indicator system uses metaprogramming reflection to bind functions dynamically at runtime:
  `df[indicator]` (e.g. `df["close_50_sma"]`).
* **Go Pattern**: The parser splits strings explicitly and maps targets via static conditionals. This provides strict compilation validation, enforces expected types, and prevents production environment panics.

### Lazy-Refilling Rate Limiter over Blocking Refill Channels
* **Channel Refill Pattern**: A separate background go-routine updates the bucket at constant intervals using `time.Ticker`. While simple, it requires an active background routine per client, introducing scheduling overhead and goroutine leaks if client instances are not explicitly shut down.
* **Lazy-Refill Pattern**: Computing the refill amount dynamically upon each ticket request via `time.Since(lastRefill)` operates with zero background thread overhead and achieves superior performance with absolute safety.
