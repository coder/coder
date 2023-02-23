<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder schedule override-stop

Override the stop time of a currently running workspace instance.

- The new stop time is calculated from _now_.
- The new stop time must be at least 30 minutes in the future.
- The workspace template may restrict the maximum workspace runtime.

## Usage

```console
coder schedule override-stop <workspace-name> <duration from now> [flags]
```

## Examples

```console
  $ coder schedule override-stop my-workspace 90m
```
