// Package testutil provides helpers for constructing analyzer types in tests.
// It is intended for use in test files only.
package testutil

import (
	"gopkg.in/yaml.v3"

	"helmtrace/pkg/analyzer"
)

// LayerFromYAML constructs a Layer from a raw YAML string, populating both
// Values and Node so that location tracking can be exercised in tests.
// Panics on parse error.
func LayerFromYAML(name, filePath, src string) analyzer.Layer {
	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(src), &values); err != nil {
		panic("LayerFromYAML: " + err.Error())
	}
	if values == nil {
		values = map[string]interface{}{}
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(src), &node); err != nil {
		panic("LayerFromYAML AST: " + err.Error())
	}
	return analyzer.Layer{
		Name:     name,
		FilePath: filePath,
		Values:   values,
		Node:     &node,
	}
}
