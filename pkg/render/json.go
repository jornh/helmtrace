package render

import (
	"encoding/json"
	"fmt"
	"io"

	"helmtrace/pkg/analyzer"
)

// jsonSource is the JSON representation of a single layer's contribution.
type jsonSource struct {
	Layer       string      `json:"layer"`
	Value       interface{} `json:"value"`
	Null        bool        `json:"null,omitempty"`
	ResourceKey string      `json:"resourceKey,omitempty"`
	Redundant   bool        `json:"redundant,omitempty"`
}

// jsonNode is the JSON representation of a single key's provenance.
type jsonNode struct {
	Key            string       `json:"key"`
	EffectiveValue interface{}  `json:"effectiveValue"`
	Sources        []jsonSource `json:"sources"`
}

// jsonOutput is the top-level structure: either flat (no resource keys) or
// grouped by resource key when any source carries one.
type jsonOutput struct {
	// Flat is populated when no ResourceKey is present on any source.
	Flat []jsonNode `json:"keys,omitempty"`
	// ByResource is populated when at least one source has a ResourceKey.
	ByResource map[string][]jsonNode `json:"resources,omitempty"`
}

// JSON writes a JSON representation of the provenance data to w.
// When nodes carry ResourceKey values (kustomize mode), output is grouped
// under a "resources" map keyed by "Kind/name". Otherwise a flat "keys"
// array is emitted.
func JSON(w io.Writer, nodes []analyzer.ValueNode, layers []analyzer.Layer) error {
	groups := BuildGroups(nodes, layers)

	out := jsonOutput{}

	if len(groups) == 1 && groups[0].ResourceKey == "" {
		// Flat / Helm mode.
		out.Flat = make([]jsonNode, 0, len(groups[0].Nodes))
		for _, n := range groups[0].Nodes {
			out.Flat = append(out.Flat, toJSONNode(n, groups[0].Layers))
		}
	} else {
		// Kustomize mode — one entry per resource group.
		out.ByResource = make(map[string][]jsonNode, len(groups))
		for _, g := range groups {
			rk := g.ResourceKey
			jNodes := make([]jsonNode, 0, len(g.Nodes))
			for _, n := range g.Nodes {
				jNodes = append(jNodes, toJSONNode(n, g.Layers))
			}
			out.ByResource[rk] = jNodes
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

// toJSONNode converts a ValueNode to its JSON representation, annotating
// each source with its redundancy status.
func toJSONNode(n analyzer.ValueNode, layers []analyzer.Layer) jsonNode {
	// Build layer name → index map for IsRedundant.
	layerIdx := map[string]int{}
	for i, l := range layers {
		layerIdx[l.Name] = i
	}

	sources := make([]jsonSource, len(n.Sources))
	for i, s := range n.Sources {
		idx, ok := layerIdx[s.Layer]
		sources[i] = jsonSource{
			Layer:       s.Layer,
			Value:       s.Value,
			Null:        s.Null,
			ResourceKey: s.ResourceKey,
			Redundant:   ok && n.IsRedundant(idx),
		}
	}
	return jsonNode{
		Key:            n.Key,
		EffectiveValue: n.EffectiveValue,
		Sources:        sources,
	}
}
