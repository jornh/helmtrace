package analyzer

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Layer represents a single values file with a name and its parsed content.
// ResourceKey is optionally set by loaders that understand Kubernetes structure
// (e.g. KustomizeLoader) to identify which Kind/name resource this layer
// belongs to. HelmLoader leaves it empty.
type Layer struct {
	Name        string
	FilePath    string                 // absolute or relative path to the source file
	Values      map[string]interface{} // decoded values for analysis
	Node        *yaml.Node             // raw AST for line/column resolution; may be nil
	ResourceKey string                 // e.g. "Deployment/myapp", empty for Helm layers
}

// SourceLocation records where in a source file a value is defined.
type SourceLocation struct {
	File   string
	Line   int
	Column int
}

// String returns a compact "file:line:col" representation.
func (l SourceLocation) String() string {
	if l.File == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column)
}

// Source records a single layer's contribution to a key.
type Source struct {
	Layer    string
	Value    interface{}
	Location *SourceLocation // nil when the layer has no Node (e.g. inline test fixtures)
	// Null is true when the layer explicitly sets this key to null, which
	// suppresses any value from lower-precedence layers.
	Null bool
	// ResourceKey mirrors Layer.ResourceKey, carried through so renderers
	// can group or filter by resource without re-joining to the layer list.
	ResourceKey string
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
	// Null overrides are never redundant — they actively suppress lower layers.
	if n.Sources[idx].Null {
		return false
	}
	// Find the effective value contributed by layers below idx.
	effectiveBelow := n.Sources[idx-1].Value
	for i := idx - 2; i >= 0; i-- {
		if n.Sources[i].Value != nil {
			effectiveBelow = n.Sources[i].Value
		}
	}
	return reflect.DeepEqual(n.Sources[idx].Value, effectiveBelow)
}

// Options controls what Analyze returns.
type Options struct {
	// MultiLayerOnly omits keys that appear in only one layer, keeping the
	// output focused on values that are actually overridden or duplicated
	// across layers. Base-only keys are hidden unless something else touches
	// them.
	MultiLayerOnly bool
}

// IsMultiLayer reports whether the key is defined in more than one layer.
// Used to filter base-only noise from the rendered table.
func (n *ValueNode) IsMultiLayer() bool {
	return len(n.Sources) > 1
}

// Analyze merges layers in order (last layer wins) and returns one ValueNode
// per leaf key found across all layers, filtered by opts.
func Analyze(layers []Layer, opts Options) []ValueNode {
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
		if opts.MultiLayerOnly && !node.IsMultiLayer() {
			continue
		}
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
		val, found, null := getPath(l.Values, path)
		if !found {
			continue
		}
		var loc *SourceLocation
		if l.Node != nil {
			loc = nodeAt(l.Node, path, l.FilePath)
		}
		node.Sources = append(node.Sources, Source{
			Layer:       l.Name,
			Value:       val,
			Null:        null,
			ResourceKey: l.ResourceKey,
			Location:    loc,
		})
		// A null override suppresses lower layers: effective value becomes nil.
		node.EffectiveValue = val
	}

	return node
}

// leafPaths returns all dot-separated paths to scalar (non-map, non-slice)
// values within a nested map. Slices are walked by index (foo.0.bar),
// and slices of scalars surface as foo.0, foo.1, etc.
func leafPaths(m map[string]interface{}, prefix string) []string {
	var paths []string
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		paths = append(paths, walkValue(v, full)...)
	}
	return paths
}

// walkValue recurses into maps and slices, returning leaf paths.
func walkValue(v interface{}, prefix string) []string {
	switch val := v.(type) {
	case map[string]interface{}:
		var paths []string
		for k, child := range val {
			paths = append(paths, walkValue(child, prefix+"."+k)...)
		}
		return paths
	case []interface{}:
		var paths []string
		for i, elem := range val {
			paths = append(paths, walkValue(elem, fmt.Sprintf("%s.%d", prefix, i))...)
		}
		return paths
	default:
		// nil (explicit YAML null) and scalars are both leaves.
		return []string{prefix}
	}
}

// getPath retrieves the value at a dot-separated path from a nested
// map/slice structure. Numeric segments index into slices (e.g. "foo.0.bar").
// Returns (value, found, isNull):
//   - found=false means the key does not exist in this layer
//   - found=true, isNull=true means the key is explicitly set to null
//   - found=true, isNull=false means the key has a non-null value
func getPath(m map[string]interface{}, path string) (interface{}, bool, bool) {
	parts := strings.SplitN(path, ".", 2)
	v, ok := m[parts[0]]
	if !ok {
		return nil, false, false
	}
	if len(parts) == 1 {
		return v, true, v == nil
	}
	if v == nil {
		// Explicit null at an intermediate key — path cannot continue.
		return nil, false, false
	}
	val, found := getPathValue(v, parts[1])
	return val, found, found && val == nil
}

// getPathValue continues path traversal into any value type.
func getPathValue(v interface{}, path string) (interface{}, bool) {
	parts := strings.SplitN(path, ".", 2)
	switch val := v.(type) {
	case map[string]interface{}:
		child, ok := val[parts[0]]
		if !ok {
			return nil, false
		}
		if len(parts) == 1 {
			return child, true
		}
		if child == nil {
			return nil, false
		}
		return getPathValue(child, parts[1])
	case []interface{}:
		idx := 0
		for _, c := range parts[0] {
			if c < '0' || c > '9' {
				return nil, false
			}
			idx = idx*10 + int(c-'0')
		}
		if idx >= len(val) {
			return nil, false
		}
		if len(parts) == 1 {
			return val[idx], true
		}
		if val[idx] == nil {
			return nil, false
		}
		return getPathValue(val[idx], parts[1])
	default:
		return nil, false
	}
}

// deepEqual compares two values for structural equality.
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
