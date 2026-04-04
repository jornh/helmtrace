package analyzer

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLeafPaths(t *testing.T) {
	m := map[string]interface{}{
		"replicaCount": 1,
		"database": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
		"tags": []interface{}{"a", "b"},
	}
	paths := leafPaths(m, "")
	want := map[string]bool{
		"replicaCount":  true,
		"database.host": true,
		"database.port": true,
		"tags":          true,
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d: %v", len(paths), len(want), paths)
	}
	for _, p := range paths {
		if !want[p] {
			t.Errorf("unexpected path: %q", p)
		}
	}
}

func TestGetPath(t *testing.T) {
	m := map[string]interface{}{
		"database": map[string]interface{}{
			"host": "db.internal",
			"port": 5432,
		},
	}
	v, ok := getPath(m, "database.host")
	if !ok || v != "db.internal" {
		t.Errorf("got (%v, %v), want (db.internal, true)", v, ok)
	}
	_, ok = getPath(m, "database.missing")
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestAnalyze_Provenance(t *testing.T) {
	layers := []Layer{
		{
			Name: "base",
			Values: map[string]interface{}{
				"replicaCount": 1,
				"database": map[string]interface{}{
					"host": "db.internal",
					"port": 5432,
				},
			},
		},
		{
			Name: "prod",
			Values: map[string]interface{}{
				"replicaCount": 3,
				"database": map[string]interface{}{
					"host": "db.prod",
					// port intentionally omitted — base value should stand
				},
			},
		},
		{
			Name: "override",
			Values: map[string]interface{}{
				"replicaCount": 5,
			},
		},
	}

	assertProvenanceNodes(t, Analyze(layers, Options{}))
}

func TestAnalyze_FromFiles(t *testing.T) {
	paths := []string{
		"testdata/base.yaml",
		"testdata/prod.yaml",
		"testdata/override.yaml",
	}
	layers, err := loadTestLayers(t, paths)
	if err != nil {
		t.Fatalf("loading test layers: %v", err)
	}

	assertProvenanceNodes(t, Analyze(layers, Options{}))
}

// assertProvenanceNodes encodes the expected outcomes for the three-layer
// base/prod/override scenario used by both TestAnalyze_Provenance and
// TestAnalyze_FromFiles.
func assertProvenanceNodes(t *testing.T, nodes []ValueNode) {
	t.Helper()

	byKey := map[string]ValueNode{}
	for _, n := range nodes {
		byKey[n.Key] = n
	}

	// replicaCount: set in all three layers, override wins.
	rc, ok := byKey["replicaCount"]
	if !ok {
		t.Fatal("replicaCount: key missing from results")
	}
	if rc.EffectiveValue != 5 {
		t.Errorf("replicaCount effective: got %v, want 5", rc.EffectiveValue)
	}
	if len(rc.Sources) != 3 {
		t.Errorf("replicaCount sources: got %d, want 3", len(rc.Sources))
	}

	// database.host: set in base and prod, prod wins.
	host, ok := byKey["database.host"]
	if !ok {
		t.Fatal("database.host: key missing from results")
	}
	if host.EffectiveValue != "db.prod" {
		t.Errorf("database.host effective: got %v, want db.prod", host.EffectiveValue)
	}
	if len(host.Sources) != 2 {
		t.Errorf("database.host sources: got %d, want 2", len(host.Sources))
	}

	// database.port: only in base, never overridden.
	port, ok := byKey["database.port"]
	if !ok {
		t.Fatal("database.port: key missing from results")
	}
	if port.EffectiveValue != 5432 {
		t.Errorf("database.port effective: got %v, want 5432", port.EffectiveValue)
	}
	if len(port.Sources) != 1 {
		t.Errorf("database.port sources: got %d, want 1", len(port.Sources))
	}
}

// loadTestLayers reads YAML files from disk and returns them as Layers,
// deriving the layer name from the filename stem.
func loadTestLayers(t *testing.T, paths []string) ([]Layer, error) {
	t.Helper()
	layers := make([]Layer, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile("../../" + path)
		if err != nil {
			return nil, err
		}
		var values map[string]interface{}
		if err := yaml.Unmarshal(data, &values); err != nil {
			return nil, err
		}
		if values == nil {
			values = map[string]interface{}{}
		}
		layers = append(layers, Layer{
			Name:   layerNameFromPath(path),
			Values: values,
		})
	}
	return layers, nil
}

// layerNameFromPath derives a display name from a file path by stripping
// the directory and extension, e.g. "testdata/prod.yaml" → "prod".
func layerNameFromPath(path string) string {
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

func TestAnalyze_MultiLayerOnly(t *testing.T) {
	layers := []Layer{
		{
			Name: "base",
			Values: map[string]interface{}{
				"baseOnly":     "x",
				"replicaCount": 1,
			},
		},
		{
			Name: "prod",
			Values: map[string]interface{}{
				"replicaCount": 3,
			},
		},
	}

	all := Analyze(layers, Options{})
	filtered := Analyze(layers, Options{MultiLayerOnly: true})

	// Unfiltered should contain both keys.
	allByKey := map[string]ValueNode{}
	for _, n := range all {
		allByKey[n.Key] = n
	}
	if _, ok := allByKey["baseOnly"]; !ok {
		t.Error("unfiltered: expected baseOnly to be present")
	}
	if _, ok := allByKey["replicaCount"]; !ok {
		t.Error("unfiltered: expected replicaCount to be present")
	}

	// Filtered should suppress baseOnly (single-layer) but keep replicaCount.
	filtByKey := map[string]ValueNode{}
	for _, n := range filtered {
		filtByKey[n.Key] = n
	}
	if _, ok := filtByKey["baseOnly"]; ok {
		t.Error("filtered: baseOnly should be suppressed (only in base layer)")
	}
	if _, ok := filtByKey["replicaCount"]; !ok {
		t.Error("filtered: replicaCount should be present (in both layers)")
	}
}

func TestIsRedundant(t *testing.T) {
	layers := []Layer{
		{
			Name:   "base",
			Values: map[string]interface{}{"database": map[string]interface{}{"port": 5432}},
		},
		{
			// prod redundantly re-declares the same port as base
			Name:   "prod",
			Values: map[string]interface{}{"database": map[string]interface{}{"port": 5432}},
		},
	}

	nodes := Analyze(layers, Options{})
	byKey := map[string]ValueNode{}
	for _, n := range nodes {
		byKey[n.Key] = n
	}

	port := byKey["database.port"]

	// base (idx 0) is never redundant
	if port.IsRedundant(0) {
		t.Error("base layer should never be redundant")
	}
	// prod (idx 1) sets the same value as base — redundant
	if !port.IsRedundant(1) {
		t.Error("prod layer should be redundant for database.port")
	}
}

func TestIsRedundant_NotRedundant(t *testing.T) {
	layers := []Layer{
		{
			Name:   "base",
			Values: map[string]interface{}{"replicaCount": 1},
		},
		{
			Name:   "prod",
			Values: map[string]interface{}{"replicaCount": 3},
		},
	}

	nodes := Analyze(layers, Options{})
	byKey := map[string]ValueNode{}
	for _, n := range nodes {
		byKey[n.Key] = n
	}

	rc := byKey["replicaCount"]
	if rc.IsRedundant(1) {
		t.Error("prod replicaCount=3 differs from base=1, should not be redundant")
	}
}
