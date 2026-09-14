# checkexperimentkeys

`checkexperimentkeys` validates experiment keys explicitly named in Markdown
under `docs/` against `codersdk.ExperimentsKnown`. It runs as
`make lint/check-experiment-keys`, which is part of `make lint`.

## Recognized forms

The check only recognizes explicit enablement examples:

- `--experiments=<key>`
- `CODER_EXPERIMENTS=<key>`
- comma-separated key lists in either form, such as
  `--experiments=key-one,key-two`

It deliberately does not try to infer bare identifiers from prose. The
placeholder values `feature1`, `feature2`, `<experiment-key>`, and `*` are
ignored.

## Usage

```console
$ go run ./scripts/checkexperimentkeys        # scans docs/
$ go run ./scripts/checkexperimentkeys path/to/file.md path/to/dir
```
