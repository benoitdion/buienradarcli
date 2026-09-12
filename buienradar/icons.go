package buienradar

// Condition maps a Buienradar icon code to a stable English condition label.
// Source: https://github.com/mjj4791/python-buienradar/blob/master/buienradar/buienradar_json.py
func Condition(iconCode string) string {
	base := iconCode
	if len(base) > 1 {
		base = string(base[0])
	}
	switch base {
	case "a":
		return "clear"
	case "b":
		return "partly-cloudy"
	case "c":
		return "cloudy"
	case "d", "n":
		return "fog"
	case "f", "q":
		return "light-rain"
	case "g", "s":
		return "thunderstorm"
	case "h", "l":
		return "heavy-rain"
	case "i", "u":
		return "light-snow"
	case "j", "r":
		return "partly-cloudy-rain"
	case "k":
		return "rain-showers"
	case "m", "w":
		return "rain-and-snow"
	case "o":
		return "partly-cloudy"
	case "p":
		return "cloudy"
	case "t":
		return "heavy-snow"
	case "v":
		return "snow-showers"
	default:
		return "unknown"
	}
}
