package buienradar

import (
	"context"
	"encoding/json"
	"fmt"
)

const ForecastAPIBase = "https://forecast.buienradar.nl/2.0/forecast"

type ForecastResponse struct {
	Days []ForecastDay `json:"days"`
}

// ForecastDay holds the day-level summary fields that are present on every
// day in the 14-day forecast, plus the hourly entries the first week carries.
type ForecastDay struct {
	DateTime        string         `json:"datetime"`
	DateTimeUTC     string         `json:"datetimeutc"`
	MinTemperature  float64        `json:"mintemperature"`
	MaxTemperature  float64        `json:"maxtemperature"`
	PrecipitationMm float64        `json:"precipitationmm"`
	CloudCover      int            `json:"cloudcover"`
	IconCode        string         `json:"iconcode"`
	WindDirection   string         `json:"winddirection"`
	WindDegrees     float64        `json:"winddirectiondegrees"`
	WindSpeedMS     float64        `json:"windspeedms"`
	Beaufort        int            `json:"beaufort"`
	Humidity        int            `json:"humidity"`
	SunshinePct     int            `json:"sunshine"`
	Visibility      float64        `json:"visibility"`
	UVIndex         int            `json:"uvindex"`
	PollenIndex     int            `json:"pollenindex"`
	MoonAgeDays     float64        `json:"moonAge"`
	Hours           []ForecastHour `json:"hours"`
}

type ForecastHour struct {
	DateTime        string  `json:"datetime"`
	DateTimeUTC     string  `json:"datetimeutc"`
	Temperature     float64 `json:"temperature"`
	PrecipitationMm float64 `json:"precipitationmm"`
	CloudCover      int     `json:"cloudcover"`
	SunshinePct     int     `json:"sunshine"`
	IconCode        string  `json:"iconcode"`
}

func (c *Client) LocationForecast(ctx context.Context, locationID int) (*ForecastResponse, error) {
	u := fmt.Sprintf("%s/%d", ForecastAPIBase, locationID)
	body, err := c.get(ctx, u, nil)
	if err != nil {
		return nil, fmt.Errorf("forecast api: %w", err)
	}
	var resp ForecastResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode forecast: %w", err)
	}
	return &resp, nil
}
