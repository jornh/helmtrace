package analyzer

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNodeAt_ScalarLeaf(t *testing.T) {
	src := `database:
  host: db.internal
  port: 5432
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}

	loc := nodeAt(&root, "database.host", "base.yaml")
	if loc == nil {
		t.Fatal("expected location, got nil")
	}
	if loc.File != "base.yaml" {
		t.Errorf("File = %q, want base.yaml", loc.File)
	}
	// "host: db.internal" is on line 2 in the YAML above.
	if loc.Line != 2 {
		t.Errorf("Line = %d, want 2", loc.Line)
	}
}

func TestNodeAt_SequenceIndex(t *testing.T) {
	src := `sidecars:
  - name: logging
    image: fluent/fluent-bit:2.2
  - name: metrics
    image: prom/statsd-exporter:v0.26
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}

	loc := nodeAt(&root, "sidecars.1.image", "base.yaml")
	if loc == nil {
		t.Fatal("expected location for sidecars.1.image, got nil")
	}
	// "image: prom/statsd-exporter:v0.26" is on line 5.
	if loc.Line != 5 {
		t.Errorf("Line = %d, want 5", loc.Line)
	}
}

func TestNodeAt_MissingPath(t *testing.T) {
	src := `foo: bar`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatal(err)
	}

	if loc := nodeAt(&root, "foo.does.not.exist", "f.yaml"); loc != nil {
		t.Errorf("expected nil for missing path, got %v", loc)
	}
}

func TestNodeAt_NilRoot(t *testing.T) {
	if loc := nodeAt(nil, "foo", "f.yaml"); loc != nil {
		t.Errorf("expected nil for nil root, got %v", loc)
	}
}

func TestSourceLocation_String(t *testing.T) {
	loc := SourceLocation{File: "prod.yaml", Line: 12, Column: 5}
	if got := loc.String(); got != "prod.yaml:12:5" {
		t.Errorf("String() = %q, want prod.yaml:12:5", got)
	}
	empty := SourceLocation{}
	if got := empty.String(); got != "" {
		t.Errorf("empty String() = %q, want empty", got)
	}
}
