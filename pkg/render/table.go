package render

import (
	"fmt"
	"io"
	"strings"

	"helmtrace/pkg/analyzer"
)

// Table writes a human-readable provenance table to w.
//
// Example output:
//
//	KEY                   base        prod        override    EFFECTIVE
//	database.host         db.internal db.prod     —           db.prod
//	database.port         5432        5432 ✕      —           5432
//	replicaCount          1           3           5           5
//
// A ✕ marks values that are redundant (identical to effective value from lower layers).
func Table(w io.Writer, nodes []analyzer.ValueNode, layers []analyzer.Layer) {
	layerNames := make([]string, len(layers))
	for i, l := range layers {
		layerNames[i] = l.Name
	}

	// Column widths: key column + one per layer + effective.
	keyWidth := len("KEY")
	for _, n := range nodes {
		if len(n.Key) > keyWidth {
			keyWidth = len(n.Key)
		}
	}

	colWidth := len("EFFECTIVE")
	for _, name := range layerNames {
		if len(name) > colWidth {
			colWidth = len(name)
		}
	}
	for _, n := range nodes {
		for _, s := range n.Sources {
			if w := len(fmt.Sprintf("%v", s.Value)); w > colWidth {
				colWidth = w
			}
		}
		if w := len(fmt.Sprintf("%v", n.EffectiveValue)); w > colWidth {
			colWidth = w
		}
	}
	colWidth += 2 // padding

	// Header.
	fmt.Fprintf(w, "%-*s", keyWidth+2, "KEY")
	for _, name := range layerNames {
		fmt.Fprintf(w, "%-*s", colWidth, name)
	}
	fmt.Fprintln(w, "EFFECTIVE")
	fmt.Fprintln(w, strings.Repeat("─", keyWidth+2+colWidth*len(layerNames)+len("EFFECTIVE")))

	// Build a lookup: layer name → index, for IsRedundant calls.
	layerIdx := map[string]int{}
	for i, l := range layers {
		layerIdx[l.Name] = i
	}

	// Rows.
	for _, n := range nodes {
		// Index sources by layer name for O(1) lookup per row.
		sourceByLayer := map[string]analyzer.Source{}
		for _, s := range n.Sources {
			sourceByLayer[s.Layer] = s
		}

		fmt.Fprintf(w, "%-*s", keyWidth+2, n.Key)

		for i, name := range layerNames {
			s, ok := sourceByLayer[name]
			if !ok {
				fmt.Fprintf(w, "%-*s", colWidth, "—")
				continue
			}
			cell := fmt.Sprintf("%v", s.Value)
			if n.IsRedundant(i) {
				cell += " ✕"
			}
			fmt.Fprintf(w, "%-*s", colWidth, cell)
		}

		fmt.Fprintln(w, fmt.Sprintf("%v", n.EffectiveValue))
	}

	// Legend if any redundancies exist.
	hasRedundant := false
	for _, n := range nodes {
		for i := range layers {
			if n.IsRedundant(i) {
				hasRedundant = true
				break
			}
		}
	}
	if hasRedundant {
		fmt.Fprintln(w, "\n✕ = redundant (value is identical to effective value from lower layers)")
	}
}
