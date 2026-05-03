package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type OutputMode string

const (
	OutputJSON  OutputMode = "json"
	OutputPlain OutputMode = "plain"
	OutputText  OutputMode = "text"
)

func ParseOutputMode(s string) (OutputMode, error) {
	switch s {
	case "json":
		return OutputJSON, nil
	case "plain":
		return OutputPlain, nil
	case "text":
		return OutputText, nil
	case "":
		return "", nil
	}
	return "", fmt.Errorf("output must be one of: json, plain, text")
}

// Resolve picks an output mode. An explicit request wins; otherwise non-TTY
// stdout defaults to JSON, TTY defaults to text.
func Resolve(requested OutputMode, stdoutIsTTY bool) OutputMode {
	if requested != "" {
		return requested
	}
	if stdoutIsTTY {
		return OutputText
	}
	return OutputJSON
}

type Envelope struct {
	OK      bool        `json:"ok"`
	Command string      `json:"command"`
	Data    interface{} `json:"data"`
}

type ErrorEnvelope struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Error   string `json:"error"`
}

func WriteJSON(w io.Writer, command string, data interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Envelope{OK: true, Command: command, Data: data})
}

func WriteJSONError(w io.Writer, command string, err error) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(ErrorEnvelope{OK: false, Command: command, Error: err.Error()})
}

// WritePlain prints rows of "key=value" pairs joined by tabs, one entry per
// line. Designed to be greppable from shell scripts.
func WritePlain(w io.Writer, rows []map[string]string, order []string) {
	for _, row := range rows {
		fields := make([]string, 0, len(order))
		for _, k := range order {
			if v, ok := row[k]; ok {
				fields = append(fields, fmt.Sprintf("%s=%s", k, plainEscape(v)))
			}
		}
		fmt.Fprintln(w, strings.Join(fields, "\t"))
	}
}

func plainEscape(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
