package buienradar

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

const KeyScrapeURL = "https://www.buienradar.nl/nederland/neerslag/neerslagkaart"

var apiKeyRE = regexp.MustCompile(`window\.apiKey\s*=\s*['"]([0-9a-f-]{36})['"]`)

type cachedKey struct {
	Key       string    `json:"key"`
	FetchedAt time.Time `json:"fetched_at"`
}

// KeyManager manages the graphdata.buienradar.nl API key. On first use it
// scrapes window.apiKey from the buienradar rain map page and caches it on
// disk. On 401/403 it re-scrapes automatically.
type KeyManager struct {
	// ScrapeURL is the buienradar page that injects window.apiKey into HTML.
	// Overridable for testing.
	ScrapeURL string
	// StorePath is where the scraped key is cached on disk.
	// Overridable for testing.
	StorePath string
	HTTP      *http.Client
	UA        string

	mu     sync.Mutex
	active string // in-memory current key; empty means "load from disk or scrape"
}

// NewKeyManager constructs a KeyManager backed by the user's OS cache dir.
func NewKeyManager(httpClient *http.Client, ua string) *KeyManager {
	cacheDir, _ := os.UserCacheDir()
	return &KeyManager{
		ScrapeURL: KeyScrapeURL,
		StorePath: filepath.Join(cacheDir, "buienradarcli", "api_key.json"),
		HTTP:      httpClient,
		UA:        ua,
	}
}

// Override sets a fixed key, bypassing disk and scraping. Used when the caller
// passes --api-key explicitly.
func (km *KeyManager) Override(key string) {
	km.mu.Lock()
	defer km.mu.Unlock()
	km.active = key
}

// Key returns the current best key: explicit override > disk cache > "" (triggers
// a 401 on first use, which causes RainGraph to auto-scrape and retry).
func (km *KeyManager) Key() string {
	km.mu.Lock()
	defer km.mu.Unlock()

	if km.active != "" {
		return km.active
	}
	if stored, err := km.loadDisk(); err == nil && stored.Key != "" {
		km.active = stored.Key
		return km.active
	}
	return ""
}

// Refresh scrapes a fresh key from the buienradar website, stores it on disk,
// and updates the in-memory key. Called automatically on 401/403.
func (km *KeyManager) Refresh() (string, error) {
	key, err := km.scrape()
	if err != nil {
		return "", fmt.Errorf("key refresh failed: %w", err)
	}

	km.mu.Lock()
	km.active = key
	km.mu.Unlock()

	_ = km.storeDisk(key) // cache on disk; non-fatal if it fails
	return key, nil
}

func (km *KeyManager) scrape() (string, error) {
	req, err := http.NewRequest(http.MethodGet, km.ScrapeURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", km.UA)
	req.Header.Set("Accept", "text/html")

	resp, err := km.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch key page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read key page: %w", err)
	}

	m := apiKeyRE.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("window.apiKey not found on %s", km.ScrapeURL)
	}
	return string(m[1]), nil
}

func (km *KeyManager) loadDisk() (*cachedKey, error) {
	f, err := os.Open(km.StorePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var c cachedKey
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (km *KeyManager) storeDisk(key string) error {
	dir := filepath.Dir(km.StorePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Atomic write via temp-file rename.
	f, err := os.CreateTemp(dir, ".api_key_*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := json.NewEncoder(f).Encode(cachedKey{Key: key, FetchedAt: time.Now()}); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, km.StorePath)
}
