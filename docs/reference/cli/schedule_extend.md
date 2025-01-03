<!-- DO NOT EDIT | GENERATED CONTENT -->
# schedule extend

Extend the stop time of a currently running workspace instance.

Aliases:

* override-stop

## Usage

```console
coder schedule extend <workspace-name> <duration from now>
```

## Description

```console

  * The new stop time is calculated from *now*.
  * The new stop time must be at least 30 minutes in the future.
  * The workspace template may restrict the maximum workspace runtime.

 $ coder schedule extend my-workspace 90m
```
