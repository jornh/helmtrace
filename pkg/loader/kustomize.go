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
			// Resource is a plain values file — may be multi-doc.
			fileLayers, err := loadYAMLLayers(resPath)
			if err != nil {
				return nil, err
			}
			layers = append(layers, fileLayers...)
		}
	}

	// Strategic merge patches from patchesStrategicMerge (deprecated field).
	for _, p := range kfile.PatchesStrategicMerge {
		patchPath := filepath.Join(dir, p)
		fileLayers, err := loadYAMLLayers(patchPath)
		if err != nil {
			return nil, err
		}
		layers = append(layers, fileLayers...)
	}

	// Strategic merge patches from the unified patches field (v4+).
	for _, p := range kfile.Patches {
		if p.Path == "" {
			// Inline patches (no path) are not yet supported.
			continue
		}
		patchPath := filepath.Join(dir, p.Path)
		fileLayers, err := loadYAMLLayers(patchPath)
		if err != nil {
			return nil, err
		}
		layers = append(layers, fileLayers...)
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

// loadYAMLLayers reads a YAML file and returns one Layer per document.
// Each document has its Kubernetes envelope stripped and its ResourceKey set.
// Single-document files return a slice of length 1.
func loadYAMLLayers(path string) ([]analyzer.Layer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	docs, err := decodeAllYAML(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	nodes, err := decodeAllYAMLNodes(data)
	if err != nil {
		return nil, fmt.Errorf("parsing AST %s: %w", path, err)
	}

	name := LayerName(path)
	var layers []analyzer.Layer
	for i, doc := range docs {
		if doc == nil {
			continue
		}
		stripped := stripEnvelope(doc)
		if len(stripped) == 0 {
			continue
		}
		rk := resourceKey(doc)
		// For multi-doc files suffix the layer name with the resource key so
		// each document gets a distinct, readable column header.
		layerName := name
		if len(docs) > 1 && rk != "" {
			layerName = name + ":" + rk
		}
		var node *yaml.Node
		if i < len(nodes) {
			node = nodes[i]
		}
		layers = append(layers, analyzer.Layer{
			Name:        layerName,
			FilePath:    path,
			Values:      stripped,
			Node:        node,
			ResourceKey: rk,
		})
	}
	if len(layers) == 0 {
		layers = []analyzer.Layer{{Name: name, FilePath: path, Values: map[string]interface{}{}}}
	}
	return layers, nil
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

// decodeAllYAMLNodes decodes all YAML documents from data as *yaml.Node trees,
// preserving line/column information for source location tracking.
func decodeAllYAMLNodes(data []byte) ([]*yaml.Node, error) {
	var nodes []*yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		nodes = append(nodes, &node)
	}
	return nodes, nil
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
