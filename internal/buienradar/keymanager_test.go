package buienradar_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benoitdion/buienradarcli/internal/buienradar"
)

const (
	fakeKey = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	realKey = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

// fakeForecast returns a minimal valid GraphForecast JSON body.
func fakeForecast() string {
	return `{"borders":[],"unit":"mm/u","forecasts":[
		{"dateTime":"2026-01-01T10:00:00","utcDateTime":"2026-01-01T09:00:00","dataValue":0.5,"percentageValue":10}
	]}`
}

// scrapeHTML mimics the buienradar.nl rain map page with window.apiKey injected.
func scrapeHTML(key string) string {
	return fmt.Sprintf(`<html><head></head><body><script>window.apiKey = '%s';</script></body></html>`, key)
}

func TestRainGraph_AutoRefreshOnAuthFailure(t *testing.T) {
	callCount := 0

	// graphdata server: rejects fakeKey with 401, accepts realKey with 200.
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		ak := r.URL.Query().Get("ak")
		if ak == fakeKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if ak == realKey {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, fakeForecast())
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer graphSrv.Close()

	// scrape server: serves the HTML page containing the real key.
	scrapeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, scrapeHTML(realKey))
	}))
	defer scrapeSrv.Close()

	// Build a client wired to our test servers.
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "api_key.json")

	client := buienradar.NewClient()
	client.Keys = &buienradar.KeyManager{
		ScrapeURL: scrapeSrv.URL,
		StorePath: storePath,
		HTTP:      client.HTTP,
		UA:        "test",
	}
	// Inject the fake key as if the user passed --api-key fake...
	client.Keys.Override(fakeKey)

	// Call an endpoint on our fake graphdata server.
	g, err := client.RainGraph(context.Background(), graphSrv.URL, 52.0, 5.0)
	if err != nil {
		t.Fatalf("expected success after refresh, got: %v", err)
	}
	if len(g.Entries) == 0 {
		t.Fatal("expected at least one forecast entry")
	}

	// Should have made exactly 2 calls: first with fakeKey (401), then with realKey (200).
	if callCount != 2 {
		t.Errorf("expected 2 graphdata calls (fail + retry), got %d", callCount)
	}

	// Key should be persisted on disk.
	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("expected key to be stored on disk: %v", err)
	}
	var stored struct{ Key string }
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("stored key is not valid JSON: %v", err)
	}
	if stored.Key != realKey {
		t.Errorf("stored key = %q, want %q", stored.Key, realKey)
	}
}

func TestRainGraph_LoadsKeyFromDisk(t *testing.T) {
	callCount := 0

	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Query().Get("ak") == realKey {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, fakeForecast())
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer graphSrv.Close()

	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "api_key.json")

	// Pre-write a cached key on disk.
	_ = os.WriteFile(storePath, []byte(fmt.Sprintf(`{"key":%q,"fetched_at":"2026-01-01T00:00:00Z"}`, realKey)), 0o600)

	client := buienradar.NewClient()
	client.Keys = &buienradar.KeyManager{
		ScrapeURL: "http://should-not-be-called",
		StorePath: storePath,
		HTTP:      client.HTTP,
		UA:        "test",
	}
	// No Override() call — should load from disk automatically.

	_, err := client.RainGraph(context.Background(), graphSrv.URL, 52.0, 5.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry needed), got %d", callCount)
	}
}

func TestRainGraph_RefreshFailureSurfacesError(t *testing.T) {
	// graphdata always returns 401.
	graphSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer graphSrv.Close()

	// scrape server returns HTML without the apiKey.
	scrapeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><body>no key here</body></html>")
	}))
	defer scrapeSrv.Close()

	client := buienradar.NewClient()
	client.Keys = &buienradar.KeyManager{
		ScrapeURL: scrapeSrv.URL,
		StorePath: filepath.Join(t.TempDir(), "api_key.json"),
		HTTP:      client.HTTP,
		UA:        "test",
	}
	client.Keys.Override(fakeKey)

	_, err := client.RainGraph(context.Background(), graphSrv.URL, 52.0, 5.0)
	if err == nil {
		t.Fatal("expected error when both graphdata and scrape fail")
	}
	if !strings.Contains(err.Error(), "key refresh failed") {
		t.Errorf("expected 'key refresh failed' in error, got: %v", err)
	}
}

func TestKeyManager_Refresh_ScrapesAndStores(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, scrapeHTML(realKey))
	}))
	defer srv.Close()

	storePath := filepath.Join(t.TempDir(), "api_key.json")
	km := &buienradar.KeyManager{
		ScrapeURL: srv.URL,
		StorePath: storePath,
		HTTP:      &http.Client{},
		UA:        "test",
	}

	got, err := km.Refresh()
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if got != realKey {
		t.Errorf("Refresh returned %q, want %q", got, realKey)
	}
	if km.Key() != realKey {
		t.Errorf("Key() after Refresh = %q, want %q", km.Key(), realKey)
	}

	// Verify disk.
	raw, _ := os.ReadFile(storePath)
	if !strings.Contains(string(raw), realKey) {
		t.Errorf("key not found in stored file: %s", raw)
	}
}
