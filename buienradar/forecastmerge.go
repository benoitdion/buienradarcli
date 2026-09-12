package buienradar

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// ForecastEntry is one time-ordered data point in the merged weather forecast.
// All fields except Time are pointers; a nil field means that data was not
// available from any source for this entry.
type ForecastEntry struct {
	Time    string `json:"time"`
	UTCTime string `json:"utc_time,omitempty"`

	// Station identity — only on the first (live observation) entry.
	StationName *string  `json:"station_name,omitempty"`
	StationID   *int     `json:"station_id,omitempty"`
	Region      *string  `json:"region,omitempty"`
	DistanceKM  *float64 `json:"distance_km,omitempty"`

	// Temperature
	TempC       *float64 `json:"temp_c,omitempty"`
	MinTempC    *float64 `json:"min_temp_c,omitempty"`
	MaxTempC    *float64 `json:"max_temp_c,omitempty"`
	FeelsLikeC  *float64 `json:"feels_like_c,omitempty"`
	GroundTempC *float64 `json:"ground_temp_c,omitempty"`

	// Wind
	WindSpeedMS   *float64 `json:"wind_speed_ms,omitempty"`
	WindGustsMS   *float64 `json:"wind_gusts_ms,omitempty"`
	WindBft       *int     `json:"wind_bft,omitempty"`
	WindDirection *string  `json:"wind_direction,omitempty"`
	WindDegrees   *float64 `json:"wind_direction_deg,omitempty"`

	// Atmosphere
	HumidityPct   *float64 `json:"humidity_pct,omitempty"`
	PressureHpa   *float64 `json:"pressure_hpa,omitempty"`
	VisibilityM   *float64 `json:"visibility_m,omitempty"`
	CloudCoverPct *int     `json:"cloud_cover_pct,omitempty"`
	SunshinePct   *int     `json:"sunshine_pct,omitempty"`
	SunPowerWm2   *float64 `json:"sun_power_w_m2,omitempty"`

	// Precipitation
	PrecipMmH      *float64 `json:"precip_mm_h,omitempty"`
	PrecipMm       *float64 `json:"precip_mm,omitempty"`
	RainLastHourMm *float64 `json:"rain_last_hour_mm,omitempty"`
	Rain24hMm      *float64 `json:"rain_last_24h_mm,omitempty"`

	// Pollen (0–100 % on the licht/matig/zwaar scale)
	PollenGrassPct *float64 `json:"pollen_grass_pct,omitempty"`
	PollenTreePct  *float64 `json:"pollen_tree_pct,omitempty"`
	PollenBirchPct *float64 `json:"pollen_birch_pct,omitempty"`
	PollenWeedPct  *float64 `json:"pollen_weed_pct,omitempty"`

	// Condition
	Condition *string `json:"condition,omitempty"`
	IconCode  *string `json:"icon_code,omitempty"`
}

// WeatherForecast is the result of MergedWeatherForecast.
type WeatherForecast struct {
	Lat           float64         `json:"lat"`
	Lon           float64         `json:"lon"`
	Entries       []ForecastEntry `json:"entries"`
	PartialErrors []string        `json:"partial_errors,omitempty"`
}

// MergedWeatherForecast fetches all available weather sources in parallel and
// stitches them into one time-ordered flat entry list, following the same
// resolution-priority logic as MergedRainForecast:
//
//   - Near-term (5-min): live station observation + rain + pollen
//   - Medium-term (hourly): Temp24Hour fills in beyond the rain/pollen window
//   - Long-term (daily): 14-day forecast extends beyond Temp24Hour
func (c *Client) MergedWeatherForecast(ctx context.Context, lat, lon float64) (*WeatherForecast, error) {
	type fetchResult struct {
		name string
		data interface{}
		err  error
	}

	ch := make(chan fetchResult, 8)

	go func() {
		s, err := c.AllStations(ctx)
		ch <- fetchResult{"obs", s, err}
	}()
	go func() {
		g, err := c.RainGraph(ctx, GraphTemp24Hour, lat, lon)
		ch <- fetchResult{"temp24h", g, err}
	}()
	go func() {
		mf, err := c.MergedRainForecast(ctx, lat, lon)
		ch <- fetchResult{"rain", mf, err}
	}()
	for _, p := range []struct{ name, endpoint string }{
		{"pollen_grass", GraphPollenGrass},
		{"pollen_tree", GraphPollenTree},
		{"pollen_birch", GraphPollenBirch},
		{"pollen_weed", GraphPollenWeed},
	} {
		p := p
		go func() {
			g, err := c.RainGraph(ctx, p.endpoint, lat, lon)
			ch <- fetchResult{p.name, g, err}
		}()
	}
	// Location → forecast is serial internally but runs in parallel with the rest.
	go func() {
		loc, err := c.LocationByGeo(ctx, lat, lon)
		if err != nil {
			ch <- fetchResult{"forecast", nil, fmt.Errorf("location lookup: %w", err)}
			return
		}
		fr, err := c.LocationForecast(ctx, loc.ID)
		ch <- fetchResult{"forecast", fr, err}
	}()

	var (
		stations    []StationObservation
		temp24h     *GraphForecast
		rain        *MergedForecast
		pollenGrass *GraphForecast
		pollenTree  *GraphForecast
		pollenBirch *GraphForecast
		pollenWeed  *GraphForecast
		dayForecast *ForecastResponse
		partialErrs []string
	)
	for range 8 {
		r := <-ch
		if r.err != nil {
			partialErrs = append(partialErrs, r.name+": "+r.err.Error())
			continue
		}
		switch r.name {
		case "obs":
			stations = r.data.([]StationObservation)
		case "temp24h":
			temp24h = r.data.(*GraphForecast)
		case "rain":
			rain = r.data.(*MergedForecast)
		case "pollen_grass":
			pollenGrass = r.data.(*GraphForecast)
		case "pollen_tree":
			pollenTree = r.data.(*GraphForecast)
		case "pollen_birch":
			pollenBirch = r.data.(*GraphForecast)
		case "pollen_weed":
			pollenWeed = r.data.(*GraphForecast)
		case "forecast":
			dayForecast = r.data.(*ForecastResponse)
		}
	}

	entries := buildForecastEntries(lat, lon, stations, temp24h, rain,
		pollenGrass, pollenTree, pollenBirch, pollenWeed, dayForecast)

	return &WeatherForecast{
		Lat:           lat,
		Lon:           lon,
		Entries:       entries,
		PartialErrors: partialErrs,
	}, nil
}

func buildForecastEntries(
	lat, lon float64,
	stations []StationObservation,
	temp24h *GraphForecast,
	rain *MergedForecast,
	pollenGrass, pollenTree, pollenBirch, pollenWeed *GraphForecast,
	dayForecast *ForecastResponse,
) []ForecastEntry {
	parseUTC := func(s string) (time.Time, bool) {
		t, err := time.Parse("2006-01-02T15:04:05", s)
		return t, err == nil
	}

	byUTC := make(map[time.Time]*ForecastEntry)
	getOrCreate := func(utc time.Time, localTime string) *ForecastEntry {
		utc = utc.Truncate(time.Minute)
		if e, ok := byUTC[utc]; ok {
			return e
		}
		e := &ForecastEntry{Time: localTime, UTCTime: utc.Format("2006-01-02T15:04:05")}
		byUTC[utc] = e
		return e
	}

	var lastNearTermUTC time.Time
	trackCutoff := func(utc time.Time) {
		if utc.After(lastNearTermUTC) {
			lastNearTermUTC = utc
		}
	}

	// Rain entries (already merged across 3h/8h/48h sources).
	if rain != nil {
		for _, e := range rain.Entries {
			if e.UTCTime == "" {
				continue
			}
			utc, ok := parseUTC(e.UTCTime)
			if !ok {
				continue
			}
			fe := getOrCreate(utc, e.Time)
			fe.PrecipMmH = fptr(e.MMPerH)
			trackCutoff(utc)
		}
	}

	// Pollen — four types, same GraphForecast shape.
	addGraphField := func(g *GraphForecast, set func(*ForecastEntry, float64)) {
		if g == nil {
			return
		}
		for _, e := range g.Entries {
			utc, ok := parseUTC(e.UTCDateTime)
			if !ok {
				continue
			}
			fe := getOrCreate(utc, e.DateTime)
			set(fe, e.Percentage)
			trackCutoff(utc)
		}
	}
	addGraphField(pollenGrass, func(fe *ForecastEntry, v float64) { fe.PollenGrassPct = fptr(v) })
	addGraphField(pollenTree, func(fe *ForecastEntry, v float64) { fe.PollenTreePct = fptr(v) })
	addGraphField(pollenBirch, func(fe *ForecastEntry, v float64) { fe.PollenBirchPct = fptr(v) })
	addGraphField(pollenWeed, func(fe *ForecastEntry, v float64) { fe.PollenWeedPct = fptr(v) })

	// Temp24Hour — merges onto existing entries where timestamps align (top of
	// hour overlaps rain/pollen entries), and adds new hourly entries beyond.
	if temp24h != nil {
		for _, e := range temp24h.Entries {
			utc, ok := parseUTC(e.UTCDateTime)
			if !ok {
				continue
			}
			fe := getOrCreate(utc, e.DateTime)
			fe.TempC = fptr(e.DataValue)
			trackCutoff(utc)
		}
	}

	// 14-day forecast — only entries strictly after near-term coverage ends.
	if dayForecast != nil {
		for _, day := range dayForecast.Days {
			utc, ok := parseUTC(day.DateTimeUTC)
			if !ok || !utc.After(lastNearTermUTC) {
				continue
			}
			// Skip days with no meaningful forecast data (far-future placeholders).
			if day.MinTemperature == 0 && day.MaxTemperature == 0 {
				continue
			}
			fe := getOrCreate(utc, day.DateTime)
			fe.MinTempC = fptr(day.MinTemperature)
			fe.MaxTempC = fptr(day.MaxTemperature)
			if day.PrecipitationMm > 0 {
				fe.PrecipMm = fptr(day.PrecipitationMm)
			}
			if day.CloudCover > 0 {
				fe.CloudCoverPct = iptr(day.CloudCover)
			}
			if day.WindSpeedMS > 0 {
				fe.WindSpeedMS = fptr(day.WindSpeedMS)
				fe.WindBft = iptr(day.Beaufort)
				if day.WindDirection != "" {
					fe.WindDirection = sptr(day.WindDirection)
				}
				if day.WindDegrees > 0 {
					fe.WindDegrees = fptr(day.WindDegrees)
				}
			}
			if day.Humidity > 0 {
				fe.HumidityPct = fptr(float64(day.Humidity))
			}
			if day.SunshinePct > 0 {
				fe.SunshinePct = iptr(day.SunshinePct)
			}
			if day.Visibility > 0 {
				fe.VisibilityM = fptr(day.Visibility)
			}
			if day.IconCode != "" {
				fe.IconCode = sptr(day.IconCode)
				fe.Condition = sptr(Condition(day.IconCode))
			}
		}
	}

	// Sort all near/far-term entries by UTC time.
	utcTimes := make([]time.Time, 0, len(byUTC))
	for t := range byUTC {
		utcTimes = append(utcTimes, t)
	}
	sort.Slice(utcTimes, func(i, j int) bool { return utcTimes[i].Before(utcTimes[j]) })

	// Build the final slice: observation first, then entries after it.
	var obsUTC time.Time
	var result []ForecastEntry

	if len(stations) > 0 {
		if st := NearestStationObs(stations, lat, lon); st != nil {
			obsEntry := stationObsToEntry(st, lat, lon)

			amsterdamLoc, _ := time.LoadLocation("Europe/Amsterdam")
			if amsterdamLoc == nil {
				amsterdamLoc = time.FixedZone("CET", 2*3600)
			}
			if t, err := time.ParseInLocation("2006-01-02T15:04:05", st.Timestamp, amsterdamLoc); err == nil {
				obsUTC = t.UTC()
				obsEntry.UTCTime = obsUTC.Format("2006-01-02T15:04:05")
			}
			result = append(result, *obsEntry)
		}
	}

	for _, t := range utcTimes {
		if !obsUTC.IsZero() && !t.After(obsUTC) {
			continue
		}
		result = append(result, *byUTC[t])
	}

	fillNearestTemperatures(result)
	return result
}

type temperatureAnchor struct {
	time  time.Time
	value float64
}

// fillNearestTemperatures copies the closest hourly temperature onto sparse
// rain and pollen entries. An exact tie uses the earlier value. Daily rows keep
// their min/max temperatures and are never filled with an instantaneous value.
func fillNearestTemperatures(entries []ForecastEntry) {
	anchors := make([]temperatureAnchor, 0, len(entries))
	hasHourlyAnchor := false
	for i := range entries {
		e := &entries[i]
		if e.TempC == nil || e.UTCTime == "" {
			continue
		}
		at, err := time.Parse("2006-01-02T15:04:05", e.UTCTime)
		if err != nil {
			continue
		}
		anchors = append(anchors, temperatureAnchor{time: at, value: *e.TempC})
		if e.StationName == nil {
			hasHourlyAnchor = true
		}
	}
	if !hasHourlyAnchor {
		return
	}

	for i := range entries {
		e := &entries[i]
		if e.TempC != nil || e.UTCTime == "" || !isSparseNearTermEntry(e) {
			continue
		}
		at, err := time.Parse("2006-01-02T15:04:05", e.UTCTime)
		if err != nil {
			continue
		}

		var closest temperatureAnchor
		var closestDistance time.Duration
		found := false
		for _, anchor := range anchors {
			distance := anchor.time.Sub(at)
			if distance < 0 {
				distance = -distance
			}
			if !found || distance < closestDistance {
				closest = anchor
				closestDistance = distance
				found = true
			}
		}
		if found {
			e.TempC = fptr(closest.value)
		}
	}
}

func isSparseNearTermEntry(e *ForecastEntry) bool {
	return e.PrecipMmH != nil || e.PollenGrassPct != nil || e.PollenTreePct != nil ||
		e.PollenBirchPct != nil || e.PollenWeedPct != nil
}

func stationObsToEntry(st *StationObservation, lat, lon float64) *ForecastEntry {
	e := &ForecastEntry{
		Time:          st.Timestamp,
		StationName:   sptr(st.StationName),
		StationID:     iptr(st.StationID),
		Region:        sptr(st.Regio),
		DistanceKM:    fptr(obsHaversineKM(lat, lon, st.Lat, st.Lon)),
		TempC:         fptr(st.Temperature),
		FeelsLikeC:    fptr(st.FeelTemperature),
		GroundTempC:   fptr(st.GroundTemperature),
		HumidityPct:   fptr(st.Humidity),
		WindSpeedMS:   fptr(st.WindSpeed),
		WindBft:       iptr(st.WindSpeedBft),
		WindDirection: sptr(st.WindDirection),
		WindDegrees:   fptr(st.WindDirectionDeg),
		PrecipMmH:     fptr(st.Precipitation),
	}
	if st.AirPressure > 0 {
		e.PressureHpa = fptr(st.AirPressure)
	}
	if st.Visibility > 0 {
		e.VisibilityM = fptr(st.Visibility)
	}
	if st.WindGusts > 0 {
		e.WindGustsMS = fptr(st.WindGusts)
	}
	if st.SunPower > 0 {
		e.SunPowerWm2 = fptr(st.SunPower)
	}
	if st.RainFallLastHour > 0 {
		e.RainLastHourMm = fptr(st.RainFallLastHour)
	}
	if st.RainFallLast24Hour > 0 {
		e.Rain24hMm = fptr(st.RainFallLast24Hour)
	}
	if st.IconCode != "" {
		e.IconCode = sptr(st.IconCode)
		e.Condition = sptr(Condition(st.IconCode))
	}
	return e
}

func obsHaversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func fptr(v float64) *float64 { return &v }
func iptr(v int) *int         { return &v }
func sptr(v string) *string   { return &v }
