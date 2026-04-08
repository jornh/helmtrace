# helmtrace

Context: You have a setup with several “layers” of helm values files. You want help with that! 

## What does it offer?

- **Show provenance** — `prov | provenance`: for any key, which layer(s) define it and what each layer's value is
- **Detect redundancy** — `redundant`: values set in a higher layer that are identical to what the lower layer already provides (DRY violations)
- **Trim suggestions** — `trim`: flag keys that could be removed from a layer without changing the effective merged result (DRY it up!)

To do that it will:

1. Parse layered Helm values files (base → environment → override, etc.)
2. Merge them in the correct precedence order
3. --> perform analysis (the 3 above)
4. Render result

## How do I run `helmtrace`?

```bash
# one way to install
mise use github:jornh/helmtrace
# CLI shape
helmtrace -f testdata/base/values.yaml -f testdata/env/prod.yaml [-f testdata/override.yaml] [--all-rows]

# TODO helmtrace trim       -f base.yaml -f env/prod.yaml --layer env/prod.yaml
# `trim` outputs a copy of the target layer with redundant keys removed — safe to write back.
```

....

`provenance` output example:

```bash
$ helmtrace -f testdata/base.yaml -f testdata/prod.yaml -f testdata/override.yaml --all-rows
KEY                 base                        env/prod                    override                    EFFECTIVE
──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
database.host       db.internal                 db.prod                     —                           db.prod
database.port       5432                        —                           —                           5432                           ← filtered out unless --all-rows
replicaCount        1                           3                           5                           5
sidecars.0.image    fluent/fluent-bit:2.2       fluent/fluent-bit:3.0       —                           fluent/fluent-bit:3.0
sidecars.0.name     logging                     logging ✕                   —                           logging
sidecars.1.image    prom/statsd-exporter:v0…    prom/statsd-exporter:v0… ✕  —                           prom/statsd-exporter:v0.26
sidecars.1.name     metrics                     metrics ✕                   —                           metrics
tags.0              backend                     —                           —                           backend                        ← filtered out unless --all-rows
tags.1              production                  —                           —                           production                     ← filtered out unless --all-rows

✕ = redundant (identical to effective value from lower layers)
```

The ✕ = redundant marked cells are DRY violations — `env/prod` is setting something the `base` already provides at the same value.


## Details

### Architecture
```bash
cmd/
  helmtrace/
    main.go
pkg/
  loader/     # load and label each values file with its layer name
  merger/     # merge using Helm's actual coalesceTables logic
  analyzer/
    redundancy.go   # key exists in layer N but value == effective value from layer N-1
    provenance.go   # for each leaf key, list [layer → value] chain
    trim.go         # which keys in layer N can be dropped safely
  render/
    table.go    # terminal table output
    json.go     # machine-readable for CI
```
