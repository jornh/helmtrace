package main

import (
	"flag"
	"fmt"
	"os"

	"helmtrace/pkg/analyzer"
	"helmtrace/pkg/lint"
	"helmtrace/pkg/loader"
	"helmtrace/pkg/render"
)

// Build-time variables populated by ldflags:
//
//	-X main.Version={{ .Env.VERSION }}
//	-X main.Commit={{ .Env.COMMIT }}
//	-X main.CommitDate={{ .Env.COMMIT_DATE }}
//	-X main.TreeState={{ .Env.TREE_STATE }}
var (
	Version    = "dev"
	Commit     = "none"
	CommitDate = "unknown"
	TreeState  = "unknown"
)

func main() {
	// Top-level subcommand dispatch before flag parsing so that
	// `helmtrace version` works without any other flags.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		printVersion()
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "lint" {
		os.Exit(runLint(os.Args[2:]))
	}

	var files layerFlags
	var kustomizeRoot string
	var allRows bool
	var plain bool
	var output string

	flag.Var(&files, "f", "values file, may be repeated; order defines precedence (lowest first)")
	flag.StringVar(&kustomizeRoot, "k", "", "kustomize root directory; mutually exclusive with -f")
	flag.BoolVar(&allRows, "all-rows", false, "show all keys, including those that appear in only one layer")
	flag.BoolVar(&plain, "plain", false, "plain text output without colours or styling")
	flag.StringVar(&output, "output", "tui", "output format: tui, plain, json")
	flag.StringVar(&output, "o", "tui", "output format (shorthand)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `helmtrace - show provenance of values across layered Helm values files or kustomize overlays

Usage:
  helmtrace version
  helmtrace lint   [-f file ...|-k dir] [--error] [--output plain|json]
  helmtrace        [-f file ...|-k dir] [--all-rows] [--output tui|plain|json]

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	// --plain is a legacy alias for --output plain.
	if plain && output == "tui" {
		output = "plain"
	}

	if len(files) == 0 && kustomizeRoot == "" {
		flag.Usage()
		os.Exit(1)
	}
	if len(files) > 0 && kustomizeRoot != "" {
		fmt.Fprintln(os.Stderr, "error: -f and -k are mutually exclusive")
		os.Exit(1)
	}

	layers, err := loadLayers(files, kustomizeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	opts := analyzer.Options{MultiLayerOnly: !allRows}
	nodes := analyzer.Analyze(layers, opts)

	switch output {
	case "json":
		if err := render.JSON(os.Stdout, nodes, layers); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "plain":
		render.Table(os.Stdout, nodes, layers)
	default:
		render.TUITable(nodes, layers)
	}
}

// printVersion writes build info to stdout.
func printVersion() {
	dirty := ""
	if TreeState == "dirty" {
		dirty = " (dirty)"
	}
	fmt.Printf("helmtrace %s%s\n", Version, dirty)
	fmt.Printf("  commit:  %s\n", Commit)
	fmt.Printf("  date:    %s\n", CommitDate)
}

// runLint runs the lint subcommand and returns the process exit code.
// Exit 0 = clean, 1 = violations found (when --error) or load error.
func runLint(args []string) int {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	var files layerFlags
	var kustomizeRoot string
	var failOnRedundant bool
	var output string

	fs.Var(&files, "f", "values file, may be repeated; order defines precedence (lowest first)")
	fs.StringVar(&kustomizeRoot, "k", "", "kustomize root directory; mutually exclusive with -f")
	fs.BoolVar(&failOnRedundant, "error", false, "exit 1 when redundant values are found (default: exit 0 with warnings)")
	fs.StringVar(&output, "output", "plain", "output format: plain, json")
	fs.StringVar(&output, "o", "plain", "output format (shorthand)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `helmtrace lint - report redundant values across layered files

Usage:
  helmtrace lint -f base.yaml -f prod.yaml [--error] [--output plain|json]
  helmtrace lint -k ./overlays/prod [--error] [--output plain|json]

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if len(files) == 0 && kustomizeRoot == "" {
		fs.Usage()
		return 1
	}
	if len(files) > 0 && kustomizeRoot != "" {
		fmt.Fprintln(os.Stderr, "error: -f and -k are mutually exclusive")
		return 1
	}

	layers, err := loadLayers(files, kustomizeRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// Lint always analyses all keys, not just multi-layer ones.
	nodes := analyzer.Analyze(layers, analyzer.Options{MultiLayerOnly: false})
	violations := lint.Run(nodes, layers, lint.Options{FailOnRedundant: failOnRedundant})

	switch output {
	case "json":
		if err := lint.PrintJSON(os.Stdout, violations); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	default:
		lint.PrintText(os.Stdout, violations)
	}

	if lint.HasErrors(violations) {
		return 1
	}
	return 0
}

// loadLayers dispatches to the appropriate loader based on which flags were set.
func loadLayers(files []string, kustomizeRoot string) ([]analyzer.Layer, error) {
	var l loader.Loader
	if kustomizeRoot != "" {
		l = &loader.KustomizeLoader{Root: kustomizeRoot}
	} else {
		l = &loader.HelmLoader{Files: files}
	}
	return l.Load()
}

// layerFlags is a flag.Value that accumulates repeated -f arguments.
type layerFlags []string

func (f *layerFlags) String() string { return fmt.Sprint([]string(*f)) }
func (f *layerFlags) Set(v string) error {
	*f = append(*f, v)
	return nil
}

