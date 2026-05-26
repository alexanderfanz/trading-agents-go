<!-- DONE -->

![medium](https://www.gstatic.com/codereviewagent/medium-priority.svg)

in `internal/indicators/indicators_test.go`
Comment on lines +455 to +458

Comparing floating-point numbers directly using `!=` can lead to flaky tests due to precision issues. It is safer to use `math.Abs` with a small tolerance (e.g., `1e-9`), consistent with other tests in this file.

```go
		if err != nil || math.Abs(valDynSMA-150.0) > 1e-9 { // last 3 closes: 102+47=149, 102+48=150, 102+49=151 => avg = 150
```