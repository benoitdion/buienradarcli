package buienradar

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MergedEntry is one data point in a merged multi-source rain forecast.
type MergedEntry struct {
	Time            string  `json:"time"`
	UTCTime         string  `json:"utc_time"`
	MMPerH          float64 `json:"mm_per_h"`
	IntervalMinutes int     `json:"interval_minutes"`
	Source          string  `json:"source"`
}

type MergedForecast struct {
	Entries []MergedEntry
	// Source describes which feeds contributed (e.g. "3h+8h+48h", "2h-fallback").
	Source        string
	PartialErrors []string // horizons that failed, if any
}

// MergedRainForecast fetches the 3h, 8h, and 48h graphdata forecasts in
// parallel and stitches them into one timeline, always preferring
// higher-resolution data for the near-term window:
//
//	3h  → 5-min buckets, history + ~2h ahead   (most accurate near-term)
//	8h  → 15-min buckets from where 3h ends    (~8h total)
//	48h → hourly buckets from where 8h ends    (~48h total)
//
// If all graphdata endpoints fail (e.g. the network is down) it falls back to
// the legacy 2h feed.
func (c *Client) MergedRainForecast(ctx context.Context, lat, lon float64) (*MergedForecast, error) {
	type fetchResult struct {
		horizon string
		g       *GraphForecast
		err     error
	}

	horizonEndpoints := []struct{ horizon, endpoint string }{
		{"3h", GraphRainHistoryForecast},
		{"8h", GraphRain8Hour},
		{"48h", GraphRain24Hour},
	}

	ch := make(chan fetchResult, len(horizonEndpoints))
	for _, h := range horizonEndpoints {
		h := h
		go func() {
			g, err := c.RainGraph(ctx, h.endpoint, lat, lon)
			ch <- fetchResult{h.horizon, g, err}
		}()
	}

	fetched := make(map[string]*GraphForecast, 3)
	var failed []string
	for range horizonEndpoints {
		r := <-ch
		if r.err != nil {
			failed = append(failed, r.horizon)
		} else {
			fetched[r.horizon] = r.g
		}
	}

	if len(fetched) == 0 {
		// All graphdata sources failed — fall back to the legacy 2h endpoint.
		entries, err := c.PrecipitationForecast(ctx, lat, lon)
		if err != nil {
			return nil, fmt.Errorf("all graphdata endpoints failed and legacy 2h fallback also failed: %w", err)
		}
		merged := make([]MergedEntry, 0, len(entries))
		for _, e := range entries {
			merged = append(merged, MergedEntry{
				Time: e.Time, MMPerH: e.MMPerH, IntervalMinutes: 5, Source: "2h",
			})
		}
		return &MergedForecast{
			Entries:       merged,
			Source:        "2h-fallback",
			PartialErrors: failed,
		}, nil
	}

	entries := mergeGraphForecasts(fetched)
	sources := make([]string, 0, 3)
	for _, h := range []string{"3h", "8h", "48h"} {
		if _, ok := fetched[h]; ok {
			sources = append(sources, h)
		}
	}

	return &MergedForecast{
		Entries:       entries,
		Source:        strings.Join(sources, "+"),
		PartialErrors: failed,
	}, nil
}

// mergeGraphForecasts stitches forecasts together using each source only for
// the time window that follows the previous (higher-resolution) source.
func mergeGraphForecasts(forecasts map[string]*GraphForecast) []MergedEntry {
	parse := func(g *GraphForecast, source string) []parsedEntry {
		if g == nil {
			return nil
		}
		interval := g.IntervalMinutes()
		out := make([]parsedEntry, 0, len(g.Entries))
		for _, e := range g.Entries {
			t, err := time.Parse("2006-01-02T15:04:05", e.UTCDateTime)
			if err != nil {
				continue
			}
			out = append(out, parsedEntry{
				utc:      t,
				local:    e.DateTime,
				utcStr:   e.UTCDateTime,
				mmh:      e.DataValue,
				interval: interval,
				source:   source,
			})
		}
		return out
	}

	var all []parsedEntry
	cutoff := time.Time{} // UTC time of the last appended entry

	for _, h := range []string{"3h", "8h", "48h"} {
		g := forecasts[h]
		if g == nil {
			continue
		}
		for _, e := range parse(g, h) {
			if e.utc.After(cutoff) {
				all = append(all, e)
				cutoff = e.utc
			}
		}
	}

	out := make([]MergedEntry, 0, len(all))
	for _, e := range all {
		out = append(out, MergedEntry{
			Time:            e.local,
			UTCTime:         e.utcStr,
			MMPerH:          e.mmh,
			IntervalMinutes: e.interval,
			Source:          e.source,
		})
	}
	return out
}

type parsedEntry struct {
	utc      time.Time
	local    string
	utcStr   string
	mmh      float64
	interval int
	source   string
}
