package cli

const AgentSkillContent = `---
name: buienradarcli
description: Use when fetching weather for the Netherlands or Belgium. It uses the buienradarcli command line tool. Always use --output json, prefer specific subcommands over the raw feed, and pass explicit --lat/--lon when the user has a known location.
---

# buienradarcli

Use this skill when the task involves fetching weather data using the
` + "`buienradarcli`" + ` command line tool.

Run with ` + "`go run github.com/benoitdion/buienradarcli@main`" + ` unless otherwise specified.

## Operating mode

- Always use ` + "`--output json`" + `. Do not parse human-readable output.
- All commands are read-only
- Coverage is the Netherlands and Belgium but may work in other locales.
- Strings in the feed (descriptions, weather report) are in Dutch. Translate
  into the user's language before presenting unless they asked to keep Dutch.

## Discovery

` + "```bash" + `
buienradarcli --list
buienradarcli describe rain --output json
buienradarcli describe forecast --output json
` + "```" + `

## Commands

- Live conditions plus hourly, rain, pollen, and 14-day forecasts:
  ` + "`buienradarcli forecast --lat 52.37 --lon 4.90 --output json`" + `
- Precipitation forecast:
  ` + "`buienradarcli rain --lat 52.37 --lon 4.90 --output json`" + `
- All weather stations with measurements:
  ` + "`buienradarcli stations --output json`" + `
## Coordinates

- ` + "`--lat`" + ` / ` + "`--lon`" + ` default to Amsterdam (52.3676, 4.9041) when omitted.
  Pass explicit coordinates when you have them — even Dutch postal codes can
  be cheaply geocoded by the caller before invoking this CLI.

## Conditions

Weather is described without provider codes, as a ` + "`conditions`" + ` object:

- ` + "`clouds`: `clear`, `mostly-sunny`, `partly-cloudy`, `mostly-cloudy`, `cloudy`" + `
- ` + "`precipitation`: `none`, `light-rain`, `rain`, `thunderstorm`, `light-snow`, `snow`, `sleet`" + `
- ` + "`fog`, `night`: booleans" + `
`
