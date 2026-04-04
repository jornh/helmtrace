# helmtrace

Context: You have a setup with several “layers” of helm values files. You want help with that! 

## What does it offer?

- **Detect redundancy** — `redundant`: values set in a higher layer that are identical to what the lower layer already provides (DRY violations)
- **Show provenance** — `prov | provenance`: for any key, which layer(s) define it and what each layer's value is
- **Trim suggestions** — `trim`: flag keys that could be removed from a layer without changing the effective merged result

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

```
KEY                        base        env/prod    override    EFFECTIVE
database.host              db.internal db.prod     —           db.prod
database.port              5432        —           —           5432    ← filtered out unless --all-rows
image.tag                  latest      latest      —           latest  ← redundant in env/prod
replicaCount               1           3           5           5
```

The `5432` row is the DRY violation — `env/prod` is setting something the `base` already provides at the same value.


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
