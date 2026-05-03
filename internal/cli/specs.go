package cli

type SpecOption struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

type CommandSpec struct {
	Path        []string     `json:"path"`
	Summary     string       `json:"summary"`
	Auth        string       `json:"auth"`
	Safety      string       `json:"safety"`
	Options     []SpecOption `json:"options,omitempty"`
	Output      string       `json:"output"`
	Description string       `json:"description,omitempty"`
}

var commonLocOpts = []SpecOption{
	{Name: "lat", Type: "number", Default: "52.3676", Description: "Latitude (defaults to Amsterdam)"},
	{Name: "lon", Type: "number", Default: "4.9041", Description: "Longitude (defaults to Amsterdam)"},
}

var CommandSpecs = []CommandSpec{
	{
		Path:    []string{"agent-skill"},
		Summary: "Print the buienradarcli agent skill",
		Auth:    "none", Safety: "read",
		Output: "Skill markdown content. Pipe to a SKILL.md file.",
	},
	{
		Path:    []string{"describe"},
		Summary: "Describe a command path",
		Auth:    "none", Safety: "read",
		Output: "Command spec (path, options, output description).",
	},
	{
		Path:    []string{"current"},
		Summary: "Current weather from the station nearest to (lat, lon)",
		Auth:    "none", Safety: "read",
		Options: commonLocOpts,
		Output:  "Station measurement with temperature, humidity, wind, condition.",
		Description: "Buienradar's KNMI-fed station network covers the Netherlands. " +
			"Coordinates outside NL will still match the nearest available station.",
	},
	{
		Path:    []string{"forecast"},
		Summary: "Five-day forecast (national, not location-specific)",
		Auth:    "none", Safety: "read",
		Options: []SpecOption{
			{Name: "days", Type: "number", Default: "5", Description: "Days to return (1-5)"},
		},
		Output: "Per-day min/max temps, rain chance, sun chance, wind, condition.",
	},
	{
		Path:    []string{"rain"},
		Summary: "Precipitation forecast (~48h, highest resolution near-term)",
		Auth:    "none", Safety: "read",
		Options: commonLocOpts,
		Output:  "total_mm, will_rain, first_rain_time, partial_errors[], entries[{time, mm_per_h}]. " +
			"Entries span ~48h: 5-min resolution near-term tapering to hourly further out.",
	},
	{
		Path:    []string{"stations"},
		Summary: "List all KNMI weather stations with current measurements",
		Auth:    "none", Safety: "read",
		Options: []SpecOption{
			{Name: "filter", Type: "string", Description: "Substring match on station name (case-insensitive)"},
		},
		Output: "Station id, name, lat, lon, temperature, condition.",
	},
	{
		Path:    []string{"report"},
		Summary: "Free-form weather report (Dutch text from KNMI meteorologists)",
		Auth:    "none", Safety: "read",
		Output: "Title, summary, short-term and long-term outlook strings, in Dutch.",
	},
}

func FindSpec(path []string) *CommandSpec {
	for i := range CommandSpecs {
		s := &CommandSpecs[i]
		if len(s.Path) != len(path) {
			continue
		}
		match := true
		for j := range path {
			if s.Path[j] != path[j] {
				match = false
				break
			}
		}
		if match {
			return s
		}
	}
	return nil
}

func TopLevelSpecs() []CommandSpec {
	out := make([]CommandSpec, 0, len(CommandSpecs))
	for _, s := range CommandSpecs {
		if len(s.Path) == 1 {
			out = append(out, s)
		}
	}
	return out
}
