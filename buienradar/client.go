// Package buienradar is a thin client for the public Buienradar APIs.
package buienradar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrUnauthorized is returned when a graphdata endpoint rejects the API key
// (HTTP 401 or 403). The caller should refresh the key and retry.
var ErrUnauthorized = errors.New("unauthorized: api key rejected")

const PrecipitationURL = "https://gps.buienradar.nl/getrr.php"

type Client struct {
	HTTP    *http.Client
	RainURL string
	UA      string
	Keys    *KeyManager
}

func NewClient() *Client {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	ua := "buienradarcli (+https://github.com/benoitdion/buienradarcli)"
	return NewClientWithHTTP(httpClient, ua)
}

// NewClientWithHTTP constructs a client with caller-owned HTTP transport and identity.
func NewClientWithHTTP(httpClient *http.Client, userAgent string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		HTTP:    httpClient,
		RainURL: PrecipitationURL,
		UA:      userAgent,
		Keys:    NewKeyManager(httpClient, userAgent),
	}
}

type RainEntry struct {
	Time     string  `json:"time"`
	MMPerH   float64 `json:"mm_per_h"`
	RawValue int     `json:"raw_value"`
}

// PrecipitationForecast fetches the 5-minute precipitation forecast for a
// location. The Buienradar endpoint typically returns ~24 entries spanning
// the next ~2 hours — that is the documented horizon for this feed.
func (c *Client) PrecipitationForecast(ctx context.Context, lat, lon float64) ([]RainEntry, error) {
	q := map[string]string{
		"lat": fmt.Sprintf("%.4f", lat),
		"lon": fmt.Sprintf("%.4f", lon),
	}
	body, err := c.get(ctx, c.RainURL, q)
	if err != nil {
		return nil, err
	}

	entries := make([]RainEntry, 0, 32)
	for _, line := range strings.Fields(string(body)) {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		raw, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		mm := 0.0
		if raw > 0 {
			mm = math.Pow(10, (float64(raw)-109)/32)
		}
		entries = append(entries, RainEntry{
			Time:     parts[1],
			MMPerH:   mm,
			RawValue: raw,
		})
	}
	if len(entries) == 0 {
		return nil, errors.New("empty precipitation response")
	}
	return entries, nil
}

func (c *Client) get(ctx context.Context, u string, query map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		q := req.URL.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	req.Header.Set("User-Agent", c.UA)
	req.Header.Set("Accept", "application/json, text/plain;q=0.9")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, u)
	}
	return body, nil
}
