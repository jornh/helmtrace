package main

import (
	"flag"
	"fmt"
	"os"

	"helmtrace/pkg/analyzer"
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
  helmtrace -f base.yaml -f env/prod.yaml [-f override.yaml] [--all-rows] [--output tui|plain|json]
  helmtrace -k ./overlays/prod [--all-rows] [--output tui|plain|json]

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
