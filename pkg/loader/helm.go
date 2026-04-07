package loader

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"helmtrace/pkg/analyzer"
)

// HelmLoader loads an ordered list of Helm values files into layers.
// Files are specified in precedence order: index 0 is lowest (base).
type HelmLoader struct {
	Files []string // paths to values files, lowest precedence first
}

// Load reads each file and returns a Layer per file in the same order.
func (h *HelmLoader) Load() ([]analyzer.Layer, error) {
	layers := make([]analyzer.Layer, 0, len(h.Files))
	for _, path := range h.Files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var values map[string]interface{}
		if err := yaml.Unmarshal(data, &values); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		if values == nil {
			values = map[string]interface{}{}
		}
		layers = append(layers, analyzer.Layer{
			Name:   LayerName(path),
			Values: values,
		})
	}
	return layers, nil
}

// LayerName derives a display name from a file path by stripping the
// directory and extension, e.g. "env/prod.yaml" → "prod".
func LayerName(path string) string {
	name := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			name = path[i+1:]
			break
		}
	}
	for _, ext := range []string{".yaml", ".yml"} {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	return name
}
