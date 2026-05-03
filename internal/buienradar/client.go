// Package buienradar is a thin client for the public Buienradar APIs.
package buienradar

import (
	"context"
	"encoding/json"
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

const (
	FeedURL          = "https://data.buienradar.nl/2.0/feed/json"
	PrecipitationURL = "https://gps.buienradar.nl/getrr.php"
)

type Client struct {
	HTTP    *http.Client
	BaseURL string
	RainURL string
	UA      string
	Keys    *KeyManager
}

func NewClient() *Client {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	ua := "buienradarcli/0.1 (+https://github.com/benoitdion/buienradarcli)"
	return &Client{
		HTTP:    httpClient,
		BaseURL: FeedURL,
		RainURL: PrecipitationURL,
		UA:      ua,
		Keys:    NewKeyManager(httpClient, ua),
	}
}

type FeedResponse struct {
	Buienradar struct {
		Copyright string `json:"copyright"`
		Terms     string `json:"terms"`
	} `json:"$"`
	Actual   Actual   `json:"actual"`
	Forecast Forecast `json:"forecast"`
}

type Actual struct {
	ActualRadarURL      string               `json:"actualradarurl"`
	SunRise             string               `json:"sunrise"`
	SunSet              string               `json:"sunset"`
	StationMeasurements []StationMeasurement `json:"stationmeasurements"`
}

type StationMeasurement struct {
	StationID          int     `json:"stationid"`
	StationName        string  `json:"stationname"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	Regio              string  `json:"regio"`
	Timestamp          string  `json:"timestamp"`
	WeatherDescription string  `json:"weatherdescription"`
	IconURL            string  `json:"iconurl"`
	GraphURL           string  `json:"graphUrl"`
	Temperature        float64 `json:"temperature"`
	GroundTemperature  float64 `json:"groundtemperature"`
	FeelTemperature    float64 `json:"feeltemperature"`
	WindGusts          float64 `json:"windgusts"`
	WindSpeed          float64 `json:"windspeed"`
	WindSpeedBft       float64 `json:"windspeedBft"`
	Humidity           float64 `json:"humidity"`
	Precipitation      float64 `json:"precipitation"`
	SunPower           float64 `json:"sunpower"`
	RainFallLastHour   float64 `json:"rainFallLastHour"`
	RainFallLast24Hour float64 `json:"rainFallLast24Hour"`
	WindDirection      string  `json:"winddirection"`
	WindDirectionDeg   float64 `json:"winddirectiondegrees"`
	AirPressure        float64 `json:"airpressure"`
	Visibility         float64 `json:"visibility"`
}

type Forecast struct {
	WeatherReport struct {
		Published string `json:"published"`
		Title     string `json:"title"`
		Summary   string `json:"summary"`
		Author    string `json:"author"`
	} `json:"weatherreport"`
	ShortTerm struct {
		StartDate string `json:"startdate"`
		EndDate   string `json:"enddate"`
		Forecast  string `json:"forecast"`
	} `json:"shortterm"`
	LongTerm struct {
		StartDate string `json:"startdate"`
		EndDate   string `json:"enddate"`
		Forecast  string `json:"forecast"`
	} `json:"longterm"`
	FiveDay []DayForecast `json:"fivedayforecast"`
}

type DayForecast struct {
	Day                string  `json:"day"`
	MinTemperatureRaw  string  `json:"mintemperature"`
	MaxTemperatureRaw  string  `json:"maxtemperature"`
	MinTemperature     float64 `json:"mintemperatureMin"`
	MaxTemperature     float64 `json:"maxtemperatureMax"`
	RainChance         int     `json:"rainChance"`
	SunChance          int     `json:"sunChance"`
	WindDirection      string  `json:"windDirection"`
	Wind               int     `json:"wind"`
	MMRainMin          float64 `json:"mmRainMin"`
	MMRainMax          float64 `json:"mmRainMax"`
	WeatherDescription string  `json:"weatherdescription"`
	IconURL            string  `json:"iconurl"`
}

func (c *Client) Feed(ctx context.Context) (*FeedResponse, error) {
	body, err := c.get(ctx, c.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	var feed FeedResponse
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("decode feed: %w", err)
	}
	return &feed, nil
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

// NearestStation returns the station closest to (lat, lon) by squared-degree
// distance. Adequate for the small geographic span of the Netherlands.
func NearestStation(stations []StationMeasurement, lat, lon float64) *StationMeasurement {
	if len(stations) == 0 {
		return nil
	}
	bestIdx := -1
	bestD := math.Inf(1)
	for i, s := range stations {
		dlat := s.Lat - lat
		dlon := s.Lon - lon
		d := dlat*dlat + dlon*dlon
		if d < bestD {
			bestD = d
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil
	}
	return &stations[bestIdx]
}
