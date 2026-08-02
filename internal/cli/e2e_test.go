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

func TestCLIColdStartKeyExtraction(t *testing.T) {
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
	command := exec.CommandContext(t.Context(), binary,
		"rain", "--lat", lat, "--lon", lon, "--output", "json",
	)
	command.Env = smokeEnvironment(home, cache)
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("run live rain forecast: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("run live rain forecast: %v", err)
	}

	verifyRainOutput(t, output)
	verifyExtractedKey(t, smokeRoot)
	t.Log("PASS: fresh key extracted, cached privately, and accepted by all rain graph endpoints")
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
		if entry.MMPerH < 0 || math.IsNaN(entry.MMPerH) || math.IsInf(entry.MMPerH, 0) {
			t.Fatalf("rain entry %d has invalid precipitation value %v", i, entry.MMPerH)
		}
	}
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
