package buienradar

import "strings"

// Clouds says how much of the sky is covered.
type Clouds string

const (
	CloudsClear        Clouds = "clear"
	CloudsMostlySunny  Clouds = "mostly-sunny"
	CloudsPartlyCloudy Clouds = "partly-cloudy"
	CloudsMostlyCloudy Clouds = "mostly-cloudy"
	CloudsCloudy       Clouds = "cloudy"
)

// Precipitation says what is falling.
type Precipitation string

const (
	PrecipitationNone         Precipitation = "none"
	PrecipitationLightRain    Precipitation = "light-rain"
	PrecipitationRain         Precipitation = "rain"
	PrecipitationThunderstorm Precipitation = "thunderstorm"
	PrecipitationLightSnow    Precipitation = "light-snow"
	PrecipitationSnow         Precipitation = "snow"
	PrecipitationSleet        Precipitation = "sleet"
)

// Conditions describes the weather without Buienradar's icon codes.
type Conditions struct {
	Clouds        Clouds        `json:"clouds"`
	Precipitation Precipitation `json:"precipitation"`
	Fog           bool          `json:"fog"`
	Night         bool          `json:"night"`
}

// Conditions reports false when Buienradar sent a code it doesn't define.
func (s StationObservation) Conditions() (Conditions, bool) { return conditionsFromIcon(s.IconCode) }

func (d ForecastDay) Conditions() (Conditions, bool) { return conditionsFromIcon(d.IconCode) }

func (h ForecastHour) Conditions() (Conditions, bool) { return conditionsFromIcon(h.IconCode) }

// iconConditions follows the icon Buienradar draws for each code; a doubled
// letter is the night version of the same weather.
var iconConditions = map[byte]Conditions{
	'a': {Clouds: CloudsClear, Precipitation: PrecipitationNone},
	'j': {Clouds: CloudsMostlySunny, Precipitation: PrecipitationNone},
	'b': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationNone},
	'o': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationNone},
	'r': {Clouds: CloudsMostlyCloudy, Precipitation: PrecipitationNone},
	'c': {Clouds: CloudsCloudy, Precipitation: PrecipitationNone},
	'p': {Clouds: CloudsCloudy, Precipitation: PrecipitationNone},
	'n': {Clouds: CloudsClear, Precipitation: PrecipitationNone, Fog: true},
	'd': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationNone, Fog: true},
	'f': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationLightRain},
	'k': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationLightRain},
	'h': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationRain},
	'm': {Clouds: CloudsCloudy, Precipitation: PrecipitationLightRain},
	'l': {Clouds: CloudsCloudy, Precipitation: PrecipitationRain},
	'q': {Clouds: CloudsCloudy, Precipitation: PrecipitationRain},
	'g': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationThunderstorm},
	's': {Clouds: CloudsCloudy, Precipitation: PrecipitationThunderstorm},
	'u': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationLightSnow},
	'i': {Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationSnow},
	'v': {Clouds: CloudsCloudy, Precipitation: PrecipitationLightSnow},
	't': {Clouds: CloudsCloudy, Precipitation: PrecipitationSnow},
	'w': {Clouds: CloudsCloudy, Precipitation: PrecipitationSleet},
}

func conditionsFromIcon(code string) (Conditions, bool) {
	code = strings.ToLower(code)
	if code == "" || len(code) > 2 || (len(code) == 2 && code[1] != code[0]) {
		return Conditions{}, false
	}
	c, ok := iconConditions[code[0]]
	c.Night = ok && len(code) == 2
	return c, ok
}
