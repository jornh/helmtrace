package render

import "helmtrace/pkg/analyzer"

// Group is a named set of nodes that share a ResourceKey, together with only
// the layers that contribute to those nodes. In Helm mode there is always
// exactly one Group with an empty ResourceKey.
type Group struct {
	ResourceKey string                 // "Deployment/myapp" or "" for Helm
	Nodes       []analyzer.ValueNode
	Layers      []analyzer.Layer // only layers touching this group's nodes
}

// BuildGroups partitions nodes into groups by ResourceKey.
// When no source carries a ResourceKey (Helm mode), a single flat group is
// returned. Order matches first-seen ResourceKey across nodes.
func BuildGroups(nodes []analyzer.ValueNode, allLayers []analyzer.Layer) []Group {
	// Fast path: no resource keys → single flat group with all layers.
	if !hasResourceKeys(nodes) {
		return []Group{{Nodes: nodes, Layers: allLayers}}
	}

	// Build an index of all layers by name for O(1) lookup.
	layerByName := make(map[string]analyzer.Layer, len(allLayers))
	for _, l := range allLayers {
		layerByName[l.Name] = l
	}

	type groupAccum struct {
		nodes      []analyzer.ValueNode
		layerNames []string         // ordered, first-seen
		layerSeen  map[string]bool
	}

	order := []string{} // insertion-ordered resource keys
	accums := map[string]*groupAccum{}

	for _, n := range nodes {
		// Determine which resource key(s) this node belongs to.
		nodeRKs := nodeResourceKeys(n)

		for _, rk := range nodeRKs {
			a, exists := accums[rk]
			if !exists {
				a = &groupAccum{layerSeen: map[string]bool{}}
				accums[rk] = a
				order = append(order, rk)
			}
			a.nodes = append(a.nodes, n)

			// Record layer names that contribute to this group, in source order.
			for _, s := range n.Sources {
				if s.ResourceKey == rk || (s.ResourceKey == "" && rk == "_") {
					if !a.layerSeen[s.Layer] {
						a.layerSeen[s.Layer] = true
						a.layerNames = append(a.layerNames, s.Layer)
					}
				}
			}
		}
	}

	groups := make([]Group, 0, len(order))
	for _, rk := range order {
		a := accums[rk]
		layers := make([]analyzer.Layer, 0, len(a.layerNames))
		for _, name := range a.layerNames {
			if l, ok := layerByName[name]; ok {
				layers = append(layers, l)
			}
		}
		groups = append(groups, Group{
			ResourceKey: rk,
			Nodes:       a.nodes,
			Layers:      layers,
		})
	}
	return groups
}

// hasResourceKeys returns true if any source in any node carries a ResourceKey.
func hasResourceKeys(nodes []analyzer.ValueNode) bool {
	for _, n := range nodes {
		for _, s := range n.Sources {
			if s.ResourceKey != "" {
				return true
			}
		}
	}
	return false
}

// nodeResourceKeys returns the distinct ResourceKeys referenced by a node's
// sources. Empty resource keys are mapped to "_" (ungrouped sentinel).
func nodeResourceKeys(n analyzer.ValueNode) []string {
	seen := map[string]bool{}
	var keys []string
	for _, s := range n.Sources {
		rk := s.ResourceKey
		if rk == "" {
			rk = "_"
		}
		if !seen[rk] {
			seen[rk] = true
			keys = append(keys, rk)
		}
	}
	return keys
}
