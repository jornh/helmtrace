package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"helmtrace/pkg/analyzer"
)

// KustomizeLoader loads layers by walking a kustomization.yaml tree.
// It handles strategic merge patches only; JSON patches are not yet supported.
// Layers are returned in application order: base resource first, then patches
// in the order they appear in kustomization.yaml, lowest precedence first.
type KustomizeLoader struct {
	// Root is the directory containing the top-level kustomization.yaml.
	Root string
}

// Load walks the kustomization tree rooted at k.Root and returns one Layer
// per resolved values source (base resource and each strategic merge patch).
func (k *KustomizeLoader) Load() ([]analyzer.Layer, error) {
	return loadKustomizeDir(k.Root)
}

// kustomizationFile is the subset of kustomization.yaml we care about.
type kustomizationFile struct {
	Resources            []string `yaml:"resources"`
	PatchesStrategicMerge []string `yaml:"patchesStrategicMerge"`
	// Kustomize v4+ unified patches field
	Patches []kustomizePatch `yaml:"patches"`
}

type kustomizePatch struct {
	Path string `yaml:"path"`
}

// loadKustomizeDir resolves layers from a single kustomization directory.
// It recurses into base directories listed under resources.
func loadKustomizeDir(dir string) ([]analyzer.Layer, error) {
	kfile, err := readKustomizationFile(dir)
	if err != nil {
		return nil, err
	}

	var layers []analyzer.Layer

	// Recurse into resource bases first (lowest precedence).
	for _, res := range kfile.Resources {
		resPath := filepath.Join(dir, res)
		info, err := os.Stat(resPath)
		if err != nil {
			return nil, fmt.Errorf("stat resource %s: %w", resPath, err)
		}
		if info.IsDir() {
			// Resource is a base directory — recurse.
			baseLayers, err := loadKustomizeDir(resPath)
			if err != nil {
				return nil, fmt.Errorf("loading base %s: %w", resPath, err)
			}
			layers = append(layers, baseLayers...)
		} else {
			// Resource is a plain values file.
			layer, err := loadYAMLLayer(resPath)
			if err != nil {
				return nil, err
			}
			layers = append(layers, layer)
		}
	}

	// Strategic merge patches from patchesStrategicMerge (deprecated field).
	for _, p := range kfile.PatchesStrategicMerge {
		patchPath := filepath.Join(dir, p)
		layer, err := loadYAMLLayer(patchPath)
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}

	// Strategic merge patches from the unified patches field (v4+).
	for _, p := range kfile.Patches {
		if p.Path == "" {
			// Inline patches (no path) are not yet supported.
			continue
		}
		patchPath := filepath.Join(dir, p.Path)
		layer, err := loadYAMLLayer(patchPath)
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}

	return layers, nil
}

// readKustomizationFile reads and parses kustomization.yaml (or kustomization.yml)
// from dir, returning an error if neither exists.
func readKustomizationFile(dir string) (*kustomizationFile, error) {
	for _, name := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var kf kustomizationFile
		if err := yaml.Unmarshal(data, &kf); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		return &kf, nil
	}
	return nil, fmt.Errorf("no kustomization.yaml found in %s", dir)
}

// loadYAMLLayer reads a YAML file (single or multi-document) and returns it
// as a Layer. Multiple documents are merged into a single values map keyed by
// "kind/name" to avoid collisions, then the Kubernetes envelope is stripped so
// the analyzer only sees spec/data/stringData fields.
func loadYAMLLayer(path string) (analyzer.Layer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return analyzer.Layer{}, fmt.Errorf("reading %s: %w", path, err)
	}

	docs, err := decodeAllYAML(data)
	if err != nil {
		return analyzer.Layer{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	merged := map[string]interface{}{}
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		stripped := stripEnvelope(doc)
		if len(stripped) == 0 {
			continue
		}
		// Namespace multi-doc values under "kind/name" to avoid key collisions
		// when a file contains e.g. a Deployment and an Ingress.
		key := resourceKey(doc)
		if key == "" || len(docs) == 1 {
			// Single doc — merge directly at the top level.
			for k, v := range stripped {
				merged[k] = v
			}
		} else {
			merged[key] = stripped
		}
	}

	return analyzer.Layer{
		Name:   LayerName(path),
		Values: merged,
	}, nil
}

// decodeAllYAML decodes all YAML documents from data, returning one map per
// document. The standard yaml.Unmarshal only reads the first document.
func decodeAllYAML(data []byte) ([]map[string]interface{}, error) {
	var docs []map[string]interface{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var doc map[string]interface{}
		err := dec.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// stripEnvelope returns only the fields relevant for tracing — spec, data,
// and stringData — dropping apiVersion, kind, metadata, and status.
// This keeps pkg/analyzer unaware of Kubernetes structure.
func stripEnvelope(doc map[string]interface{}) map[string]interface{} {
	keep := []string{"spec", "data", "stringData"}
	out := make(map[string]interface{}, len(keep))
	for _, k := range keep {
		if v, ok := doc[k]; ok {
			out[k] = v
		}
	}
	return out
}

// resourceKey returns a "Kind/name" identifier for a Kubernetes manifest, used
// to namespace multiple documents within a single file.
func resourceKey(doc map[string]interface{}) string {
	kind, _ := doc["kind"].(string)
	meta, _ := doc["metadata"].(map[string]interface{})
	name, _ := meta["name"].(string)
	if kind == "" || name == "" {
		return ""
	}
	return kind + "/" + name
}
