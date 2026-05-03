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

## Quickstart

```bash
buienradarcli current --lat 52.37 --lon 4.90
buienradarcli rain    --lat 52.37 --lon 4.90
buienradarcli forecast
buienradarcli stations --filter Schiphol
buienradarcli report
```

## Discovery

```bash
buienradarcli --list
buienradarcli describe rain --output json
buienradarcli describe current --output json
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

| Command       | What it returns                                                     |
| ------------- | ------------------------------------------------------------------- |
| `current`     | Current measurements at the station nearest to `--lat`/`--lon`.     |
| `forecast`    | Five-day national forecast (min/max temp, rain%, sun%, wind, etc.). |
| `rain`        | Current rain forecast                                               |
| `stations`    | All KNMI stations with their latest measurements.                   |
| `report`      | Free-form Dutch weather report from KNMI meteorologists.            |
| `describe`    | Spec for a command path.                                            |
| `agent-skill` | Prints the agent SKILL.md for installation under `~/.agents`.       |

`--lat` and `--lon` default to Amsterdam (52.3676, 4.9041) when omitted.


## Conditions

Icon codes are normalized to stable English labels:
`clear`, `partly-cloudy`, `cloudy`, `fog`, `light-rain`, `heavy-rain`,
`rain-showers`, `thunderstorm`, `light-snow`, `heavy-snow`, `snow-showers`,
`rain-and-snow`, `partly-cloudy-rain`. The original Dutch description and icon
code are preserved alongside.

## For agents

Run `buienradarcli agent-skill > ~/.agents/skills/buienradarcli/SKILL.md` to
install a skill describing how to drive this CLI.

Operating recommendations baked into the skill:

- Always pass `--output json` and parse the envelope, never the text output.
- Use `--list` and `describe <path>` for discovery before guessing arguments.
- Translate Dutch descriptions/report text into the user's language unless
  asked otherwise.