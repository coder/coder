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
- single- or double-quoted values, such as `CODER_EXPERIMENTS="key-one"`
- shell line continuations that start with one of the forms above

It deliberately does not try to infer bare identifiers from prose. The
placeholder values `*`, the documented illustrative pair `feature1` and
`feature2`, and any angle-bracketed value such as `<experiment-key>` or
`<key>` are ignored. Other `featureN` values are checked so a real stale key
is not treated as a placeholder.

Directory symlinks are not traversed. This keeps a directory scan contained to
the explicitly supplied tree and prevents a documentation checkout from
silently scanning Markdown outside it. A Markdown file supplied directly is
still scanned according to normal file access rules.

## Usage

```console
$ go run ./scripts/checkexperimentkeys        # scans docs/
$ go run ./scripts/checkexperimentkeys path/to/file.md path/to/dir
```
