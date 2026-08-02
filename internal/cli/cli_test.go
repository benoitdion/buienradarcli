package cli

import (
	"math"
	"testing"
)

func TestValidateCoordinates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		lat     float64
		lon     float64
		wantErr bool
	}{
		{name: "Amsterdam", lat: DefaultLat, lon: DefaultLon},
		{name: "bounds", lat: -90, lon: 180},
		{name: "latitude too low", lat: -90.1, wantErr: true},
		{name: "latitude too high", lat: 90.1, wantErr: true},
		{name: "longitude too low", lon: -180.1, wantErr: true},
		{name: "longitude too high", lon: 180.1, wantErr: true},
		{name: "NaN latitude", lat: math.NaN(), wantErr: true},
		{name: "infinite longitude", lon: math.Inf(1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCoordinates(tt.lat, tt.lon)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCoordinates(%v, %v) error = %v, wantErr %v", tt.lat, tt.lon, err, tt.wantErr)
			}
		})
	}
}
