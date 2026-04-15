# helmtrace

Trace the provenance of values across either layered Helm values files or
Kustomize overlays. See exactly where every key comes from, spot redundancies,
and enforce DRY config in CI.

## What does it offer?

- **Provenance** — for any key, which layer(s) define it and what each layer's
  value is
- **Redundancy detection** — values set in a higher layer that are identical to
  what a lower layer already provides (DRY violations), marked with `✕`
- **Trim** — (upcoming) `helmtrace trim` outputs a copy of a target layer with
  redundant keys removed — safe to write back
- **Lint** — `helmtrace lint` reports redundancies as warnings or errors, with
  a non-zero exit code for CI enforcement

## Installation

```bash
# via mise
mise use github:jornh/helmtrace

# direct download (replace OS/arch as needed)
curl -sSL "https://github.com/jornh/helmtrace/releases/latest/download/helmtrace_$(uname -s)_$(uname -m).tar.gz" \
  | tar -xz -C /usr/local/bin helmtrace

# via Docker (ghcr.io)
docker run --rm -v $(pwd):/work ghcr.io/jornh/helmtrace:latest \
  -f /work/base.yaml -f /work/prod.yaml
```

## Usage

### Helm values files

```bash
# default output: coloured TUI table, multi-layer keys only
helmtrace -f base.yaml -f env/prod.yaml -f override.yaml

# include base-only keys
helmtrace -f base.yaml -f env/prod.yaml --all-rows

# plain text or JSON
helmtrace -f base.yaml -f env/prod.yaml -o plain
helmtrace -f base.yaml -f env/prod.yaml -o json
```

### Kustomize overlays

```bash
helmtrace -k ./overlays/prod
helmtrace -k ./overlays/prod -o json
```

### Lint (CI enforcement)

```bash
# warn about redundant values (exit 0)
helmtrace lint -f base.yaml -f prod.yaml

# fail the build on redundant values (exit 1)
helmtrace lint -f base.yaml -f prod.yaml --error

# kustomize lint with JSON output for tooling
helmtrace lint -k ./overlays/prod --error -o json
```

## Output examples

### Provenance table (Helm)

```bash
$ helmtrace -f base.yaml -f env/prod.yaml -f override.yaml --all-rows

KEY                  base                      env/prod                    override    EFFECTIVE
────────────────────────────────────────────────────────────────────────────────────────────────────────────
database.host        db.internal               db.prod                     —           db.prod
database.port        5432                      —                           —           5432
replicaCount         1                         3                           5           5
sidecars.0.image     fluent/fluent-bit:2.2     fluent/fluent-bit:3.0       —           fluent/fluent-bit:3.0
sidecars.0.name      logging                   logging ✕                   —           logging
sidecars.1.image     prom/statsd-exporter:v0…  prom/statsd-exporter:v0… ✕  —           prom/statsd-exporter:v0.26
sidecars.1.name      metrics                   metrics ✕                   —           metrics
tags.0               backend                   —                           —           backend
tags.1               production                —                           —           production

✕ = redundant (identical to effective value from lower layers)
```

`✕` marks DRY violations — `env/prod` is setting something `base` already
provides at the same value. Without `--all-rows`, keys that only appear in one
layer are hidden to reduce noise.

### Provenance table (Kustomize)

Output is grouped by Kubernetes resource when using `-k`:

```bash
Deployment/myapp
────────────────────────────────────────────────────────────────────
KEY                                    base        prod        EFFECTIVE
spec.replicas                          1           3           3
spec.template.spec.containers.0.image  myapp:1.0   myapp:2.0   myapp:2.0

Ingress/myapp
────────────────────────────────────────────────────────────────────
KEY                   base  prod                    EFFECTIVE
spec.rules.0.host     —     myapp.prod.example.com  myapp.prod.example.com
```

### Lint output

```bash
$ helmtrace lint -f base.yaml -f prod.yaml

warn: "sidecars.0.name" in layer "prod" is redundant: identical to effective value from lower layers
warn: "sidecars.1.image" in layer "prod" is redundant: identical to effective value from lower layers
warn: "sidecars.1.name" in layer "prod" is redundant: identical to effective value from lower layers

3 warning(s), 0 error(s)
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-f` | — | Values file, repeatable, lowest precedence first |
| `-k` | — | Kustomize root directory (mutually exclusive with `-f`) |
| `--all-rows` | `false` | Show all keys including single-layer ones |
| `-o`, `--output` | `tui` | Output format: `tui`, `plain`, `json` |

**Lint-specific:**

| Flag | Default | Description |
| --- | --- | --- |
| `--error` | `false` | Exit 1 on redundant values (default: exit 0 with warnings) |
| `-o`, `--output` | `plain` | Output format: `plain`, `json` |

## Architecture

```txt
main.go           ← subcommand dispatch: (default), lint, version

pkg/
  analyzer/           ← core logic, Kubernetes-unaware
    provenance.go     ← Layer, Source, ValueNode types; Analyze(); IsRedundant()

  loader/             ← file loading, one Layer per values source
    interface.go      ← Loader interface
    helm.go           ← loads -f values files → []Layer
    kustomize.go      ← walks kustomization.yaml, strips K8s envelope,
                         sets ResourceKey per document

  lint/               ← redundancy checks for CI enforcement
    lint.go           ← Run(); Violation type; severity (warn/error)
    render.go         ← PrintText(), PrintJSON()

  render/             ← output renderers
    groups.go         ← BuildGroups() — shared grouping by ResourceKey
    table.go          ← plain text, section-per-resource
    tui.go            ← lipgloss-styled terminal output, width-aware
    json.go           ← structured JSON, flat or grouped by resource
```

There is a Glossary at https://deepwiki.com/jornh/helmtrace/9-glossary giving a good first intro to terms and concepts

**Key design boundary:** `pkg/analyzer` is completely unaware of Kubernetes -
and if the source is Kustomize or Helm. It only ever sees `[]Layer`.
All Kubernetes-specific knowledge (envelope stripping, `ResourceKey`, patch
ordering) lives in `pkg/loader/kustomize.go`.

## Roadmap

| Feature | Status |
| --- | --- |
| `patchesStrategicMerge` (pre-v4) | ✅ |
| `patches:` with `path:` (v4+) | ✅ |
| Recursive base directories | ✅ |
| Multi-document YAML files | ✅ |
| Lint / CI enforcement | ✅ |
| `helmtrace trim` | 🔜 |
| `images:` transformer | 🔜 |
| JSON patches (`op/path/value`) | 🔜 |
| Source line/column tracking | 🔜 |
| Remote bases | 🔜 |
