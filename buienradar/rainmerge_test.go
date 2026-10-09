package buienradar

import (
	"reflect"
	"testing"
)

func TestMergeGraphForecastsCoverage(t *testing.T) {
	stamp := func(at string) string { return "2026-10-25T" + at + ":00" }
	graph := func(value float64, times ...string) *GraphForecast {
		g := &GraphForecast{}
		for i := 0; i < len(times); i += 2 {
			g.Entries = append(g.Entries, GraphForecastEntry{
				UTCDateTime: stamp(times[i]), DateTime: stamp(times[i+1]), DataValue: value,
			})
		}
		return g
	}
	entry := func(utc, local string, minutes int, value float64, source string) MergedEntry {
		return MergedEntry{Time: stamp(local), UTCTime: stamp(utc), IntervalMinutes: minutes, MMPerH: value, Source: source}
	}
	tests := []struct {
		name      string
		forecasts map[string]*GraphForecast
		want      []MergedEntry
	}{
		{
			name: "five to fifteen minutes",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "10:10", "11:10", "10:15", "11:15"),
				"8h": graph(2, "10:00", "11:00", "10:15", "11:15", "10:30", "11:30"),
			},
			want: []MergedEntry{
				entry("10:10", "11:10", 5, 1, "3h"), entry("10:15", "11:15", 5, 1, "3h"),
				entry("10:20", "11:20", 10, 2, "8h"), entry("10:30", "11:30", 15, 2, "8h"),
			},
		},
		{
			name: "fifteen to sixty minutes",
			forecasts: map[string]*GraphForecast{
				"8h":  graph(2, "10:15", "11:15", "10:30", "11:30"),
				"48h": graph(3, "09:00", "10:00", "10:00", "11:00", "11:00", "12:00"),
			},
			want: []MergedEntry{
				entry("10:15", "11:15", 15, 2, "8h"), entry("10:30", "11:30", 15, 2, "8h"),
				entry("10:45", "11:45", 15, 3, "48h"), entry("11:00", "12:00", 60, 3, "48h"),
			},
		},
		{
			name: "aligned boundary",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "10:05", "11:05", "10:10", "11:10"),
				"8h": graph(2, "10:00", "11:00", "10:15", "11:15"),
			},
			want: []MergedEntry{
				entry("10:05", "11:05", 5, 1, "3h"), entry("10:10", "11:10", 5, 1, "3h"),
				entry("10:15", "11:15", 15, 2, "8h"),
			},
		},
		{
			name: "dry crossing interval",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "10:10", "11:10", "10:15", "11:15"),
				"8h": graph(0, "10:00", "11:00", "10:15", "11:15", "10:30", "11:30"),
			},
			want: []MergedEntry{
				entry("10:10", "11:10", 5, 1, "3h"), entry("10:15", "11:15", 5, 1, "3h"),
				entry("10:20", "11:20", 10, 0, "8h"), entry("10:30", "11:30", 15, 0, "8h"),
			},
		},
		{
			name: "absent crossing interval remains a gap",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "10:10", "11:10", "10:15", "11:15"),
				"8h": graph(2, "09:45", "10:45", "10:00", "11:00", "10:30", "11:30"),
			},
			want: []MergedEntry{
				entry("10:10", "11:10", 5, 1, "3h"), entry("10:15", "11:15", 5, 1, "3h"),
				entry("10:30", "11:30", 15, 2, "8h"),
			},
		},
		{
			name: "daylight saving offset follows crossing entry",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "01:00", "02:00", "01:05", "02:05"),
				"8h": graph(2, "00:45", "02:45", "01:00", "02:00", "01:15", "02:15"),
			},
			want: []MergedEntry{
				entry("01:00", "02:00", 5, 1, "3h"), entry("01:05", "02:05", 5, 1, "3h"),
				entry("01:10", "02:10", 5, 2, "8h"), entry("01:15", "02:15", 15, 2, "8h"),
			},
		},
		{
			name: "invalid local time retains utc coverage",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "10:10", "11:10", "10:15", "11:15"),
				"8h": graph(2, "10:00", "11:00", "10:15", "invalid", "10:30", "11:30"),
			},
			want: []MergedEntry{
				entry("10:10", "11:10", 5, 1, "3h"), entry("10:15", "11:15", 5, 1, "3h"),
				entry("10:20", "invalid", 10, 2, "8h"), entry("10:30", "11:30", 15, 2, "8h"),
			},
		},
		{
			name: "singleton has no inferred coverage",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "10:15", "11:15"),
				"8h": graph(2, "10:30", "11:30"),
			},
			want: []MergedEntry{
				entry("10:15", "11:15", 0, 1, "3h"), entry("10:30", "11:30", 0, 2, "8h"),
			},
		},
		{
			name: "invalid utc is skipped",
			forecasts: map[string]*GraphForecast{
				"3h": graph(1, "invalid", "11:10", "10:15", "11:15"),
			},
			want: []MergedEntry{entry("10:15", "11:15", 0, 1, "3h")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeGraphForecasts(tt.forecasts)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("merged intervals = %+v; want %+v", got, tt.want)
			}
		})
	}
}
