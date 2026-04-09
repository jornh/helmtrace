package render

import (
	"fmt"
	"io"
	"strings"

	"helmtrace/pkg/analyzer"
)

// Table writes a human-readable provenance table to w.
// When nodes carry ResourceKey values (kustomize mode), output is split into
// one labelled section per resource. Otherwise a single flat table is written.
//
// Example output (kustomize mode):
//
//	Deployment/myapp
//	────────────────────────────────────────────────────
//	KEY                      base        prod      EFFECTIVE
//	spec.replicas            1           3         3
//	spec.template.spec.co…   myapp:1.0   myapp:2…  myapp:2.0
//
//	Ingress/myapp
//	────────────────────────────────────────────────────
//	KEY                      base   prod            EFFECTIVE
//	spec.rules.0.host        —      myapp.prod.ex…  myapp.prod.example.com
//
// A ✕ marks values that are redundant (identical to effective value from lower layers).
func Table(w io.Writer, nodes []analyzer.ValueNode, layers []analyzer.Layer) {
	groups := BuildGroups(nodes, layers)
	hasRedundant := false

	for i, g := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if g.ResourceKey != "" && g.ResourceKey != "_" {
			fmt.Fprintf(w, "%s\n", g.ResourceKey)
		}
		redundant := tableSection(w, g.Nodes, g.Layers)
		if redundant {
			hasRedundant = true
		}
	}

	if hasRedundant {
		fmt.Fprintln(w, "\n✕ = redundant (value is identical to effective value from lower layers)")
	}
}

// tableSection renders one table section and returns true if any redundant
// values were found.
func tableSection(w io.Writer, nodes []analyzer.ValueNode, layers []analyzer.Layer) bool {
	layerNames := make([]string, len(layers))
	for i, l := range layers {
		layerNames[i] = l.Name
	}

	// Column widths.
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
	colWidth += 2

	// Header.
	fmt.Fprintf(w, "%-*s", keyWidth+2, "KEY")
	for _, name := range layerNames {
		fmt.Fprintf(w, "%-*s", colWidth, name)
	}
	fmt.Fprintln(w, "EFFECTIVE")
	fmt.Fprintln(w, strings.Repeat("─", keyWidth+2+colWidth*len(layerNames)+len("EFFECTIVE")))

	// Layer name → index for IsRedundant.
	layerIdx := map[string]int{}
	for i, l := range layers {
		layerIdx[l.Name] = i
	}

	hasRedundant := false
	for _, n := range nodes {
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
				hasRedundant = true
				cell += " ✕"
			}
			fmt.Fprintf(w, "%-*s", colWidth, cell)
		}
		fmt.Fprintln(w, fmt.Sprintf("%v", n.EffectiveValue))
	}

	return hasRedundant
}

