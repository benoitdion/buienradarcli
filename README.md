# buienradarcli

Agent-friendly CLI for Buienradar.

## Install

```bash
go install github.com/benoitdion/buienradarcli@latest
```

Or build from source:

```bash
go build -o buienradarcli .
```

## Library

```go
client := buienradar.NewClient()
forecast, err := client.MergedRainForecast(ctx, 52.3676, 4.9041)
```

Import `github.com/benoitdion/buienradarcli/buienradar`. Use
`NewClientWithHTTP` when the caller owns tracing, retries, or transport policy.

## Quickstart

```bash
buienradarcli forecast --lat 52.37 --lon 4.90
buienradarcli rain     --lat 52.37 --lon 4.90
buienradarcli stations --filter Schiphol
```

## Discovery

```bash
buienradarcli --list
buienradarcli describe rain --output json
buienradarcli describe forecast --output json
```

`--list` shows top-level commands. `describe <path>` returns the full spec
(options, defaults, output description) so an agent can plan a call without
guessing.

## Output modes

- `--output text` — human-readable terminal output (default in TTY)
- `--output plain` — tab-separated `key=value` rows (parse-friendly for shell)
- `--output json` — `{ok, command, data}` JSON envelope (default off-TTY)

When stdout is not a TTY, the CLI defaults to JSON.

## Commands

| Command       | What it returns                                                   |
| ------------- | ----------------------------------------------------------------- |
| `forecast`    | Merged live, hourly, rain, pollen, and 14-day forecast.            |
| `rain`        | Precipitation forecast, with the highest resolution near-term.     |
| `stations`    | All KNMI stations with their latest measurements.                  |
| `describe`    | Spec for a command path.                                           |
| `agent-skill` | Prints the agent SKILL.md for installation under `~/.agents`.       |

`--lat` and `--lon` default to Amsterdam (52.3676, 4.9041) when omitted.
The graph-data API key is discovered automatically and cached in the user cache
directory. Set `BUIENRADAR_API_KEY` or pass `--api-key` to override it.

## Conditions

Icon codes are normalized to stable English labels:
`clear`, `partly-cloudy`, `cloudy`, `fog`, `light-rain`, `heavy-rain`,
`rain-showers`, `thunderstorm`, `light-snow`, `heavy-snow`, `snow-showers`,
`rain-and-snow`, `partly-cloudy-rain`. The original Dutch description and icon
code are preserved alongside.

## For agents

Run the following to install a skill describing how to drive this CLI:

```bash
mkdir -p ~/.agents/skills/buienradarcli
buienradarcli agent-skill > ~/.agents/skills/buienradarcli/SKILL.md
```

Operating recommendations baked into the skill:

- Always pass `--output json` and parse the envelope, never the text output.
- Use `--list` and `describe <path>` for discovery before guessing arguments.
- Translate Dutch source text into the user's language unless
  asked otherwise.

## Live smoke test

Run the opt-in end-to-end smoke test whenever you want to confirm that the
Buienradar integration still works:

```bash
./scripts/smoke-test.sh
```

This uses the live Buienradar services to verify API-key extraction and the
rain and forecast responses.
