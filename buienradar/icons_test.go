package buienradar

import "testing"

func TestConditionsFromIcon(t *testing.T) {
	tests := []struct {
		code string
		want Conditions
		ok   bool
	}{
		{"a", Conditions{Clouds: CloudsClear, Precipitation: PrecipitationNone}, true},
		{"aa", Conditions{Clouds: CloudsClear, Precipitation: PrecipitationNone, Night: true}, true},
		{"j", Conditions{Clouds: CloudsMostlySunny, Precipitation: PrecipitationNone}, true},
		{"o", Conditions{Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationNone}, true},
		{"rr", Conditions{Clouds: CloudsMostlyCloudy, Precipitation: PrecipitationNone, Night: true}, true},
		{"p", Conditions{Clouds: CloudsCloudy, Precipitation: PrecipitationNone}, true},
		{"n", Conditions{Clouds: CloudsClear, Precipitation: PrecipitationNone, Fog: true}, true},
		{"k", Conditions{Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationLightRain}, true},
		{"m", Conditions{Clouds: CloudsCloudy, Precipitation: PrecipitationLightRain}, true},
		{"q", Conditions{Clouds: CloudsCloudy, Precipitation: PrecipitationRain}, true},
		{"g", Conditions{Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationThunderstorm}, true},
		{"v", Conditions{Clouds: CloudsCloudy, Precipitation: PrecipitationLightSnow}, true},
		{"i", Conditions{Clouds: CloudsPartlyCloudy, Precipitation: PrecipitationSnow}, true},
		{"w", Conditions{Clouds: CloudsCloudy, Precipitation: PrecipitationSleet}, true},
		{"e", Conditions{}, false},
		{"ab", Conditions{}, false},
		{"", Conditions{}, false},
	}
	for _, tt := range tests {
		got, ok := conditionsFromIcon(tt.code)
		if got != tt.want || ok != tt.ok {
			t.Errorf("conditionsFromIcon(%q) = %+v, %v; want %+v, %v", tt.code, got, ok, tt.want, tt.ok)
		}
	}
}
