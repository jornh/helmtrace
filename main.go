package main

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"helmtrace/pkg/analyzer"
	"helmtrace/pkg/render"
)

func main() {
	var files layerFlags
	var allRows bool
	flag.Var(&files, "f", "values file, may be repeated; order defines precedence (lowest first)")
	flag.BoolVar(&allRows, "all-rows", false, "show all keys, including those that appear in only one layer")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `helmtrace - show provenance of values across layered Helm values files

Usage:
  helmtrace -f base.yaml -f env/prod.yaml [-f override.yaml] [--all-rows]

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if len(files) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	layers, err := loadLayers(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	opts := analyzer.Options{MultiLayerOnly: !allRows}
	nodes := analyzer.Analyze(layers, opts)
	render.Table(os.Stdout, nodes, layers)
}

// loadLayers reads each file and returns a slice of Layers in the same order.
func loadLayers(files []string) ([]analyzer.Layer, error) {
	layers := make([]analyzer.Layer, 0, len(files))
	for _, path := range files {
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
			Name:   layerName(path),
			Values: values,
		})
	}
	return layers, nil
}

// layerName derives a display name from a file path by stripping directory
// and extension, e.g. "env/prod.yaml" → "prod".
func layerName(path string) string {
	name := path
	// Strip leading directories.
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			name = path[i+1:]
			break
		}
	}
	// Strip .yaml / .yml extension.
	for _, ext := range []string{".yaml", ".yml"} {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			name = name[:len(name)-len(ext)]
			break
		}
	}
	return name
}

// layerFlags is a flag.Value that accumulates repeated -f arguments.
type layerFlags []string

func (f *layerFlags) String() string { return fmt.Sprint([]string(*f)) }
func (f *layerFlags) Set(v string) error {
	*f = append(*f, v)
	return nil
}
