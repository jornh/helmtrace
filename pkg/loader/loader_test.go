package loader

import (
	"path/filepath"
	"testing"
)

// ── HelmLoader ────────────────────────────────────────────────────────────────

func TestHelmLoader_Basic(t *testing.T) {
	dir := "../../testdata/"
	l := &HelmLoader{Files: []string{
		filepath.Join(dir, "base.yaml"),
		filepath.Join(dir, "prod.yaml"),
	}}
	layers, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(layers))
	}
	if layers[0].Name != "base" {
		t.Errorf("layers[0].Name = %q, want base", layers[0].Name)
	}
	if layers[1].Name != "prod" {
		t.Errorf("layers[1].Name = %q, want prod", layers[1].Name)
	}
	if layers[1].Values["replicaCount"] != 3 {
		t.Errorf("prod replicaCount = %v, want 3", layers[1].Values["replicaCount"])
	}
}

func TestHelmLoader_EmptyFile(t *testing.T) {
    dir := "../../testdata/"
	l := &HelmLoader{Files: []string{filepath.Join(dir, "empty.yaml")}}
	layers, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(layers))
	}
	if layers[0].Values == nil {
		t.Error("Values should be non-nil for empty file")
	}
}

func TestHelmLoader_MissingFile(t *testing.T) {
	dir := t.TempDir()
	l := &HelmLoader{Files: []string{filepath.Join(dir, "nonexistent/file.yaml")}}
	_, err := l.Load()
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// ── KustomizeLoader ───────────────────────────────────────────────────────────

func TestKustomizeLoader_FlatPatchesStrategicMerge(t *testing.T) {
	l := &KustomizeLoader{Root: "testdata/kustomize/flat-strategic-merge"}
	layers, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(layers))
	}
	if layers[0].Name != "base" {
		t.Errorf("layers[0].Name = %q, want base", layers[0].Name)
	}
	if layers[1].Name != "patch" {
		t.Errorf("layers[1].Name = %q, want patch", layers[1].Name)
	}
	// Envelope stripped — apiVersion/kind/metadata must not be present.
	for _, banned := range []string{"apiVersion", "kind", "metadata"} {
		if _, ok := layers[0].Values[banned]; ok {
			t.Errorf("base: envelope field %q should have been stripped", banned)
		}
	}
	// spec should be present.
	if _, ok := layers[0].Values["spec"]; !ok {
		t.Error("base: spec missing after strip")
	}
	// patch replicas lives under spec.replicas.
	spec, ok := layers[1].Values["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("patch: spec missing or wrong type")
	}
	if spec["replicas"] != 3 {
		t.Errorf("patch spec.replicas = %v, want 3", spec["replicas"])
	}
}

func TestKustomizeLoader_PatchesField(t *testing.T) {
	l := &KustomizeLoader{Root: "testdata/kustomize/patches-field"}
	layers, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(layers))
	}
	spec, ok := layers[1].Values["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("prod: spec missing or wrong type")
	}
	if spec["replicas"] != 5 {
		t.Errorf("prod spec.replicas = %v, want 5", spec["replicas"])
	}
}

func TestKustomizeLoader_RecursiveBase(t *testing.T) {
	l := &KustomizeLoader{Root: "testdata/kustomize/recursive-base"}
	layers, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2 (base values + prod patch)", len(layers))
	}
	if layers[0].Name != "values" {
		t.Errorf("layers[0].Name = %q, want values", layers[0].Name)
	}
	if layers[1].Name != "prod" {
		t.Errorf("layers[1].Name = %q, want prod", layers[1].Name)
	}
	// Both layers should have spec after stripping.
	for i, l := range layers {
		if _, ok := l.Values["spec"]; !ok {
			t.Errorf("layers[%d]: spec missing after strip", i)
		}
	}
}

func TestKustomizeLoader_MultiPatch(t *testing.T) {
	l := &KustomizeLoader{Root: "testdata/kustomize/multi-patch"}
	layers, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// base (multi-doc: Deployment + Ingress) + 3 patches = 4 layers.
	if len(layers) != 4 {
		t.Fatalf("got %d layers, want 4", len(layers))
	}
	wantNames := []string{"base", "patch-env", "patch-ingress", "patch-resources"}
	for i, want := range wantNames {
		if layers[i].Name != want {
			t.Errorf("layers[%d].Name = %q, want %q", i, layers[i].Name, want)
		}
	}
	// base is multi-doc — values should be namespaced under "Deployment/myapp"
	// and "Ingress/myapp".
	if _, ok := layers[0].Values["Deployment/myapp"]; !ok {
		t.Error("base: Deployment/myapp key missing (multi-doc namespacing)")
	}
	if _, ok := layers[0].Values["Ingress/myapp"]; !ok {
		t.Error("base: Ingress/myapp key missing (multi-doc namespacing)")
	}
}

func TestKustomizeLoader_MissingKustomizationFile(t *testing.T) {
	l := &KustomizeLoader{Root: t.TempDir()} // empty dir, no kustomization.yaml
	_, err := l.Load()
	if err == nil {
		t.Error("expected error for missing kustomization.yaml, got nil")
	}
}

func TestStripEnvelope(t *testing.T) {
	doc := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "myapp"},
		"spec": map[string]interface{}{
			"replicas": 3,
		},
		"status": map[string]interface{}{
			"readyReplicas": 3,
		},
	}
	stripped := stripEnvelope(doc)

	for _, banned := range []string{"apiVersion", "kind", "metadata", "status"} {
		if _, ok := stripped[banned]; ok {
			t.Errorf("field %q should have been stripped", banned)
		}
	}
	spec, ok := stripped["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("spec missing after strip")
	}
	if spec["replicas"] != 3 {
		t.Errorf("spec.replicas = %v, want 3", spec["replicas"])
	}
}

// ── LayerName ─────────────────────────────────────────────────────────────────

func TestLayerName(t *testing.T) {
    tests := map[string]string{
        "base/scg/values.yaml":          "base/scg",
        "overlays/common/scg.yaml":      "scg",
        "overlays/uat/scg/values.yaml":  "uat/scg",
        "values.yaml":                   "values",
        "uat.yaml":                      "uat",
        "foo/bar/my-values.yml":         "my-values",
    }

    for input, expected := range tests {
        got := LayerName(input)
        if got != expected {
            t.Errorf("LayerName(%q) = %q, want %q", input, got, expected)
        }
    }
}
	
