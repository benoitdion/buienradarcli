package buienradar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	GraphRainHistoryForecast = "https://graphdata.buienradar.nl/3.0/forecast/geo/RainHistoryForecast"
	GraphRain8Hour           = "https://graphdata.buienradar.nl/3.0/forecast/geo/Rain8Hour"
	GraphRain24Hour          = "https://graphdata.buienradar.nl/3.0/forecast/geo/Rain24Hour"
)

type GraphForecast struct {
	Borders []GraphBorder        `json:"borders"`
	Unit    string               `json:"unit"`
	Entries []GraphForecastEntry `json:"forecasts"`
}

type GraphBorder struct {
	Title           string  `json:"title"`
	PercentageValue float64 `json:"percentageValue"`
}

type GraphForecastEntry struct {
	DateTime    string  `json:"dateTime"`
	UTCDateTime string  `json:"utcDateTime"`
	DataValue   float64 `json:"dataValue"`
	Percentage  float64 `json:"percentageValue"`
}

// RainGraph fetches one of the graphdata.buienradar.nl forecast endpoints.
//
// Key lifecycle:
//   - If no key is cached (cold start), it scrapes one proactively so the key
//     is stored for future runs even when the API doesn't yet enforce it.
//   - On auth failure (401/403) it re-scrapes and retries once.
func (c *Client) RainGraph(ctx context.Context, endpoint string, lat, lon float64) (*GraphForecast, error) {
	key := c.Keys.Key()
	if key == "" {
		// No key on disk yet — scrape proactively so it's cached going forward.
		// If scraping fails we proceed with an empty key (API may not enforce it).
		if fresh, err := c.Keys.Refresh(); err == nil {
			key = fresh
		}
	}

	result, err := c.rainGraphOnce(ctx, endpoint, lat, lon, key)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, ErrUnauthorized) {
		return nil, err
	}

	// Key was rejected — scrape a fresh one and retry.
	freshKey, refreshErr := c.Keys.Refresh()
	if refreshErr != nil {
		return nil, fmt.Errorf("api key refresh: %w (original error: %w)", refreshErr, err)
	}
	return c.rainGraphOnce(ctx, endpoint, lat, lon, freshKey)
}

func (c *Client) rainGraphOnce(ctx context.Context, endpoint string, lat, lon float64, key string) (*GraphForecast, error) {
	q := map[string]string{
		"lat": fmt.Sprintf("%.4f", lat),
		"lon": fmt.Sprintf("%.4f", lon),
		"ak":  key,
	}
	body, err := c.get(ctx, endpoint, q)
	if err != nil {
		return nil, err
	}
	var g GraphForecast
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, fmt.Errorf("decode rain graph: %w", err)
	}
	return &g, nil
}

// IntervalMinutes returns the spacing between consecutive entries in minutes,
// or 0 if it cannot be determined.
func (g *GraphForecast) IntervalMinutes() int {
	if len(g.Entries) < 2 {
		return 0
	}
	t1, err1 := time.Parse("2006-01-02T15:04:05", g.Entries[0].UTCDateTime)
	t2, err2 := time.Parse("2006-01-02T15:04:05", g.Entries[1].UTCDateTime)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(t2.Sub(t1).Minutes())
}
