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
// When sources carry ResourceKey values (kustomize mode), output is grouped
// under a "resources" map keyed by "Kind/name". Otherwise a flat "keys"
// array is emitted.
func JSON(w io.Writer, nodes []analyzer.ValueNode, layers []analyzer.Layer) error {
	// Detect whether any source carries a resource key.
	hasResourceKeys := false
	for _, n := range nodes {
		for _, s := range n.Sources {
			if s.ResourceKey != "" {
				hasResourceKeys = true
				break
			}
		}
		if hasResourceKeys {
			break
		}
	}

	out := jsonOutput{}

	if !hasResourceKeys {
		out.Flat = flatNodes(nodes, layers)
	} else {
		out.ByResource = groupedNodes(nodes, layers)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

// flatNodes converts nodes to a flat slice with redundancy annotations.
func flatNodes(nodes []analyzer.ValueNode, layers []analyzer.Layer) []jsonNode {
	result := make([]jsonNode, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, toJSONNode(n, layers))
	}
	return result
}

// groupedNodes groups nodes by the ResourceKey of their sources.
// A node may appear under multiple resource keys if its sources span resources.
func groupedNodes(nodes []analyzer.ValueNode, layers []analyzer.Layer) map[string][]jsonNode {
	// Preserve insertion order of resource keys for stable output.
	order := []string{}
	seen := map[string]bool{}
	groups := map[string][]jsonNode{}

	for _, n := range nodes {
		// Collect distinct resource keys referenced by this node's sources.
		nodeKeys := []string{}
		nodeKeySeen := map[string]bool{}
		for _, s := range n.Sources {
			rk := s.ResourceKey
			if rk == "" {
				rk = "_" // ungrouped sources go under a sentinel key
			}
			if !nodeKeySeen[rk] {
				nodeKeys = append(nodeKeys, rk)
				nodeKeySeen[rk] = true
			}
		}

		jn := toJSONNode(n, layers)
		for _, rk := range nodeKeys {
			if !seen[rk] {
				order = append(order, rk)
				seen[rk] = true
			}
			groups[rk] = append(groups[rk], jn)
		}
	}

	// Re-build as ordered map (Go maps have no order, but JSON output is
	// produced in insertion order when we iterate order slice).
	ordered := make(map[string][]jsonNode, len(order))
	for _, rk := range order {
		ordered[rk] = groups[rk]
	}
	return ordered
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
