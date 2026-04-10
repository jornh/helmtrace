package lint

import (
	"encoding/json"
	"fmt"
	"io"
)

// PrintText writes violations to w in a human-readable format suitable for
// CI logs. Each line is prefixed with its severity.
//
// Example output:
//
//	warn: "database.port" in layer "prod" is redundant: identical to effective value from lower layers
//	warn: "replicaCount" in layer "staging" is redundant: identical to effective value from lower layers
//	2 warning(s), 0 error(s)
func PrintText(w io.Writer, violations []Violation) {
	for _, v := range violations {
		fmt.Fprintf(w, "%s: %s\n", v.Severity, v.Message)
	}

	if len(violations) == 0 {
		fmt.Fprintln(w, "ok: no lint violations found")
		return
	}

	warns, errs := 0, 0
	for _, v := range violations {
		switch v.Severity {
		case SeverityError:
			errs++
		default:
			warns++
		}
	}
	fmt.Fprintf(w, "\n%d warning(s), %d error(s)\n", warns, errs)
}

// jsonViolation is the JSON representation of a single violation.
type jsonViolation struct {
	Key      string      `json:"key"`
	Layer    string      `json:"layer"`
	Value    interface{} `json:"value"`
	Severity string      `json:"severity"`
	Message  string      `json:"message"`
}

// PrintJSON writes violations to w as a JSON array.
func PrintJSON(w io.Writer, violations []Violation) error {
	out := make([]jsonViolation, len(violations))
	for i, v := range violations {
		out[i] = jsonViolation{
			Key:      v.Key,
			Layer:    v.Layer,
			Value:    v.Value,
			Severity: string(v.Severity),
			Message:  v.Message,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
