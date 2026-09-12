package buienradar

import (
	"context"
	"encoding/json"
	"fmt"
)

const LocationGeoURL = "https://location.buienradar.nl/1.1/location/geo"

type LocationResult struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ASCIIName   string `json:"asciiname"`
	CountryCode string `json:"countrycode"`
	Location    struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"location"`
}

func (c *Client) LocationByGeo(ctx context.Context, lat, lon float64) (*LocationResult, error) {
	q := map[string]string{
		"lat": fmt.Sprintf("%.4f", lat),
		"lon": fmt.Sprintf("%.4f", lon),
	}
	body, err := c.get(ctx, LocationGeoURL, q)
	if err != nil {
		return nil, err
	}
	var result LocationResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode location: %w", err)
	}
	if result.ID == 0 {
		return nil, fmt.Errorf("no location found for %.4f,%.4f", lat, lon)
	}
	return &result, nil
}
