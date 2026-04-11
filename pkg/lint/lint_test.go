package lint

import (
	"bytes"
	"strings"
	"testing"

	"helmtrace/internal/testutil"
	"helmtrace/pkg/analyzer"
)

func TestRun_RedundantWarn(t *testing.T) {
	layers, nodes := redundantScenario()

	violations := Run(nodes, layers, Options{})

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	v := violations[0]
	if v.Key != "database.port" {
		t.Errorf("Key = %q, want database.port", v.Key)
	}
	if v.Layer != "prod" {
		t.Errorf("Layer = %q, want prod", v.Layer)
	}
	if v.Severity != SeverityWarn {
		t.Errorf("Severity = %q, want warn", v.Severity)
	}
}

func TestRun_RedundantError(t *testing.T) {
	layers, nodes := redundantScenario()

	violations := Run(nodes, layers, Options{FailOnRedundant: true})

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	if violations[0].Severity != SeverityError {
		t.Errorf("Severity = %q, want error", violations[0].Severity)
	}
}

func TestRun_NoViolations(t *testing.T) {
	layers := []analyzer.Layer{
		{Name: "base", Values: map[string]interface{}{"replicaCount": 1}},
		{Name: "prod", Values: map[string]interface{}{"replicaCount": 3}},
	}
	nodes := analyzer.Analyze(layers, analyzer.Options{})

	violations := Run(nodes, layers, Options{})
	if len(violations) != 0 {
		t.Errorf("got %d violations, want 0", len(violations))
	}
}

func TestRun_NullOverrideNotRedundant(t *testing.T) {
	// Explicit null should never be flagged as redundant.
	layers := []analyzer.Layer{
		{Name: "base", Values: map[string]interface{}{"feature": "enabled"}},
		{Name: "prod", Values: map[string]interface{}{"feature": nil}},
	}
	nodes := analyzer.Analyze(layers, analyzer.Options{})

	violations := Run(nodes, layers, Options{FailOnRedundant: true})
	if len(violations) != 0 {
		t.Errorf("null override flagged as redundant: got %d violations, want 0", len(violations))
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors(nil) {
		t.Error("nil violations should not have errors")
	}
	warns := []Violation{{Severity: SeverityWarn}}
	if HasErrors(warns) {
		t.Error("warn-only violations should not have errors")
	}
	errs := []Violation{{Severity: SeverityError}}
	if !HasErrors(errs) {
		t.Error("error violations should return true")
	}
}

func TestPrintText_Violations(t *testing.T) {
	layers, nodes := redundantScenario()
	violations := Run(nodes, layers, Options{})

	var buf bytes.Buffer
	PrintText(&buf, violations)
	out := buf.String()

	if !strings.Contains(out, "warn:") {
		t.Errorf("expected warn: prefix, got:\n%s", out)
	}
	if !strings.Contains(out, "database.port") {
		t.Errorf("expected key in output, got:\n%s", out)
	}
	if !strings.Contains(out, "1 warning(s), 0 error(s)") {
		t.Errorf("expected summary line, got:\n%s", out)
	}
}

func TestPrintText_NoViolations(t *testing.T) {
	var buf bytes.Buffer
	PrintText(&buf, nil)
	if !strings.Contains(buf.String(), "ok:") {
		t.Errorf("expected ok: prefix for clean output, got: %s", buf.String())
	}
}

func TestPrintJSON(t *testing.T) {
	layers, nodes := redundantScenario()
	violations := Run(nodes, layers, Options{})

	var buf bytes.Buffer
	if err := PrintJSON(&buf, violations); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"key"`) {
		t.Errorf("expected JSON output with key field, got:\n%s", out)
	}
	if !strings.Contains(out, `"severity"`) {
		t.Errorf("expected JSON output with severity field, got:\n%s", out)
	}
}

// redundantScenario returns a two-layer setup where prod redundantly
// re-declares database.port at the same value as base.
func redundantScenario() ([]analyzer.Layer, []analyzer.ValueNode) {
	layers := []analyzer.Layer{
		{
			Name: "base",
			Values: map[string]interface{}{
				"database": map[string]interface{}{"port": 5432},
			},
		},
		{
			Name: "prod",
			Values: map[string]interface{}{
				"database": map[string]interface{}{"port": 5432},
			},
		},
	}
	nodes := analyzer.Analyze(layers, analyzer.Options{})
	return layers, nodes
}

func TestRun_LocationInMessage(t *testing.T) {
	// Load a real file so Node is populated and location can be resolved.
	const yaml = `database:
  port: 5432
`
	layers, nodes := redundantScenarioFromYAML(t, yaml, yaml)
	violations := Run(nodes, layers, Options{})

	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
	v := violations[0]
	if v.Location == nil {
		t.Fatal("expected Location to be set when layer has a Node")
	}
	if v.Location.Line == 0 {
		t.Error("expected non-zero line number")
	}
	// Message should include the file:line:col prefix.
	if !strings.Contains(v.Message, ":") {
		t.Errorf("expected location in message, got: %q", v.Message)
	}
}

func TestPrintJSON_Location(t *testing.T) {
	const yaml = `database:
  port: 5432
`
	layers, nodes := redundantScenarioFromYAML(t, yaml, yaml)
	violations := Run(nodes, layers, Options{})

	var buf bytes.Buffer
	if err := PrintJSON(&buf, violations); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"line"`) {
		t.Errorf("expected line field in JSON output, got:\n%s", out)
	}
	if !strings.Contains(out, `"file"`) {
		t.Errorf("expected file field in JSON output, got:\n%s", out)
	}
}

// redundantScenarioFromYAML builds two layers from raw YAML strings so that
// *yaml.Node is populated and location tracking can be exercised.
func redundantScenarioFromYAML(t *testing.T, baseYAML, prodYAML string) ([]analyzer.Layer, []analyzer.ValueNode) {
	t.Helper()
	layers := []analyzer.Layer{
		testutil.LayerFromYAML("base", "base.yaml", baseYAML),
		testutil.LayerFromYAML("prod", "prod.yaml", prodYAML),
	}
	nodes := analyzer.Analyze(layers, analyzer.Options{})
	return layers, nodes
}
