# checkexperimentregistry

`checkexperimentregistry` checks `experiments/registry.yaml` against the
experiment flags in `codersdk.ExperimentsKnown`. It runs as
`make lint/check-experiment-registry`, which is part of `make lint`.

## Checks

- Every flag in `codersdk.ExperimentsKnown` belongs to exactly one registry
  entry.
- Every `flag` in the registry is in `codersdk.ExperimentsKnown`.
- Every entry has a unique `key`, a `stage` of `early-access`, `beta`, or
  `ga`, and a `since` date in `YYYY-MM-DD` form.
- The registry has no fields outside its schema.

Adding, renaming, or removing a flag therefore needs a registry change.
`CODEOWNERS` routes registry changes to its owners for approval.

## Usage

```console
$ go run ./scripts/checkexperimentregistry                  # reads experiments/registry.yaml
$ go run ./scripts/checkexperimentregistry path/to/registry.yaml
```
