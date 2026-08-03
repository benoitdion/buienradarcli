//go:build e2e

package cli_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var apiKeyPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type smokeEnvelope struct {
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Data    json.RawMessage `json:"data"`
}

type smokeRainResult struct {
	PartialErrors []string `json:"partial_errors"`
	Entries       []struct {
		Time   string  `json:"time"`
		MMPerH float64 `json:"mm_per_h"`
	} `json:"entries"`
}

type smokeForecastResult struct {
	PartialErrors []string `json:"partial_errors"`
	Entries       []struct {
		Time           string   `json:"time"`
		StationName    *string  `json:"station_name"`
		TempC          *float64 `json:"temp_c"`
		MinTempC       *float64 `json:"min_temp_c"`
		MaxTempC       *float64 `json:"max_temp_c"`
		PrecipMmH      *float64 `json:"precip_mm_h"`
		PollenGrassPct *float64 `json:"pollen_grass_pct"`
		PollenTreePct  *float64 `json:"pollen_tree_pct"`
		PollenBirchPct *float64 `json:"pollen_birch_pct"`
		PollenWeedPct  *float64 `json:"pollen_weed_pct"`
	} `json:"entries"`
}

func TestCLIEndToEnd(t *testing.T) {
	repoRoot := repositoryRoot(t)
	smokeRoot := t.TempDir()
	binary := filepath.Join(smokeRoot, "buienradarcli")

	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	home := filepath.Join(smokeRoot, "home")
	cache := filepath.Join(smokeRoot, "cache")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("create temporary home: %v", err)
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatalf("create temporary cache: %v", err)
	}

	lat := envOrDefault("BUIENRADAR_SMOKE_LAT", "52.3676")
	lon := envOrDefault("BUIENRADAR_SMOKE_LON", "4.9041")
	env := smokeEnvironment(home, cache)
	rainOutput := runSmokeCommand(t, env, binary,
		"rain", "--lat", lat, "--lon", lon, "--output", "json",
	)
	verifyRainOutput(t, rainOutput)
	verifyExtractedKey(t, smokeRoot)

	forecastOutput := runSmokeCommand(t, env, binary,
		"forecast", "--lat", lat, "--lon", lon, "--output", "json",
	)
	verifyForecastOutput(t, forecastOutput)
	t.Log("PASS: key extraction, rain graphs, and complete forecast temperature coverage")
}

func runSmokeCommand(t *testing.T, env []string, binary string, args ...string) []byte {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Env = env
	output, err := command.Output()
	if err == nil {
		return output
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("run %s: %v\n%s", strings.Join(args, " "), err, exitErr.Stderr)
	}
	t.Fatalf("run %s: %v", strings.Join(args, " "), err)
	return nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate smoke test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func smokeEnvironment(home, cache string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "HOME=") ||
			strings.HasPrefix(entry, "XDG_CACHE_HOME=") ||
			strings.HasPrefix(entry, "BUIENRADAR_API_KEY=") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "HOME="+home, "XDG_CACHE_HOME="+cache)
}

func verifyRainOutput(t *testing.T, output []byte) {
	t.Helper()
	var envelope smokeEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("decode CLI JSON output: %v", err)
	}
	if !envelope.OK || envelope.Command != "rain" {
		t.Fatalf("unexpected CLI envelope: ok=%v command=%q", envelope.OK, envelope.Command)
	}

	var rain smokeRainResult
	if err := json.Unmarshal(envelope.Data, &rain); err != nil {
		t.Fatalf("decode rain result: %v", err)
	}
	if len(rain.PartialErrors) > 0 {
		t.Fatalf("rain graph endpoint failures: %s", strings.Join(rain.PartialErrors, ", "))
	}
	if len(rain.Entries) < 2 {
		t.Fatalf("rain entries = %d, want at least 2", len(rain.Entries))
	}
	for i, entry := range rain.Entries {
		if entry.Time == "" {
			t.Fatalf("rain entry %d has no timestamp", i)
		}
		if entry.MMPerH < 0 || invalidNumber(entry.MMPerH) {
			t.Fatalf("rain entry %d has invalid precipitation value %v", i, entry.MMPerH)
		}
	}
}

func verifyForecastOutput(t *testing.T, output []byte) {
	t.Helper()
	var envelope smokeEnvelope
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("decode forecast CLI JSON output: %v", err)
	}
	if !envelope.OK || envelope.Command != "forecast" {
		t.Fatalf("unexpected forecast envelope: ok=%v command=%q", envelope.OK, envelope.Command)
	}

	var forecast smokeForecastResult
	if err := json.Unmarshal(envelope.Data, &forecast); err != nil {
		t.Fatalf("decode forecast result: %v", err)
	}
	if len(forecast.PartialErrors) > 0 {
		t.Fatalf("forecast source failures: %s", strings.Join(forecast.PartialErrors, ", "))
	}
	if len(forecast.Entries) < 2 {
		t.Fatalf("forecast entries = %d, want at least 2", len(forecast.Entries))
	}

	liveRows := 0
	nearTermRows := 0
	dailyRows := 0
	for i, entry := range forecast.Entries {
		if entry.Time == "" {
			t.Fatalf("forecast entry %d has no timestamp", i)
		}
		if entry.StationName != nil {
			liveRows++
		}
		if entry.PrecipMmH != nil || entry.PollenGrassPct != nil || entry.PollenTreePct != nil ||
			entry.PollenBirchPct != nil || entry.PollenWeedPct != nil {
			nearTermRows++
		}
		if entry.MinTempC != nil || entry.MaxTempC != nil {
			dailyRows++
			if entry.MinTempC == nil || entry.MaxTempC == nil {
				t.Fatalf("daily forecast entry %d is missing a temperature bound", i)
			}
			if invalidNumber(*entry.MinTempC) || invalidNumber(*entry.MaxTempC) || *entry.MinTempC > *entry.MaxTempC {
				t.Fatalf("daily forecast entry %d has invalid temperature range", i)
			}
			continue
		}
		if entry.TempC == nil {
			t.Fatalf("forecast entry %d at %s has no temperature data", i, entry.Time)
		}
		if invalidNumber(*entry.TempC) {
			t.Fatalf("forecast entry %d has invalid temperature %v", i, *entry.TempC)
		}
	}
	if liveRows == 0 {
		t.Fatal("forecast contains no live observation")
	}
	if nearTermRows == 0 {
		t.Fatal("forecast contains no rain or pollen timeline rows")
	}
	if dailyRows == 0 {
		t.Fatal("forecast contains no daily temperature ranges")
	}
}

func invalidNumber(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0)
}

func verifyExtractedKey(t *testing.T, smokeRoot string) {
	t.Helper()
	var keyFile string
	err := filepath.WalkDir(smokeRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "api_key.json" && filepath.Base(filepath.Dir(path)) == "buienradarcli" {
			keyFile = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatalf("find extracted key cache: %v", err)
	}
	if keyFile == "" {
		t.Fatal("fresh key was not cached after the live request")
	}

	contents, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("read extracted key cache: %v", err)
	}
	var cached struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(contents, &cached); err != nil {
		t.Fatalf("decode extracted key cache: %v", err)
	}
	if !apiKeyPattern.MatchString(cached.Key) {
		t.Fatal("cached API key does not have the expected UUID format")
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
