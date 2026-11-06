package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// PriceAPIResponse is the response from gold-api.com /price/XAU endpoint.
// This is a free, no-auth, no-rate-limit API for real-time gold prices.
type PriceAPIResponse struct {
	Currency          string  `json:"currency"`
	Price             float64 `json:"price"`
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	UpdatedAt         string  `json:"updatedAt"`
	UpdatedAtReadable string  `json:"updatedAtReadable"`
}

const goldAPIURL = "https://api.gold-api.com/price/XAU"

// FetchLiveGoldPrice gets the current real-time XAU/USD price from gold-api.com (free, no API key).
func FetchLiveGoldPrice() (*PriceAPIResponse, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	resp, err := client.Get(goldAPIURL)
	if err != nil {
		return nil, fmt.Errorf("requesting gold price: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gold API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result PriceAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing gold price response: %w", err)
	}

	return &result, nil
}

// AppendLiveCandle fetches the current real-time price and appends it as a
// live (forming) candle to the end of the historical records.
// The candle's open = previous close, and its datetime = current 15-min slot start.
func AppendLiveCandle(records []PriceRecord, interval string) ([]PriceRecord, error) {
	price, err := FetchLiveGoldPrice()
	if err != nil {
		return records, fmt.Errorf("fetching live price: %w", err)
	}

	now := time.Now().In(time.FixedZone("ICT", 7*3600))

	// Round down to the nearest candle boundary
	candleMinutes := candleIntervalMinutes(interval)
	slotMinute := (now.Minute() / candleMinutes) * candleMinutes
	slotStart := time.Date(now.Year(), now.Month(), now.Day(),
		now.Hour(), slotMinute, 0, 0, now.Location())

	// Use previous close as open for the forming candle
	var open float64
	if len(records) > 0 {
		open = records[len(records)-1].Close
	} else {
		open = price.Price
	}

	liveCandle := PriceRecord{
		Date:  slotStart.Format("2006-01-02 15:04:05") + " ⟳", // mark as live
		Open:  open,
		High:  price.Price,
		Low:   price.Price,
		Close: price.Price,
	}
	if open > price.Price {
		liveCandle.Low = price.Price
		liveCandle.High = open
	} else {
		liveCandle.High = price.Price
		liveCandle.Low = open
	}

	fmt.Printf("Live candle (forming): %s  O:%.2f H:%.2f L:%.2f C:%.2f\n",
		slotStart.Format("15:04"), liveCandle.Open, liveCandle.High, liveCandle.Low, liveCandle.Close)

	return append(records, liveCandle), nil
}

// candleIntervalMinutes converts interval strings like "15min","30min","1h","1day" to minutes.
func candleIntervalMinutes(interval string) int {
	switch interval {
	case "1min":
		return 1
	case "5min":
		return 5
	case "15min":
		return 15
	case "30min":
		return 30
	case "45min":
		return 45
	case "1h":
		return 60
	case "2h":
		return 120
	case "4h":
		return 240
	case "1day":
		return 1440
	default:
		return 15
	}
}
type TwelveDataResponse struct {
	Meta struct {
		Symbol   string `json:"symbol"`
		Interval string `json:"interval"`
		Currency string `json:"currency"`
	} `json:"meta"`
	Values []struct {
		Datetime string `json:"datetime"`
		Open     string `json:"open"`
		High     string `json:"high"`
		Low      string `json:"low"`
		Close    string `json:"close"`
		Volume   string `json:"volume"`
	} `json:"values"`
	Status string `json:"status"`
	Code   int    `json:"code"`
	Message string `json:"message"`
}

// FetchHistoricalOHLC fetches historical OHLC candles for XAU/USD from Twelve Data.
// interval examples: "15min", "30min", "1h", "1day".
// outputsize: number of candles (1–5000).
func FetchHistoricalOHLC(symbol, interval string, outputsize int) ([]PriceRecord, error) {
	apiKey := os.Getenv("TWELVEDATA_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("TWELVEDATA_API_KEY not set. Get a free key at https://twelvedata.com/pricing")
	}

	url := fmt.Sprintf("https://api.twelvedata.com/time_series?symbol=%s&interval=%s&outputsize=%d&apikey=%s&order=asc&timezone=Asia%%2FHo_Chi_Minh",
		symbol, interval, outputsize, apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("requesting historical data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Twelve Data API returned status %d: %s", resp.StatusCode, string(body))
	}

	var tdResp TwelveDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&tdResp); err != nil {
		return nil, fmt.Errorf("parsing Twelve Data response: %w", err)
	}

	if tdResp.Status == "error" || tdResp.Code != 0 {
		return nil, fmt.Errorf("Twelve Data error: %s", tdResp.Message)
	}

	if len(tdResp.Values) == 0 {
		return nil, fmt.Errorf("no historical data returned")
	}

	records := make([]PriceRecord, 0, len(tdResp.Values))
	for _, v := range tdResp.Values {
		open, _ := strconv.ParseFloat(v.Open, 64)
		high, _ := strconv.ParseFloat(v.High, 64)
		low, _ := strconv.ParseFloat(v.Low, 64)
		close, _ := strconv.ParseFloat(v.Close, 64)
		vol, _ := strconv.ParseFloat(v.Volume, 64)

		records = append(records, PriceRecord{
			Date:   v.Datetime,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: vol,
		})
	}

	return records, nil
}

// FetchLiveOHLCHistory collects real-time gold prices and builds proper OHLC candles.
// Each candle spans `candleDuration` (e.g. 15 minutes). Within each candle, the price
// is polled every `pollInterval` (e.g. 30 seconds) to capture open/high/low/close.
//
// Example: 5 candles × 15min, polling every 30s = 5 candles from 75 minutes of data.
func FetchLiveOHLCHistory(numCandles int, candleDuration, pollInterval time.Duration) ([]PriceRecord, error) {
	records := make([]PriceRecord, 0, numCandles)

	fmt.Printf("=== LIVE MODE: Collecting %d OHLC candles (%v each, polling every %v) ===\n",
		numCandles, candleDuration, pollInterval)
	fmt.Printf("Total time: ~%v\n\n", time.Duration(numCandles)*candleDuration)

	for c := 0; c < numCandles; c++ {
		candleStart := time.Now()
		var open, high, low, closePrice float64
		var firstPrice float64
		sampleCount := 0

		// Collect samples within this candle window
		for {
			now := time.Now()
			elapsed := now.Sub(candleStart)
			if elapsed >= candleDuration {
				break
			}

			price, err := FetchLiveGoldPrice()
			if err != nil {
				fmt.Printf("  [candle %d] fetch error: %v\n", c+1, err)
				time.Sleep(pollInterval)
				continue
			}

			p := price.Price
			sampleCount++

			if sampleCount == 1 {
				// First sample of this candle
				open = p
				high = p
				low = p
				firstPrice = p
			} else {
				if p > high {
					high = p
				}
				if p < low {
					low = p
				}
			}
			closePrice = p

			remaining := candleDuration - elapsed
			fmt.Printf("  [candle %d/%d] %s — $%.2f (O:%.2f H:%.2f L:%.2f C:%.2f, %d samples, %v left)\n",
				c+1, numCandles, now.Format("15:04:05"), p, open, high, low, closePrice,
				sampleCount, remaining.Round(time.Second))

			// Sleep until next poll, but don't overshoot candle end
			sleep := pollInterval
			if remaining < pollInterval {
				sleep = remaining
			}
			if sleep > 0 {
				time.Sleep(sleep)
			}
		}

		if sampleCount == 0 {
			// Fallback: at least get one price
			price, err := FetchLiveGoldPrice()
			if err != nil {
				return nil, fmt.Errorf("candle %d: no samples collected: %w", c+1, err)
			}
			open = price.Price
			high = price.Price
			low = price.Price
			closePrice = price.Price
		}

		records = append(records, PriceRecord{
			Date:  candleStart.Format("2006-01-02 15:04"),
			Open:  open,
			High:  high,
			Low:   low,
			Close: closePrice,
		})

		fmt.Printf("  ✓ Candle %d complete: O:%.2f H:%.2f L:%.2f C:%.2f (%d samples)\n\n",
			c+1, open, high, low, closePrice, sampleCount)

		// Use firstPrice for unused warning
		_ = firstPrice
	}

	return records, nil
}
