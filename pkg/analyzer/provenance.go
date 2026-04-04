package provenance

import (
	"fmt"
 "reflect"
	"strings"
)

// Layer represents a single Helm values file with a name and its parsed content.
type Layer struct {
	Name   string
	Values map[string]interface{}
}

// Source records a single layer's contribution to a key.
type Source struct {
	Layer string
	Value interface{}
}

// ValueNode is the result for a single leaf key: the effective value and
// the ordered list of layers that defined it (earliest = lowest precedence).
type ValueNode struct {
	Key            string   // dot-separated full path, e.g. "database.host"
	EffectiveValue interface{}
	Sources        []Source // index 0 = base layer; last = highest precedence
}

// IsRedundant returns true for layer at index idx if removing it would not
// change the effective value. The base layer (idx == 0) is never redundant.
func (n *ValueNode) IsRedundant(idx int) bool {
	if idx <= 0 || idx >= len(n.Sources) {
		return false
	}
	// Find the effective value contributed by layers below idx.
	effectiveBelow := n.Sources[idx-1].Value
	for i := idx - 2; i >= 0; i-- {
		if n.Sources[i].Value != nil {
			effectiveBelow = n.Sources[i].Value
		}
	}
	return deepEqual(n.Sources[idx].Value, effectiveBelow)
}

// Analyze merges layers in order (last layer wins) and returns one ValueNode
// per leaf key found across all layers.
func Analyze(layers []Layer) []ValueNode {
	// Collect all known leaf paths across all layers.
	pathSet := map[string]struct{}{}
	for _, l := range layers {
		for _, p := range leafPaths(l.Values, "") {
			pathSet[p] = struct{}{}
		}
	}

	nodes := make([]ValueNode, 0, len(pathSet))
	for path := range pathSet {
		node := buildNode(path, layers)
		nodes = append(nodes, node)
	}

	// Sort for stable output.
	sortNodes(nodes)
	return nodes
}

// buildNode constructs the ValueNode for a single dot-separated key path.
func buildNode(path string, layers []Layer) ValueNode {
	node := ValueNode{Key: path}

	for _, l := range layers {
		val, ok := getPath(l.Values, path)
		if !ok {
			continue
		}
		node.Sources = append(node.Sources, Source{
			Layer: l.Name,
			Value: val,
		})
		node.EffectiveValue = val // last write wins
	}

	return node
}

// leafPaths returns all dot-separated paths to scalar (non-map) values
// within a nested map. Slices are treated as leaf values.
func leafPaths(m map[string]interface{}, prefix string) []string {
	var paths []string
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		if child, ok := v.(map[string]interface{}); ok {
			paths = append(paths, leafPaths(child, full)...)
		} else {
			paths = append(paths, full)
		}
	}
	return paths
}

// getPath retrieves the value at a dot-separated path from a nested map.
// Returns (value, true) if found, (nil, false) if not.
func getPath(m map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.SplitN(path, ".", 2)
	v, ok := m[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return v, true
	}
	child, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return getPath(child, parts[1])
}

// deepEqual compares two values for equality, handling the map types that
// come out of YAML unmarshalling.
func deepEqual(a, b interface{}) bool {
    return reflect.DeepEqual(a, b)
}

// sortNodes sorts ValueNodes by key for deterministic output.
func sortNodes(nodes []ValueNode) {
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && nodes[j].Key < nodes[j-1].Key; j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}
}
