package data

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type YFinanceClient struct {
	client *http.Client
}

func NewYFinanceClient() *YFinanceClient {
	return &YFinanceClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// FetchOHLCV downloads historical daily price data from Yahoo Finance and parses the CSV.
func (c *YFinanceClient) FetchOHLCV(ctx context.Context, symbol string, startDate, endDate string) ([]OHLCV, error) {
	t1, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date format: %w", err)
	}
	t2, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date format: %w", err)
	}

	u := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/download/%s?period1=%d&period2=%d&interval=1d&events=history&includeAdjustedClose=true",
		url.QueryEscape(strings.ToUpper(symbol)), t1.Unix(), t2.Unix())

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("yahoo finance error (status %d): %s", resp.StatusCode, string(body))
	}

	reader := csv.NewReader(resp.Body)
	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	headerMap := make(map[string]int)
	for i, h := range header {
		headerMap[strings.ToLower(h)] = i
	}

	var ohlcvList []OHLCV
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV row: %w", err)
		}

		o := OHLCV{
			Date: record[headerMap["date"]],
		}
		o.Open, _ = strconv.ParseFloat(record[headerMap["open"]], 64)
		o.High, _ = strconv.ParseFloat(record[headerMap["high"]], 64)
		o.Low, _ = strconv.ParseFloat(record[headerMap["low"]], 64)
		o.Close, _ = strconv.ParseFloat(record[headerMap["close"]], 64)
		o.Volume, _ = strconv.ParseFloat(record[headerMap["volume"]], 64)

		ohlcvList = append(ohlcvList, o)
	}

	return ohlcvList, nil
}

// getJSON performs a GET request with Yahoo user agent and parses into target
func (c *YFinanceClient) getJSON(ctx context.Context, urlStr string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("yahoo finance api error (status %d): %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// FetchFundamentals reads financial parameters via quoteSummary modules.
func (c *YFinanceClient) FetchFundamentals(ctx context.Context, symbol string) (string, error) {
	u := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=assetProfile,financialData,defaultKeyStatistics,summaryDetail",
		url.QueryEscape(strings.ToUpper(symbol)))

	var payload struct {
		QuoteSummary struct {
			Result []struct {
				AssetProfile struct {
					LongName string `json:"longName"`
					Sector   string `json:"sector"`
					Industry string `json:"industry"`
				} `json:"assetProfile"`
				FinancialData struct {
					TotalRevenue     map[string]interface{} `json:"totalRevenue"`
					GrossProfits     map[string]interface{} `json:"grossProfits"`
					Ebitda           map[string]interface{} `json:"ebitda"`
					ReturnOnEquity   map[string]interface{} `json:"returnOnEquity"`
					ReturnOnAssets   map[string]interface{} `json:"returnOnAssets"`
					DebtToEquity     map[string]interface{} `json:"debtToEquity"`
					CurrentRatio     map[string]interface{} `json:"currentRatio"`
					FreeCashflow     map[string]interface{} `json:"freeCashflow"`
					OperatingMargins map[string]interface{} `json:"operatingMargins"`
					ProfitMargins    map[string]interface{} `json:"profitMargins"`
				} `json:"financialData"`
				DefaultKeyStatistics struct {
					TrailingEps      map[string]interface{} `json:"trailingEps"`
					ForwardEps       map[string]interface{} `json:"forwardEps"`
					Beta             map[string]interface{} `json:"beta"`
					BookValue        map[string]interface{} `json:"bookValue"`
					PriceToBook      map[string]interface{} `json:"priceToBook"`
					PegRatio         map[string]interface{} `json:"pegRatio"`
					NetIncomeToCommon map[string]interface{} `json:"netIncomeToCommon"`
				} `json:"defaultKeyStatistics"`
				SummaryDetail struct {
					MarketCap          map[string]interface{} `json:"marketCap"`
					TrailingPE         map[string]interface{} `json:"trailingPE"`
					ForwardPE          map[string]interface{} `json:"forwardPE"`
					DividendYield      map[string]interface{} `json:"dividendYield"`
					FiftyTwoWeekHigh   map[string]interface{} `json:"fiftyTwoWeekHigh"`
					FiftyTwoWeekLow    map[string]interface{} `json:"fiftyTwoWeekLow"`
					FiftyDayAverage    map[string]interface{} `json:"fiftyDayAverage"`
					TwoHundredDayAvg   map[string]interface{} `json:"twoHundredDayAverage"`
				} `json:"summaryDetail"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"quoteSummary"`
	}

	if err := c.getJSON(ctx, u, &payload); err != nil {
		return "", err
	}

	if len(payload.QuoteSummary.Result) == 0 {
		return "", fmt.Errorf("no fundamentals data found for symbol %s", symbol)
	}

	res := payload.QuoteSummary.Result[0]
	var lines []string

	extractVal := func(m map[string]interface{}) string {
		if m == nil {
			return ""
		}
		if fmtVal, ok := m["fmt"]; ok && fmtVal != nil {
			return fmt.Sprintf("%v", fmtVal)
		}
		if rawVal, ok := m["raw"]; ok && rawVal != nil {
			return fmt.Sprintf("%v", rawVal)
		}
		return ""
	}

	addField := func(label, val string) {
		if val != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", label, val))
		}
	}

	addField("Name", res.AssetProfile.LongName)
	addField("Sector", res.AssetProfile.Sector)
	addField("Industry", res.AssetProfile.Industry)
	addField("Market Cap", extractVal(res.SummaryDetail.MarketCap))
	addField("PE Ratio (TTM)", extractVal(res.SummaryDetail.TrailingPE))
	addField("Forward PE", extractVal(res.SummaryDetail.ForwardPE))
	addField("PEG Ratio", extractVal(res.DefaultKeyStatistics.PegRatio))
	addField("Price to Book", extractVal(res.DefaultKeyStatistics.PriceToBook))
	addField("EPS (TTM)", extractVal(res.DefaultKeyStatistics.TrailingEps))
	addField("Forward EPS", extractVal(res.DefaultKeyStatistics.ForwardEps))
	addField("Dividend Yield", extractVal(res.SummaryDetail.DividendYield))
	addField("Beta", extractVal(res.DefaultKeyStatistics.Beta))
	addField("52 Week High", extractVal(res.SummaryDetail.FiftyTwoWeekHigh))
	addField("52 Week Low", extractVal(res.SummaryDetail.FiftyTwoWeekLow))
	addField("50 Day Average", extractVal(res.SummaryDetail.FiftyDayAverage))
	addField("200 Day Average", extractVal(res.SummaryDetail.TwoHundredDayAvg))
	addField("Revenue (TTM)", extractVal(res.FinancialData.TotalRevenue))
	addField("Gross Profit", extractVal(res.FinancialData.GrossProfits))
	addField("EBITDA", extractVal(res.FinancialData.Ebitda))
	addField("Net Income", extractVal(res.DefaultKeyStatistics.NetIncomeToCommon))
	addField("Profit Margin", extractVal(res.FinancialData.ProfitMargins))
	addField("Operating Margin", extractVal(res.FinancialData.OperatingMargins))
	addField("Return on Equity", extractVal(res.FinancialData.ReturnOnEquity))
	addField("Return on Assets", extractVal(res.FinancialData.ReturnOnAssets))
	addField("Debt to Equity", extractVal(res.FinancialData.DebtToEquity))
	addField("Current Ratio", extractVal(res.FinancialData.CurrentRatio))
	addField("Book Value", extractVal(res.DefaultKeyStatistics.BookValue))
	addField("Free Cash Flow", extractVal(res.FinancialData.FreeCashflow))

	header := fmt.Sprintf("# Company Fundamentals for %s\n", strings.ToUpper(symbol))
	header += fmt.Sprintf("# Data retrieved on: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	return header + strings.Join(lines, "\n"), nil
}

// FetchFinancialStatements fetches Balance Sheet, Cash Flow, or Income Statement and formats as CSV.
func (c *YFinanceClient) FetchFinancialStatements(ctx context.Context, symbol, statementType, frequency string) (string, error) {
	// Map statement type to quoteSummary module
	module := ""
	switch statementType {
	case "balance_sheet":
		if frequency == "quarterly" {
			module = "balanceSheetHistoryQuarterly"
		} else {
			module = "balanceSheetHistory"
		}
	case "cash_flow":
		if frequency == "quarterly" {
			module = "cashflowStatementHistoryQuarterly"
		} else {
			module = "cashflowStatementHistory"
		}
	case "income_statement":
		if frequency == "quarterly" {
			module = "incomeStatementHistoryQuarterly"
		} else {
			module = "incomeStatementHistory"
		}
	default:
		return "", fmt.Errorf("unknown statement type: %s", statementType)
	}

	u := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=%s",
		url.QueryEscape(strings.ToUpper(symbol)), module)

	var payload map[string]interface{}
	if err := c.getJSON(ctx, u, &payload); err != nil {
		return "", err
	}

	// Dynamic traversal of nested JSON maps to emulate statements
	qSummary, ok := payload["quoteSummary"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("quoteSummary not found")
	}
	results, ok := qSummary["result"].([]interface{})
	if !ok || len(results) == 0 {
		return "", fmt.Errorf("results empty")
	}

	resMap, ok := results[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid result format")
	}

	stmtContainer, ok := resMap[module].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("module %s not found in response", module)
	}

	listKey := ""
	switch statementType {
	case "balance_sheet":
		listKey = "balanceSheetStatements"
	case "cash_flow":
		listKey = "cashflowStatements"
	case "income_statement":
		listKey = "incomeStatementHistory"
	}

	stmts, ok := stmtContainer[listKey].([]interface{})
	if !ok || len(stmts) == 0 {
		return "", fmt.Errorf("no statements found under key %s", listKey)
	}

	// We format as a CSV table.
	// Headers: metric names. Columns: dates.
	// Gather all unique metric names and columns (dates).
	dates := []string{"Metric"}
	metrics := make(map[string]map[string]string) // metric -> date -> value
	var metricKeys []string
	seenMetrics := make(map[string]bool)

	for _, stmtVal := range stmts {
		stmt, ok := stmtVal.(map[string]interface{})
		if !ok {
			continue
		}

		// Find end date
		dateStr := "unknown"
		if dateObj, ok := stmt["endDate"].(map[string]interface{}); ok {
			if fmtDate, ok := dateObj["fmt"].(string); ok {
				dateStr = fmtDate
			}
		}
		dates = append(dates, dateStr)

		// Parse all key-values
		for k, v := range stmt {
			if k == "endDate" || k == "maxAge" {
				continue
			}

			valStr := "N/A"
			if valMap, ok := v.(map[string]interface{}); ok {
				if fmtVal, ok := valMap["fmt"].(string); ok {
					valStr = fmtVal
				} else if rawVal, ok := valMap["raw"]; ok && rawVal != nil {
					valStr = fmt.Sprintf("%v", rawVal)
				}
			}

			if !seenMetrics[k] {
				seenMetrics[k] = true
				metricKeys = append(metricKeys, k)
				metrics[k] = make(map[string]string)
			}
			metrics[k][dateStr] = valStr
		}
	}

	// Build CSV
	var csvBuilder strings.Builder
	writer := csv.NewWriter(&csvBuilder)

	// Write Dates Header
	_ = writer.Write(dates)

	for _, key := range metricKeys {
		row := []string{key}
		for i := 1; i < len(dates); i++ {
			d := dates[i]
			val := "N/A"
			if v, ok := metrics[key][d]; ok {
				val = v
			}
			row = append(row, val)
		}
		_ = writer.Write(row)
	}

	writer.Flush()

	header := fmt.Sprintf("# %s statements for %s (%s)\n", strings.Title(strings.ReplaceAll(statementType, "_", " ")), strings.ToUpper(symbol), frequency)
	header += fmt.Sprintf("# Data retrieved on: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	return header + csvBuilder.String(), nil
}

// FetchInsiderTransactions fetches insider transactions in CSV format.
func (c *YFinanceClient) FetchInsiderTransactions(ctx context.Context, symbol string) (string, error) {
	u := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=insiderTransactions",
		url.QueryEscape(strings.ToUpper(symbol)))

	var payload map[string]interface{}
	if err := c.getJSON(ctx, u, &payload); err != nil {
		return "", err
	}

	qSummary, ok := payload["quoteSummary"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("quoteSummary not found")
	}
	results, ok := qSummary["result"].([]interface{})
	if !ok || len(results) == 0 {
		return "", fmt.Errorf("results empty")
	}

	resMap, ok := results[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid result format")
	}

	insContainer, ok := resMap["insiderTransactions"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("insiderTransactions not found in response")
	}

	transactions, ok := insContainer["transactions"].([]interface{})
	if !ok || len(transactions) == 0 {
		return fmt.Sprintf("No insider transactions data found for symbol '%s'", symbol), nil
	}

	// We format as a CSV table
	var csvBuilder strings.Builder
	writer := csv.NewWriter(&csvBuilder)

	headers := []string{"Insider", "Relation", "Date", "Transaction", "Shares", "Value", "Price"}
	_ = writer.Write(headers)

	for _, txVal := range transactions {
		tx, ok := txVal.(map[string]interface{})
		if !ok {
			continue
		}

		extractField := func(k string) string {
			if v, ok := tx[k].(string); ok {
				return v
			}
			if v, ok := tx[k].(map[string]interface{}); ok {
				if fmtVal, ok := v["fmt"].(string); ok {
					return fmtVal
				}
				if rawVal, ok := v["raw"]; ok && rawVal != nil {
					return fmt.Sprintf("%v", rawVal)
				}
			}
			if v, ok := tx[k]; ok && v != nil {
				return fmt.Sprintf("%v", v)
			}
			return ""
		}

		insider := extractField("filerName")
		relation := extractField("filerRelation")
		dateVal := extractField("startDate") // transaction date
		txType := extractField("transactionText")
		shares := extractField("shares")
		value := extractField("value")
		price := extractField("value") // fallback or math

		row := []string{insider, relation, dateVal, txType, shares, value, price}
		_ = writer.Write(row)
	}

	writer.Flush()

	header := fmt.Sprintf("# Insider Transactions data for %s\n", strings.ToUpper(symbol))
	header += fmt.Sprintf("# Data retrieved on: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	return header + csvBuilder.String(), nil
}

// FetchNews gets recent articles for a specific stock symbol.
func (c *YFinanceClient) FetchNews(ctx context.Context, symbol string, startStr, endStr string, limit int) (string, error) {
	u := fmt.Sprintf("https://query2.finance.yahoo.com/v1/finance/search?q=%s&newsCount=%d&enableFuzzyQuery=true",
		url.QueryEscape(strings.ToUpper(symbol)), limit)

	var payload struct {
		News []struct {
			Title     string `json:"title"`
			Publisher string `json:"publisher"`
			Link      string `json:"link"`
			PubTime   int64  `json:"providerPublishTime"`
		} `json:"news"`
	}

	if err := c.getJSON(ctx, u, &payload); err != nil {
		return "", err
	}

	if len(payload.News) == 0 {
		return fmt.Sprintf("No news found for %s", symbol), nil
	}

	startT, _ := time.Parse("2006-01-02", startStr)
	endT, _ := time.Parse("2006-01-02", endStr)
	endT = endT.Add(24 * time.Hour) // extend end filter boundary to include full trading day

	var newsBuilder strings.Builder
	filteredCount := 0

	for _, art := range payload.News {
		pubTime := time.Unix(art.PubTime, 0)
		
		// Filter by date boundaries
		if !startT.IsZero() && pubTime.Before(startT) {
			continue
		}
		if !endT.IsZero() && pubTime.After(endT) {
			continue
		}

		newsBuilder.WriteString(fmt.Sprintf("### %s (source: %s)\n", art.Title, art.Publisher))
		newsBuilder.WriteString(fmt.Sprintf("Publish Time: %s\n", pubTime.Format("2006-01-02 15:04:05")))
		if art.Link != "" {
			newsBuilder.WriteString(fmt.Sprintf("Link: %s\n", art.Link))
		}
		newsBuilder.WriteString("\n")
		filteredCount++
	}

	if filteredCount == 0 {
		return fmt.Sprintf("No news found for %s between %s and %s", symbol, startStr, endStr), nil
	}

	header := fmt.Sprintf("## %s News, from %s to %s:\n\n", strings.ToUpper(symbol), startStr, endStr)
	return header + newsBuilder.String(), nil
}

// FetchGlobalNews fetches global macro headlines based on queries.
func (c *YFinanceClient) FetchGlobalNews(ctx context.Context, queries []string, currDate string, lookBackDays, limit int) (string, error) {
	currT, err := time.Parse("2006-01-02", currDate)
	if err != nil {
		return "", fmt.Errorf("invalid current date: %w", err)
	}
	startT := currT.AddDate(0, 0, -lookBackDays)

	seenTitles := make(map[string]bool)
	var allNews []string
	count := 0

	for _, q := range queries {
		if count >= limit {
			break
		}

		u := fmt.Sprintf("https://query2.finance.yahoo.com/v1/finance/search?q=%s&newsCount=%d&enableFuzzyQuery=true",
			url.QueryEscape(q), limit)

		var payload struct {
			News []struct {
				Title     string `json:"title"`
				Publisher string `json:"publisher"`
				Link      string `json:"link"`
				PubTime   int64  `json:"providerPublishTime"`
			} `json:"news"`
		}

		if err := c.getJSON(ctx, u, &payload); err != nil {
			continue // skip failing query silently to maintain fallback resiliency
		}

		for _, art := range payload.News {
			if count >= limit {
				break
			}

			title := strings.TrimSpace(art.Title)
			if title == "" || seenTitles[title] {
				continue
			}

			pubTime := time.Unix(art.PubTime, 0)
			// Apply look-ahead guard
			if pubTime.After(currT.Add(24 * time.Hour)) {
				continue
			}
			if pubTime.Before(startT) {
				continue
			}

			seenTitles[title] = true
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("### %s (source: %s)\n", title, art.Publisher))
			sb.WriteString(fmt.Sprintf("Publish Time: %s\n", pubTime.Format("2006-01-02 15:04:05")))
			if art.Link != "" {
				sb.WriteString(fmt.Sprintf("Link: %s\n", art.Link))
			}
			sb.WriteString("\n")
			
			allNews = append(allNews, sb.String())
			count++
		}
	}

	if len(allNews) == 0 {
		return fmt.Sprintf("No global news found for %s", currDate), nil
	}

	header := fmt.Sprintf("## Global Market News, from %s to %s:\n\n", startT.Format("2006-01-02"), currDate)
	return header + strings.Join(allNews, ""), nil
}
