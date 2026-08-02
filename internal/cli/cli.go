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
	root.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for graphdata endpoints (default: auto-discovered; env BUIENRADAR_API_KEY)")
	root.Flags().BoolVar(&listTop, "list", false, "List top-level capabilities")

	root.AddCommand(newDescribeCmd(rt))
	root.AddCommand(newAgentSkillCmd(rt))
	root.AddCommand(newForecastCmd(rt))
	root.AddCommand(newRainCmd(rt))
	root.AddCommand(newStationsCmd(rt))

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

func newForecastCmd(rt *Runtime) *cobra.Command {
	var lat, lon float64
	cmd := &cobra.Command{
		Use:   "forecast",
		Short: "Merged weather forecast: live conditions, hourly outlook, 14-day ahead, pollen",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := resolvedMode(cmd, rt)
			if err != nil {
				return err
			}
			if err := validateCoordinates(lat, lon); err != nil {
				return err
			}
			wf, err := rt.Client.MergedWeatherForecast(cmd.Context(), lat, lon)
			if err != nil {
				return err
			}
			switch mode {
			case OutputJSON:
				return WriteJSON(rt.Out, "forecast", wf)
			case OutputPlain:
				rows := make([]map[string]string, 0, len(wf.Entries))
				for _, e := range wf.Entries {
					row := forecastEntryToPlain(e)
					rows = append(rows, row)
				}
				plainKeys := []string{
					"time", "station_name", "temp_c", "min_temp_c", "max_temp_c",
					"feels_like_c", "wind_speed_ms", "wind_bft", "wind_direction",
					"humidity_pct", "pressure_hpa", "precip_mm_h", "precip_mm",
					"pollen_grass_pct", "pollen_tree_pct", "pollen_birch_pct", "pollen_weed_pct",
					"condition",
				}
				WritePlain(rt.Out, rows, plainKeys)
				return nil
			default:
				return renderForecastText(rt.Out, wf)
			}
		},
	}
	cmd.Flags().Float64Var(&lat, "lat", DefaultLat, "Latitude")
	cmd.Flags().Float64Var(&lon, "lon", DefaultLon, "Longitude")
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
			if err := validateCoordinates(lat, lon); err != nil {
				return err
			}
			return runRainMerged(cmd.Context(), rt, lat, lon, mode)
		},
	}
	cmd.Flags().Float64Var(&lat, "lat", DefaultLat, "Latitude")
	cmd.Flags().Float64Var(&lon, "lon", DefaultLon, "Longitude")
	return cmd
}

func validateCoordinates(lat, lon float64) error {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return fmt.Errorf("latitude must be a finite number between -90 and 90")
	}
	if math.IsNaN(lon) || math.IsInf(lon, 0) || lon < -180 || lon > 180 {
		return fmt.Errorf("longitude must be a finite number between -180 and 180")
	}
	return nil
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
	StationID     int     `json:"station_id"`
	StationName   string  `json:"station_name"`
	Region        string  `json:"region"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	Timestamp     string  `json:"timestamp"`
	TemperatureC  float64 `json:"temperature_c"`
	HumidityPct   float64 `json:"humidity_pct"`
	WindSpeedMS   float64 `json:"wind_speed_ms"`
	WindDirection string  `json:"wind_direction"`
	Condition     string  `json:"condition"`
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
			stations, err := rt.Client.AllStations(cmd.Context())
			if err != nil {
				return err
			}
			needle := strings.ToLower(strings.TrimSpace(filter))
			out := make([]stationOut, 0, len(stations))
			for _, s := range stations {
				if needle != "" && !strings.Contains(strings.ToLower(s.StationName), needle) {
					continue
				}
				out = append(out, stationOut{
					StationID:     s.StationID,
					StationName:   s.StationName,
					Region:        s.Regio,
					Lat:           s.Lat,
					Lon:           s.Lon,
					Timestamp:     s.Timestamp,
					TemperatureC:  s.Temperature,
					HumidityPct:   s.Humidity,
					WindSpeedMS:   s.WindSpeed,
					WindDirection: s.WindDirection,
					Condition:     buienradar.Condition(s.IconCode),
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
						"id":        fmt.Sprintf("%d", s.StationID),
						"name":      s.StationName,
						"region":    s.Region,
						"lat":       fmt.Sprintf("%.4f", s.Lat),
						"lon":       fmt.Sprintf("%.4f", s.Lon),
						"temp_c":    fmt.Sprintf("%.1f", s.TemperatureC),
						"wind_ms":   fmt.Sprintf("%.1f", s.WindSpeedMS),
						"condition": s.Condition,
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

func forecastEntryToPlain(e buienradar.ForecastEntry) map[string]string {
	row := map[string]string{"time": e.Time}
	if e.StationName != nil {
		row["station_name"] = *e.StationName
	}
	if e.TempC != nil {
		row["temp_c"] = fmt.Sprintf("%.1f", *e.TempC)
	}
	if e.MinTempC != nil {
		row["min_temp_c"] = fmt.Sprintf("%.1f", *e.MinTempC)
	}
	if e.MaxTempC != nil {
		row["max_temp_c"] = fmt.Sprintf("%.1f", *e.MaxTempC)
	}
	if e.FeelsLikeC != nil {
		row["feels_like_c"] = fmt.Sprintf("%.1f", *e.FeelsLikeC)
	}
	if e.WindSpeedMS != nil {
		row["wind_speed_ms"] = fmt.Sprintf("%.1f", *e.WindSpeedMS)
	}
	if e.WindBft != nil {
		row["wind_bft"] = fmt.Sprintf("%d", *e.WindBft)
	}
	if e.WindDirection != nil {
		row["wind_direction"] = *e.WindDirection
	}
	if e.HumidityPct != nil {
		row["humidity_pct"] = fmt.Sprintf("%.0f", *e.HumidityPct)
	}
	if e.PressureHpa != nil {
		row["pressure_hpa"] = fmt.Sprintf("%.1f", *e.PressureHpa)
	}
	if e.PrecipMmH != nil {
		row["precip_mm_h"] = fmt.Sprintf("%.3f", *e.PrecipMmH)
	}
	if e.PrecipMm != nil {
		row["precip_mm"] = fmt.Sprintf("%.1f", *e.PrecipMm)
	}
	if e.PollenGrassPct != nil {
		row["pollen_grass_pct"] = fmt.Sprintf("%.0f", *e.PollenGrassPct)
	}
	if e.PollenTreePct != nil {
		row["pollen_tree_pct"] = fmt.Sprintf("%.0f", *e.PollenTreePct)
	}
	if e.PollenBirchPct != nil {
		row["pollen_birch_pct"] = fmt.Sprintf("%.0f", *e.PollenBirchPct)
	}
	if e.PollenWeedPct != nil {
		row["pollen_weed_pct"] = fmt.Sprintf("%.0f", *e.PollenWeedPct)
	}
	if e.Condition != nil {
		row["condition"] = *e.Condition
	}
	return row
}

func renderForecastText(w io.Writer, wf *buienradar.WeatherForecast) error {
	if len(wf.PartialErrors) > 0 {
		fmt.Fprintf(w, "warning: some sources failed: %s\n\n", strings.Join(wf.PartialErrors, ", "))
	}
	for _, e := range wf.Entries {
		if e.StationName != nil {
			// Live observation entry — richer single-line summary.
			dist := ""
			if e.DistanceKM != nil {
				dist = fmt.Sprintf(" (%.1f km)", *e.DistanceKM)
			}
			cond := ""
			if e.Condition != nil {
				cond = "  " + *e.Condition
			}
			fmt.Fprintf(w, "%s  %s%s%s\n", e.Time, *e.StationName, dist, cond)
			if e.TempC != nil {
				feels := ""
				if e.FeelsLikeC != nil {
					feels = fmt.Sprintf(" (feels %.1f°C)", *e.FeelsLikeC)
				}
				fmt.Fprintf(w, "  temp:    %.1f°C%s\n", *e.TempC, feels)
			}
			if e.WindSpeedMS != nil {
				dir := ""
				if e.WindDirection != nil {
					dir = " " + *e.WindDirection
				}
				gusts := ""
				if e.WindGustsMS != nil {
					gusts = fmt.Sprintf(", gusts %.1f m/s", *e.WindGustsMS)
				}
				fmt.Fprintf(w, "  wind:    %.1f m/s%s%s\n", *e.WindSpeedMS, dir, gusts)
			}
			if e.HumidityPct != nil {
				fmt.Fprintf(w, "  humid:   %.0f%%\n", *e.HumidityPct)
			}
			if e.PressureHpa != nil {
				fmt.Fprintf(w, "  pressure:%.1f hPa\n", *e.PressureHpa)
			}
			if e.VisibilityM != nil {
				fmt.Fprintf(w, "  visibility:%.0f m\n", *e.VisibilityM)
			}
			if e.PrecipMmH != nil {
				extra := ""
				if e.RainLastHourMm != nil {
					extra += fmt.Sprintf("  last hour: %.1f mm", *e.RainLastHourMm)
				}
				if e.Rain24hMm != nil {
					extra += fmt.Sprintf("  last 24h: %.1f mm", *e.Rain24hMm)
				}
				fmt.Fprintf(w, "  precip:  %.2f mm/h%s\n", *e.PrecipMmH, extra)
			}
			if e.SunPowerWm2 != nil {
				fmt.Fprintf(w, "  sun:     %.0f W/m²\n", *e.SunPowerWm2)
			}
			if e.PollenGrassPct != nil || e.PollenTreePct != nil {
				fmt.Fprint(w, "  pollen: ")
				if e.PollenGrassPct != nil {
					fmt.Fprintf(w, " grass %.0f%%", *e.PollenGrassPct)
				}
				if e.PollenTreePct != nil {
					fmt.Fprintf(w, " tree %.0f%%", *e.PollenTreePct)
				}
				if e.PollenBirchPct != nil {
					fmt.Fprintf(w, " birch %.0f%%", *e.PollenBirchPct)
				}
				if e.PollenWeedPct != nil {
					fmt.Fprintf(w, " weed %.0f%%", *e.PollenWeedPct)
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w)
			continue
		}

		// Time-series entries — compact one-liners.
		line := fmt.Sprintf("  %-19s", e.Time)
		if e.TempC != nil {
			line += fmt.Sprintf("  %5.1f°C", *e.TempC)
		} else if e.MinTempC != nil && e.MaxTempC != nil {
			line += fmt.Sprintf("  %4.0f–%4.0f°C", *e.MinTempC, *e.MaxTempC)
		}
		if e.PrecipMmH != nil {
			line += fmt.Sprintf("  %5.2f mm/h", *e.PrecipMmH)
		} else if e.PrecipMm != nil {
			line += fmt.Sprintf("  %4.1f mm", *e.PrecipMm)
		}
		if e.WindSpeedMS != nil {
			dir := ""
			if e.WindDirection != nil {
				dir = " " + *e.WindDirection
			}
			line += fmt.Sprintf("  %.1f m/s%s", *e.WindSpeedMS, dir)
		}
		if e.Condition != nil {
			line += "  " + *e.Condition
		}
		if e.PollenGrassPct != nil {
			line += fmt.Sprintf("  pollen g%.0f", *e.PollenGrassPct)
			if e.PollenTreePct != nil {
				line += fmt.Sprintf("/t%.0f", *e.PollenTreePct)
			}
		}
		fmt.Fprintln(w, line)
	}
	return nil
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
