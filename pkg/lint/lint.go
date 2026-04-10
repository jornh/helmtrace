package lint

import (
	"fmt"

	"helmtrace/pkg/analyzer"
)

// Severity classifies how serious a violation is.
type Severity string

const (
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Violation describes a single lint finding.
type Violation struct {
	Key      string   // dot-separated path, e.g. "database.port"
	Layer    string   // layer name where the redundancy occurs
	Value    interface{}
	Severity Severity
	Message  string
}

// Options controls which checks are run.
type Options struct {
	// FailOnRedundant treats redundant values as errors rather than warnings.
	// When false, redundancies are warnings (exit 0); when true, exit 1.
	FailOnRedundant bool
}

// Run analyses the provided nodes and layers and returns all violations found.
// Currently checks for redundant values — values in a higher layer that are
// identical to the effective value from lower layers and could be removed.
func Run(nodes []analyzer.ValueNode, layers []analyzer.Layer, opts Options) []Violation {
	var violations []Violation

	// Build layer name → index for IsRedundant calls.
	layerIdx := map[string]int{}
	for i, l := range layers {
		layerIdx[l.Name] = i
	}

	for _, n := range nodes {
		for _, s := range n.Sources {
			idx, ok := layerIdx[s.Layer]
			if !ok {
				continue
			}
			if !n.IsRedundant(idx) {
				continue
			}
			sev := SeverityWarn
			if opts.FailOnRedundant {
				sev = SeverityError
			}
			violations = append(violations, Violation{
				Key:      n.Key,
				Layer:    s.Layer,
				Value:    s.Value,
				Severity: sev,
				Message: fmt.Sprintf(
					"%q in layer %q is redundant: identical to effective value from lower layers",
					n.Key, s.Layer,
				),
			})
		}
	}

	return violations
}

// HasErrors returns true if any violation has error severity.
func HasErrors(violations []Violation) bool {
	for _, v := range violations {
		if v.Severity == SeverityError {
			return true
		}
	}
	return false
}
