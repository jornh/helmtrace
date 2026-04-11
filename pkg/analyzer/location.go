package analyzer

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// nodeAt walks the yaml.Node tree rooted at root to find the value node at
// the given dot-separated path, returning its file location.
// Returns nil when root is nil or the path cannot be resolved in the AST.
func nodeAt(root *yaml.Node, path string, file string) *SourceLocation {
	if root == nil {
		return nil
	}
	// yaml.v3 wraps the document in a Document node; unwrap it.
	n := root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}

	parts := strings.Split(path, ".")
	for _, part := range parts {
		if n == nil {
			return nil
		}
		switch n.Kind {
		case yaml.MappingNode:
			n = mappingChild(n, part)
		case yaml.SequenceNode:
			idx, err := strconv.Atoi(part)
			if err != nil || idx >= len(n.Content) {
				return nil
			}
			n = n.Content[idx]
		default:
			return nil
		}
	}

	if n == nil {
		return nil
	}
	return &SourceLocation{
		File:   file,
		Line:   n.Line,
		Column: n.Column,
	}
}

// mappingChild returns the value node for the given key in a MappingNode.
// MappingNode.Content is a flat [key, value, key, value, ...] slice.
func mappingChild(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
