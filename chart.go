package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ChartOptions controls how the HTML candlestick chart is generated.
type ChartOptions struct {
	Title    string
	Records  []PriceRecord
	OutPath  string
	AutoOpen bool
}

// generateChartHTML writes an HTML file with an MT5-style candlestick chart.
func generateChartHTML(opts ChartOptions) error {
	type candleData struct {
		Time  int64   `json:"time"`
		Open  float64 `json:"open"`
		High  float64 `json:"high"`
		Low   float64 `json:"low"`
		Close float64 `json:"close"`
	}

	candles := make([]candleData, 0, len(opts.Records))
	liveIdx := -1 // index of the forming candle (if any)
	for i, r := range opts.Records {
		if strings.Contains(r.Date, "⟳") {
			liveIdx = i
		}
		t, err := parseChartTime(r.Date)
		if err != nil {
			t = time.Now().Unix()
		}
		candles = append(candles, candleData{
			Time:  t,
			Open:  r.Open,
			High:  r.High,
			Low:   r.Low,
			Close: r.Close,
		})
	}

	candlesJSON, _ := json.Marshal(candles)
	html := chartTemplate(opts.Title, string(candlesJSON), liveIdx)

	if err := os.WriteFile(opts.OutPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("writing chart file: %w", err)
	}

	fmt.Printf("Chart saved: %s\n", opts.OutPath)

	if opts.AutoOpen {
		openBrowser(opts.OutPath)
	}

	return nil
}

// parseChartTime parses PriceRecord date strings into unix seconds.
// Strips trailing " ⟳" marker used for the live (forming) candle.
func parseChartTime(dateStr string) (int64, error) {
	// Strip live candle marker
	dateStr = strings.TrimSuffix(strings.TrimSpace(dateStr), "⟳")
	dateStr = strings.TrimSpace(dateStr)

	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		time.RFC3339,
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, dateStr)
		if err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("unparseable date: %s", dateStr)
}

// openBrowser opens the chart file in the default browser.
func openBrowser(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	url := "file://" + abs

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func chartTemplate(title, candlesJSON string, liveIdx int) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<script src="https://unpkg.com/lightweight-charts@4.1.3/dist/lightweight-charts.standalone.production.js"></script>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html, body { width: 100%%; height: 100%%; background: #000; overflow: hidden; }

  #header {
    position: absolute;
    top: 0; left: 0;
    padding: 8px 12px;
    z-index: 10;
    pointer-events: none;
    line-height: 1.4;
  }
  #header .symbol { font-family: Arial, sans-serif; font-size: 13px; font-weight: bold; color: #00bcd4; }
  #header .timeframe { font-family: Arial, sans-serif; font-size: 11px; color: #aaa; }

  #chart { width: 100%%; height: 100%%; }

  #tooltip {
    position: absolute; z-index: 100; pointer-events: none; display: none;
    background: rgba(0,0,0,0.85); border: 1px solid #333; border-radius: 3px;
    padding: 6px 10px; font-family: Arial, sans-serif; font-size: 11px;
    color: #ccc; line-height: 1.6; white-space: nowrap;
  }
  #price-label {
    position: absolute; right: 0; z-index: 20; pointer-events: none; display: none;
    font-family: Arial, sans-serif; font-size: 11px; font-weight: bold;
    padding: 2px 6px; border-radius: 2px 0 0 2px;
  }
  /* Live candle pulse indicator */
  #live-badge {
    position: absolute; top: 8px; right: 80px; z-index: 20;
    font-family: Arial, sans-serif; font-size: 10px; font-weight: bold;
    color: #ff0; padding: 2px 7px; border: 1px solid #ff0;
    border-radius: 3px; letter-spacing: 1px;
    animation: pulse 1.2s ease-in-out infinite;
    display: none;
  }
  @keyframes pulse { 0%%,100%% { opacity:1; } 50%% { opacity:0.3; } }
</style>
</head>
<body>
<div id="header">
  <div class="symbol">GOLDi ▼ M15</div>
  <div class="timeframe">Gold vs US Dollar</div>
</div>
<div id="live-badge">● LIVE</div>
<div id="chart"></div>
<div id="tooltip"></div>
<div id="price-label"></div>

<script>
(function () {
  const candles  = %s;
  const liveIdx  = %d;

  const chart = LightweightCharts.createChart(document.getElementById('chart'), {
    width:  window.innerWidth,
    height: window.innerHeight,
    layout: {
      background: { type: 'solid', color: '#000000' },
      textColor: '#888888',
      fontSize: 11,
      fontFamily: 'Arial, sans-serif',
    },
    grid: {
      vertLines: { color: '#1a1a1a', style: LightweightCharts.LineStyle.Dashed },
      horzLines: { color: '#1a1a1a', style: LightweightCharts.LineStyle.Dashed },
    },
    crosshair: {
      mode: LightweightCharts.CrosshairMode.Normal,
      vertLine: { color: '#555', width: 1, style: LightweightCharts.LineStyle.Solid, labelBackgroundColor: '#222' },
      horzLine: { color: '#555', width: 1, style: LightweightCharts.LineStyle.Dashed, labelBackgroundColor: '#222' },
    },
    rightPriceScale: { borderColor: '#222', textColor: '#888', scaleMargins: { top: 0.05, bottom: 0.05 } },
    timeScale: {
      borderColor: '#222', timeVisible: true, secondsVisible: false,
      tickMarkFormatter: function(time) {
        const d = new Date(time * 1000);
        const day = d.getUTCDate();
        const mon = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'][d.getUTCMonth()];
        const hh  = String(d.getUTCHours()).padStart(2,'0');
        const mm  = String(d.getUTCMinutes()).padStart(2,'0');
        return day + ' ' + mon + ' ' + hh + ':' + mm;
      },
    },
    handleScroll: { mouseWheel: true, pressedMouseMove: true },
    handleScale:  { mouseWheel: true, pinch: true },
  });

  const candleSeries = chart.addCandlestickSeries({
    upColor: '#00bcd4', downColor: '#e53935',
    borderUpColor: '#00bcd4', borderDownColor: '#e53935',
    wickUpColor: '#00bcd4', wickDownColor: '#e53935',
  });

  // Set all candles first
  candleSeries.setData(candles);

  // Override the live (forming) candle with yellow colour
  if (liveIdx >= 0 && liveIdx < candles.length) {
    const lc = candles[liveIdx];
    candleSeries.update({
      time:  lc.time,
      open:  lc.open,
      high:  lc.high,
      low:   lc.low,
      close: lc.close,
      color:        '#ffd700',
      borderColor:  '#ffd700',
      wickColor:    '#ffd700',
    });
    document.getElementById('live-badge').style.display = 'block';
  }

  chart.timeScale().fitContent();

  window.addEventListener('resize', function () {
    chart.applyOptions({ width: window.innerWidth, height: window.innerHeight });
  });

  // ── Crosshair tooltip ─────────────────────────────────────────────
  const tooltip    = document.getElementById('tooltip');
  const priceLabel = document.getElementById('price-label');

  chart.subscribeCrosshairMove(function (param) {
    if (!param.time || !param.seriesData.has(candleSeries)) {
      tooltip.style.display = 'none';
      return;
    }
    const bar    = param.seriesData.get(candleSeries);
    const isLive = liveIdx >= 0 && bar.time === candles[liveIdx].time;
    const isBull = bar.close >= bar.open;
    const clr    = isLive ? '#ffd700' : (isBull ? '#00bcd4' : '#e53935');

    const d      = new Date(param.time * 1000);
    const dateStr = d.getUTCDate() + ' ' +
      ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'][d.getUTCMonth()] +
      ' ' + String(d.getUTCHours()).padStart(2,'0') + ':' + String(d.getUTCMinutes()).padStart(2,'0') +
      (isLive ? ' ⟳ forming' : '');

    const change    = bar.close - bar.open;
    const changePct = (change / bar.open * 100).toFixed(2);

    tooltip.innerHTML =
      '<span style="color:#888">' + dateStr + '</span><br>' +
      'O: <span style="color:' + clr + '">' + bar.open.toFixed(2)  + '</span>  ' +
      'H: <span style="color:' + clr + '">' + bar.high.toFixed(2)  + '</span>  ' +
      'L: <span style="color:' + clr + '">' + bar.low.toFixed(2)   + '</span>  ' +
      'C: <span style="color:' + clr + '">' + bar.close.toFixed(2) + '</span>  ' +
      '<span style="color:' + clr + '">' + (change >= 0 ? '+' : '') + change.toFixed(2) +
      ' (' + changePct + '%%)</span>';

    const x = (param.point.x || 0) + 16;
    const y = Math.max(4, (param.point.y || 0) - 10);
    tooltip.style.left    = x + 'px';
    tooltip.style.top     = y + 'px';
    tooltip.style.display = 'block';
  });

  // ── Last-price label ──────────────────────────────────────────────
  if (candles.length > 0) {
    const last   = candles[candles.length - 1];
    const isLive = liveIdx === candles.length - 1;
    const isBull = last.close >= last.open;
    const bg     = isLive ? '#ffd700' : (isBull ? '#00bcd4' : '#e53935');
    const coord  = candleSeries.priceToCoordinate(last.close);

    priceLabel.textContent       = last.close.toFixed(2);
    priceLabel.style.background  = bg;
    priceLabel.style.color       = '#000';
    if (coord !== null) {
      priceLabel.style.top     = (coord - 10) + 'px';
      priceLabel.style.display = 'block';
    }

    chart.subscribeCrosshairMove(function() {
      const c = candleSeries.priceToCoordinate(last.close);
      if (c !== null) {
        priceLabel.style.top     = (c - 10) + 'px';
        priceLabel.style.display = 'block';
      }
    });
  }
})();
</script>
</body>
</html>`, title, candlesJSON, liveIdx)
}
