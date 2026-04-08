package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// LayerName returns a human-friendly label for a values file.
// Rules:
//   - If the file is named values.yaml or values.yml:
//         use the parent directory name (e.g. base/scg)
//   - Otherwise:
//         use the filename without extension (e.g. uat, scg, my-values)
//   - If the parent directory has multiple components, use the last two.
func LayerName(path string) string {
    p := filepath.Clean(path)

    dir := filepath.Dir(p)
    base := filepath.Base(p)
    stem := strings.TrimSuffix(base, filepath.Ext(base))

    // If it's not a values.yaml file, use the filename as the label.
    if !strings.EqualFold(base, "values.yaml") &&
       !strings.EqualFold(base, "values.yml") {
        return stem
    }

    // Otherwise derive from directory structure.
    if dir == "." || dir == "/" {
        return stem // fallback
    }

    parts := strings.Split(filepath.ToSlash(dir), "/")
    n := len(parts)

    if n >= 2 {
        return parts[n-2] + "/" + parts[n-1]
    }
    return parts[n-1]
}
