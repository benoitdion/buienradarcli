package buienradar

import "testing"

func TestFillNearestTemperatures(t *testing.T) {
	liveStation := "Amsterdam"
	liveTemp := 10.0
	hourlyTemp := 20.0
	zeroRain := 0.0
	minTemp := 8.0
	maxTemp := 16.0

	entries := []ForecastEntry{
		{UTCTime: "2026-08-03T12:00:00", TempC: &liveTemp, StationName: &liveStation},
		{UTCTime: "2026-08-03T12:15:00", PrecipMmH: &zeroRain},
		{UTCTime: "2026-08-03T12:30:00", PrecipMmH: &zeroRain},
		{UTCTime: "2026-08-03T12:45:00", PrecipMmH: &zeroRain},
		{UTCTime: "2026-08-03T13:00:00", TempC: &hourlyTemp},
		{UTCTime: "2026-08-03T13:15:00"},
		{UTCTime: "2026-08-04T12:00:00", MinTempC: &minTemp, MaxTempC: &maxTemp},
	}

	fillNearestTemperatures(entries)

	assertTemp := func(index int, want *float64) {
		t.Helper()
		got := entries[index].TempC
		if want == nil {
			if got != nil {
				t.Fatalf("entries[%d].TempC = %v, want nil", index, *got)
			}
			return
		}
		if got == nil || *got != *want {
			t.Fatalf("entries[%d].TempC = %v, want %v", index, got, *want)
		}
	}

	assertTemp(1, &liveTemp)
	assertTemp(2, &liveTemp) // Equal distance chooses the earlier observation.
	assertTemp(3, &hourlyTemp)
	assertTemp(5, nil) // Rows without rain or pollen remain sparse.
	assertTemp(6, nil) // Daily min/max rows do not gain temp_c.
}

func TestFillNearestTemperaturesRequiresHourlyForecast(t *testing.T) {
	liveStation := "Amsterdam"
	liveTemp := 10.0
	zeroRain := 0.0
	entries := []ForecastEntry{
		{UTCTime: "2026-08-03T12:00:00", TempC: &liveTemp, StationName: &liveStation},
		{UTCTime: "2026-08-03T12:15:00", PrecipMmH: &zeroRain},
	}

	fillNearestTemperatures(entries)

	if entries[1].TempC != nil {
		t.Fatalf("rain entry received temperature %v without an hourly forecast", *entries[1].TempC)
	}
}
