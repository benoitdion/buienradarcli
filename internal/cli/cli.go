// Package cli wires Buienradar API calls to a Cobra command tree with
// agent-friendly output modes and discovery.
package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/benoitdion/buienradarcli/internal/buienradar"
	"github.com/spf13/cobra"
)

// Default coordinates: Amsterdam.
const (
	DefaultLat = 52.3676
	DefaultLon = 4.9041
)

type Runtime struct {
	Out         io.Writer
	Err         io.Writer
	Client      *buienradar.Client
	StdoutIsTTY bool
}

func NewRuntime() *Runtime {
	return &Runtime{
		Out:         os.Stdout,
		Err:         os.Stderr,
		Client:      buienradar.NewClient(),
		StdoutIsTTY: isTerminal(os.Stdout),
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Build constructs the cobra command tree. The Runtime owns I/O and the API
// client, so tests can swap them out.
func Build(rt *Runtime) *cobra.Command {
	var outputRaw string
	var apiKey string
	var listTop bool

	root := &cobra.Command{
		Use:           "buienradarcli",
		Short:         "Agent-friendly CLI for the Buienradar (Dutch weather) APIs",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if apiKey != "" {
				rt.Client.Keys.Override(apiKey)
			} else if env := os.Getenv("BUIENRADAR_API_KEY"); env != "" {
				rt.Client.Keys.Override(env)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			if listTop {
				return printList(rt.Out, "list", TopLevelSpecs(), mode)
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVarP(&outputRaw, "output", "o", "", "Output mode: json|plain|text")
	root.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for graphdata endpoints (default: built-in public key; env BUIENRADAR_API_KEY)")
	root.Flags().BoolVar(&listTop, "list", false, "List top-level capabilities")

	root.AddCommand(newDescribeCmd(rt))
	root.AddCommand(newAgentSkillCmd(rt))
	root.AddCommand(newCurrentCmd(rt))
	root.AddCommand(newForecastCmd(rt))
	root.AddCommand(newRainCmd(rt))
	root.AddCommand(newStationsCmd(rt))
	root.AddCommand(newReportCmd(rt))

	return root
}

func resolvedMode(cmd *cobra.Command, rt *Runtime) (OutputMode, error) {
	raw, _ := cmd.Flags().GetString("output")
	if raw == "" && cmd.Parent() != nil {
		raw, _ = cmd.Root().PersistentFlags().GetString("output")
	}
	requested, err := ParseOutputMode(raw)
	if err != nil {
		return "", err
	}
	return Resolve(requested, rt.StdoutIsTTY), nil
}

func printList(w io.Writer, command string, specs []CommandSpec, mode OutputMode) error {
	switch mode {
	case OutputJSON:
		return WriteJSON(w, command, specs)
	case OutputPlain:
		rows := make([]map[string]string, 0, len(specs))
		for _, s := range specs {
			rows = append(rows, map[string]string{
				"command": strings.Join(s.Path, " "),
				"summary": s.Summary,
			})
		}
		WritePlain(w, rows, []string{"command", "summary"})
		return nil
	default:
		for _, s := range specs {
			fmt.Fprintf(w, "%-12s  %s\n", strings.Join(s.Path, " "), s.Summary)
		}
		return nil
	}
}

func newDescribeCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <path...>",
		Short: "Describe a command path",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			spec := FindSpec(args)
			if spec == nil {
				return fmt.Errorf("unknown command path: %s", strings.Join(args, " "))
			}
			switch mode {
			case OutputJSON:
				return WriteJSON(rt.Out, "describe", spec)
			case OutputPlain:
				rows := []map[string]string{{
					"path":    strings.Join(spec.Path, " "),
					"summary": spec.Summary,
					"auth":    spec.Auth,
					"safety":  spec.Safety,
				}}
				WritePlain(rt.Out, rows, []string{"path", "summary", "auth", "safety"})
				return nil
			default:
				fmt.Fprintf(rt.Out, "%s | %s\n", strings.Join(spec.Path, " "), spec.Summary)
				fmt.Fprintf(rt.Out, "auth:   %s\n", spec.Auth)
				fmt.Fprintf(rt.Out, "safety: %s\n", spec.Safety)
				fmt.Fprintf(rt.Out, "output: %s\n", spec.Output)
				if spec.Description != "" {
					fmt.Fprintf(rt.Out, "\n%s\n", spec.Description)
				}
				if len(spec.Options) > 0 {
					fmt.Fprintln(rt.Out, "\noptions:")
					for _, o := range spec.Options {
						def := ""
						if o.Default != "" {
							def = fmt.Sprintf(" (default %s)", o.Default)
						}
						fmt.Fprintf(rt.Out, "  --%s %s%s — %s\n", o.Name, o.Type, def, o.Description)
					}
				}
				return nil
			}
		},
	}
}

func newAgentSkillCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "agent-skill",
		Short: "Print the buienradarcli agent skill",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			if mode == OutputJSON {
				return WriteJSON(rt.Out, "agent-skill", map[string]string{
					"name":    "buienradarcli",
					"content": AgentSkillContent,
				})
			}
			fmt.Fprint(rt.Out, AgentSkillContent)
			return nil
		},
	}
}

type currentResult struct {
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	Station            string  `json:"station_name"`
	StationID          int     `json:"station_id"`
	Region             string  `json:"region"`
	StationLat         float64 `json:"station_lat"`
	StationLon         float64 `json:"station_lon"`
	DistanceKM         float64 `json:"distance_km"`
	Timestamp          string  `json:"timestamp"`
	TemperatureC       float64 `json:"temperature_c"`
	FeelsLikeC         float64 `json:"feels_like_c"`
	GroundTempC        float64 `json:"ground_temperature_c"`
	HumidityPct        float64 `json:"humidity_pct"`
	WindSpeedMS        float64 `json:"wind_speed_ms"`
	WindGustsMS        float64 `json:"wind_gusts_ms"`
	WindBft            float64 `json:"wind_bft"`
	WindDirection      string  `json:"wind_direction"`
	WindDirectionDeg   float64 `json:"wind_direction_deg"`
	AirPressureHpa     float64 `json:"air_pressure_hpa"`
	VisibilityM        float64 `json:"visibility_m"`
	PrecipitationMmH   float64 `json:"precipitation_mm_h"`
	RainLastHourMm     float64 `json:"rain_last_hour_mm"`
	RainLast24HourMm   float64 `json:"rain_last_24h_mm"`
	SunPowerWm2        float64 `json:"sun_power_w_m2"`
	Condition          string  `json:"condition"`
	IconCode           string  `json:"icon_code"`
	WeatherDescription string  `json:"weather_description"`
}

func newCurrentCmd(rt *Runtime) *cobra.Command {
	var lat, lon float64
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Current weather from the station nearest to (lat, lon)",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			feed, err := rt.Client.Feed(cmd.Context())
			if err != nil {
				return err
			}
			st := buienradar.NearestStation(feed.Actual.StationMeasurements, lat, lon)
			if st == nil {
				return fmt.Errorf("no stations in feed")
			}
			res := currentResult{
				Lat: lat, Lon: lon,
				Station: st.StationName, StationID: st.StationID, Region: st.Regio,
				StationLat: st.Lat, StationLon: st.Lon,
				DistanceKM:         haversineKM(lat, lon, st.Lat, st.Lon),
				Timestamp:          st.Timestamp,
				TemperatureC:       st.Temperature,
				FeelsLikeC:         st.FeelTemperature,
				GroundTempC:        st.GroundTemperature,
				HumidityPct:        st.Humidity,
				WindSpeedMS:        st.WindSpeed,
				WindGustsMS:        st.WindGusts,
				WindBft:            st.WindSpeedBft,
				WindDirection:      st.WindDirection,
				WindDirectionDeg:   st.WindDirectionDeg,
				AirPressureHpa:     st.AirPressure,
				VisibilityM:        st.Visibility,
				PrecipitationMmH:   st.Precipitation,
				RainLastHourMm:     st.RainFallLastHour,
				RainLast24HourMm:   st.RainFallLast24Hour,
				SunPowerWm2:        st.SunPower,
				Condition:          buienradar.Condition(buienradar.IconCode(st.IconURL)),
				IconCode:           buienradar.IconCode(st.IconURL),
				WeatherDescription: st.WeatherDescription,
			}

			switch mode {
			case OutputJSON:
				return WriteJSON(rt.Out, "current", res)
			case OutputPlain:
				row := map[string]string{
					"station":           res.Station,
					"region":            res.Region,
					"distance_km":       fmt.Sprintf("%.1f", res.DistanceKM),
					"temperature_c":     fmt.Sprintf("%.1f", res.TemperatureC),
					"feels_like_c":      fmt.Sprintf("%.1f", res.FeelsLikeC),
					"humidity_pct":      fmt.Sprintf("%.0f", res.HumidityPct),
					"wind_ms":           fmt.Sprintf("%.1f", res.WindSpeedMS),
					"wind_dir":          res.WindDirection,
					"precip_mm_h":       fmt.Sprintf("%.2f", res.PrecipitationMmH),
					"condition":         res.Condition,
					"description":       res.WeatherDescription,
				}
				WritePlain(rt.Out, []map[string]string{row}, []string{
					"station", "region", "distance_km", "temperature_c", "feels_like_c",
					"humidity_pct", "wind_ms", "wind_dir", "precip_mm_h", "condition", "description",
				})
				return nil
			default:
				fmt.Fprintf(rt.Out, "%s (%s) — %.1f km from (%.4f, %.4f)\n",
					res.Station, res.Region, res.DistanceKM, lat, lon)
				fmt.Fprintf(rt.Out, "  %s, %s\n", res.WeatherDescription, res.Condition)
				fmt.Fprintf(rt.Out, "  temp:    %.1f°C (feels %.1f°C)\n", res.TemperatureC, res.FeelsLikeC)
				fmt.Fprintf(rt.Out, "  wind:    %.1f m/s %s (%.0f°)\n", res.WindSpeedMS, res.WindDirection, res.WindDirectionDeg)
				fmt.Fprintf(rt.Out, "  humid:   %.0f%%\n", res.HumidityPct)
				fmt.Fprintf(rt.Out, "  precip:  %.2f mm/h (%.1f mm last hour, %.1f mm last 24h)\n",
					res.PrecipitationMmH, res.RainLastHourMm, res.RainLast24HourMm)
				fmt.Fprintf(rt.Out, "  pressure:%.1f hPa\n", res.AirPressureHpa)
				fmt.Fprintf(rt.Out, "  measured:%s\n", res.Timestamp)
				return nil
			}
		},
	}
	cmd.Flags().Float64Var(&lat, "lat", DefaultLat, "Latitude")
	cmd.Flags().Float64Var(&lon, "lon", DefaultLon, "Longitude")
	return cmd
}

type forecastDay struct {
	Date               string  `json:"date"`
	MinTempC           float64 `json:"min_temp_c"`
	MaxTempC           float64 `json:"max_temp_c"`
	RainChancePct      int     `json:"rain_chance_pct"`
	SunChancePct       int     `json:"sun_chance_pct"`
	RainMinMm          float64 `json:"rain_min_mm"`
	RainMaxMm          float64 `json:"rain_max_mm"`
	WindBft            int     `json:"wind_bft"`
	WindDirection      string  `json:"wind_direction"`
	Condition          string  `json:"condition"`
	IconCode           string  `json:"icon_code"`
	WeatherDescription string  `json:"weather_description"`
}

func newForecastCmd(rt *Runtime) *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "forecast",
		Short: "Five-day national forecast",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			feed, err := rt.Client.Feed(cmd.Context())
			if err != nil {
				return err
			}
			n := days
			if n <= 0 {
				n = 5
			}
			if n > len(feed.Forecast.FiveDay) {
				n = len(feed.Forecast.FiveDay)
			}
			out := make([]forecastDay, 0, n)
			for i := 0; i < n; i++ {
				d := feed.Forecast.FiveDay[i]
				out = append(out, forecastDay{
					Date:               strings.SplitN(d.Day, "T", 2)[0],
					MinTempC:           parseFloatOr(d.MinTemperatureRaw, d.MinTemperature),
					MaxTempC:           parseFloatOr(d.MaxTemperatureRaw, d.MaxTemperature),
					RainChancePct:      d.RainChance,
					SunChancePct:       d.SunChance,
					RainMinMm:          d.MMRainMin,
					RainMaxMm:          d.MMRainMax,
					WindBft:            d.Wind,
					WindDirection:      d.WindDirection,
					Condition:          buienradar.Condition(buienradar.IconCode(d.IconURL)),
					IconCode:           buienradar.IconCode(d.IconURL),
					WeatherDescription: d.WeatherDescription,
				})
			}

			switch mode {
			case OutputJSON:
				return WriteJSON(rt.Out, "forecast", out)
			case OutputPlain:
				rows := make([]map[string]string, 0, len(out))
				for _, d := range out {
					rows = append(rows, map[string]string{
						"date":          d.Date,
						"min_c":         fmt.Sprintf("%.0f", d.MinTempC),
						"max_c":         fmt.Sprintf("%.0f", d.MaxTempC),
						"rain_chance":   fmt.Sprintf("%d", d.RainChancePct),
						"sun_chance":    fmt.Sprintf("%d", d.SunChancePct),
						"rain_min_mm":   fmt.Sprintf("%.1f", d.RainMinMm),
						"rain_max_mm":   fmt.Sprintf("%.1f", d.RainMaxMm),
						"wind_bft":      fmt.Sprintf("%d", d.WindBft),
						"wind_dir":      d.WindDirection,
						"condition":     d.Condition,
						"description":   d.WeatherDescription,
					})
				}
				WritePlain(rt.Out, rows, []string{"date", "min_c", "max_c", "rain_chance", "sun_chance",
					"rain_min_mm", "rain_max_mm", "wind_bft", "wind_dir", "condition", "description"})
				return nil
			default:
				for _, d := range out {
					fmt.Fprintf(rt.Out, "%s  %2.0f–%2.0f°C  rain %3d%% (%.1f–%.1f mm)  sun %3d%%  wind %d Bft %s  %s\n",
						d.Date, d.MinTempC, d.MaxTempC, d.RainChancePct, d.RainMinMm, d.RainMaxMm,
						d.SunChancePct, d.WindBft, d.WindDirection, d.Condition)
				}
				return nil
			}
		},
	}
	cmd.Flags().IntVar(&days, "days", 5, "Days to return (1-5)")
	return cmd
}

type rainEntry struct {
	Time   string  `json:"time"`
	MMPerH float64 `json:"mm_per_h"`
}

type rainResult struct {
	Lat           float64     `json:"lat"`
	Lon           float64     `json:"lon"`
	TotalMm       float64     `json:"total_mm"`
	WillRain      bool        `json:"will_rain"`
	FirstRainTime string      `json:"first_rain_time,omitempty"`
	PartialErrors []string    `json:"partial_errors,omitempty"`
	Entries       []rainEntry `json:"entries"`
}

func newRainCmd(rt *Runtime) *cobra.Command {
	var lat, lon float64
	cmd := &cobra.Command{
		Use:   "rain",
		Short: "Precipitation forecast",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			return runRainMerged(cmd.Context(), rt, lat, lon, mode)
		},
	}
	cmd.Flags().Float64Var(&lat, "lat", DefaultLat, "Latitude")
	cmd.Flags().Float64Var(&lon, "lon", DefaultLon, "Longitude")
	return cmd
}

func runRainMerged(ctx context.Context, rt *Runtime, lat, lon float64, mode OutputMode) error {
	mf, err := rt.Client.MergedRainForecast(ctx, lat, lon)
	if err != nil {
		return err
	}

	res := rainResult{Lat: lat, Lon: lon, PartialErrors: mf.PartialErrors}
	for _, e := range mf.Entries {
		res.Entries = append(res.Entries, rainEntry{Time: e.Time, MMPerH: e.MMPerH})
		res.TotalMm += e.MMPerH * float64(e.IntervalMinutes) / 60.0
		if e.MMPerH > 0.1 && !res.WillRain {
			res.WillRain = true
			res.FirstRainTime = e.Time
		}
	}
	return renderRain(rt, res, mode)
}

func renderRain(rt *Runtime, res rainResult, mode OutputMode) error {
	switch mode {
	case OutputJSON:
		return WriteJSON(rt.Out, "rain", res)
	case OutputPlain:
		rows := make([]map[string]string, 0, len(res.Entries))
		for _, e := range res.Entries {
			rows = append(rows, map[string]string{
				"time": e.Time,
				"mm_h": fmt.Sprintf("%.3f", e.MMPerH),
			})
		}
		WritePlain(rt.Out, rows, []string{"time", "mm_h"})
		return nil
	default:
		fmt.Fprintf(rt.Out, "rain for (%.4f, %.4f)\n", res.Lat, res.Lon)
		if len(res.PartialErrors) > 0 {
			fmt.Fprintf(rt.Out, "warning: some sources failed: %s\n", strings.Join(res.PartialErrors, ", "))
		}
		if res.WillRain {
			fmt.Fprintf(rt.Out, "rain >0.1 mm/h starting %s, total ~%.2f mm\n", res.FirstRainTime, res.TotalMm)
		} else {
			fmt.Fprintln(rt.Out, "no significant rain expected")
		}
		for _, e := range res.Entries {
			fmt.Fprintf(rt.Out, "  %-19s  %5.2f mm/h  %s\n", e.Time, e.MMPerH, rainBar(e.MMPerH))
		}
		return nil
	}
}

func rainBar(mmh float64) string {
	if mmh <= 0 {
		return ""
	}
	scale := math.Log10(mmh*10+1) * 6
	if scale < 1 {
		scale = 1
	}
	if scale > 30 {
		scale = 30
	}
	return strings.Repeat("█", int(scale))
}

type stationOut struct {
	StationID          int     `json:"station_id"`
	StationName        string  `json:"station_name"`
	Region             string  `json:"region"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	Timestamp          string  `json:"timestamp"`
	TemperatureC       float64 `json:"temperature_c"`
	HumidityPct        float64 `json:"humidity_pct"`
	WindSpeedMS        float64 `json:"wind_speed_ms"`
	WindDirection      string  `json:"wind_direction"`
	Condition          string  `json:"condition"`
	WeatherDescription string  `json:"weather_description"`
}

func newStationsCmd(rt *Runtime) *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:   "stations",
		Short: "List KNMI weather stations with current measurements",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			feed, err := rt.Client.Feed(cmd.Context())
			if err != nil {
				return err
			}
			needle := strings.ToLower(strings.TrimSpace(filter))
			out := make([]stationOut, 0, len(feed.Actual.StationMeasurements))
			for _, s := range feed.Actual.StationMeasurements {
				if needle != "" && !strings.Contains(strings.ToLower(s.StationName), needle) {
					continue
				}
				out = append(out, stationOut{
					StationID: s.StationID, StationName: s.StationName, Region: s.Regio,
					Lat: s.Lat, Lon: s.Lon, Timestamp: s.Timestamp,
					TemperatureC: s.Temperature, HumidityPct: s.Humidity,
					WindSpeedMS: s.WindSpeed, WindDirection: s.WindDirection,
					Condition:          buienradar.Condition(buienradar.IconCode(s.IconURL)),
					WeatherDescription: s.WeatherDescription,
				})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].StationName < out[j].StationName })

			switch mode {
			case OutputJSON:
				return WriteJSON(rt.Out, "stations", out)
			case OutputPlain:
				rows := make([]map[string]string, 0, len(out))
				for _, s := range out {
					rows = append(rows, map[string]string{
						"id":          fmt.Sprintf("%d", s.StationID),
						"name":        s.StationName,
						"region":      s.Region,
						"lat":         fmt.Sprintf("%.4f", s.Lat),
						"lon":         fmt.Sprintf("%.4f", s.Lon),
						"temp_c":      fmt.Sprintf("%.1f", s.TemperatureC),
						"wind_ms":     fmt.Sprintf("%.1f", s.WindSpeedMS),
						"condition":   s.Condition,
					})
				}
				WritePlain(rt.Out, rows, []string{"id", "name", "region", "lat", "lon", "temp_c", "wind_ms", "condition"})
				return nil
			default:
				for _, s := range out {
					fmt.Fprintf(rt.Out, "%-30s %-20s %6.2f,%6.2f  %5.1f°C  %s\n",
						s.StationName, s.Region, s.Lat, s.Lon, s.TemperatureC, s.Condition)
				}
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "Substring filter on station name")
	return cmd
}

type reportResult struct {
	Published string `json:"published"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Author    string `json:"author"`
	ShortTerm string `json:"short_term"`
	LongTerm  string `json:"long_term"`
}

func newReportCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Free-form Dutch weather report from KNMI meteorologists",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			feed, err := rt.Client.Feed(cmd.Context())
			if err != nil {
				return err
			}
			res := reportResult{
				Published: feed.Forecast.WeatherReport.Published,
				Title:     feed.Forecast.WeatherReport.Title,
				Summary:   feed.Forecast.WeatherReport.Summary,
				Author:    feed.Forecast.WeatherReport.Author,
				ShortTerm: feed.Forecast.ShortTerm.Forecast,
				LongTerm:  feed.Forecast.LongTerm.Forecast,
			}
			switch mode {
			case OutputJSON:
				return WriteJSON(rt.Out, "report", res)
			case OutputPlain:
				WritePlain(rt.Out, []map[string]string{{
					"title":      res.Title,
					"published":  res.Published,
					"author":     res.Author,
					"summary":    res.Summary,
					"short_term": res.ShortTerm,
					"long_term":  res.LongTerm,
				}}, []string{"title", "published", "author", "summary", "short_term", "long_term"})
				return nil
			default:
				fmt.Fprintf(rt.Out, "%s\n", res.Title)
				if res.Author != "" {
					fmt.Fprintf(rt.Out, "by %s — %s\n\n", res.Author, res.Published)
				}
				if res.Summary != "" {
					fmt.Fprintf(rt.Out, "%s\n\n", res.Summary)
				}
				if res.ShortTerm != "" {
					fmt.Fprintf(rt.Out, "Short term:\n%s\n\n", res.ShortTerm)
				}
				if res.LongTerm != "" {
					fmt.Fprintf(rt.Out, "Long term:\n%s\n", res.LongTerm)
				}
				return nil
			}
		},
	}
}

func parseFloatOr(s string, fallback float64) float64 {
	v, err := strconvParseFloat(s)
	if err != nil {
		return fallback
	}
	return v
}

func strconvParseFloat(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

// haversineKM computes the great-circle distance between two points in km.
func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return r * c
}

// Execute runs the CLI with a context and writes any error envelope.
func Execute(ctx context.Context) int {
	rt := NewRuntime()
	root := Build(rt)
	root.SetOut(rt.Out)
	root.SetErr(rt.Err)
	root.SetContext(ctx)

	if err := root.ExecuteContext(ctx); err != nil {
		mode := OutputText
		if !rt.StdoutIsTTY {
			mode = OutputJSON
		}
		// If --output was passed, honor it for errors too.
		if raw, _ := root.PersistentFlags().GetString("output"); raw != "" {
			if m, perr := ParseOutputMode(raw); perr == nil && m != "" {
				mode = m
			}
		}
		switch mode {
		case OutputJSON:
			WriteJSONError(rt.Err, "error", err)
		default:
			fmt.Fprintf(rt.Err, "error: %v\n", err)
		}
		return 1
	}
	return 0
}
