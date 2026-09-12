package buienradar

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

const ObservationsURL = "https://observations.buienradar.nl/1.0/actual/weatherstation/all"

type StationObservation struct {
	StationID          int     `json:"stationid"`
	StationName        string  `json:"stationname"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	Regio              string  `json:"regio"`
	Timestamp          string  `json:"timestamp"`
	IconCode           string  `json:"iconcode"`
	WindDirection      string  `json:"winddirection"`
	WindDirectionDeg   float64 `json:"winddirectiondegrees"`
	AirPressure        float64 `json:"airpressure"`
	Temperature        float64 `json:"temperature"`
	GroundTemperature  float64 `json:"groundtemperature"`
	FeelTemperature    float64 `json:"feeltemperature"`
	Visibility         float64 `json:"visibility"`
	WindGusts          float64 `json:"windgusts"`
	WindSpeed          float64 `json:"windspeed"`
	WindSpeedBft       int     `json:"windspeedBft"`
	Humidity           float64 `json:"humidity"`
	Precipitation      float64 `json:"precipitation"`
	SunPower           float64 `json:"sunpower"`
	RainFallLastHour   float64 `json:"rainFallLastHour"`
	RainFallLast24Hour float64 `json:"rainFallLast24Hour"`
}

func (c *Client) AllStations(ctx context.Context) ([]StationObservation, error) {
	body, err := c.get(ctx, ObservationsURL, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Stations []StationObservation `json:"weatherstations"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode observations: %w", err)
	}
	return resp.Stations, nil
}

func NearestStationObs(stations []StationObservation, lat, lon float64) *StationObservation {
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
