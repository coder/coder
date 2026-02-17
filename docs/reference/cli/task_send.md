<!-- DO NOT EDIT | GENERATED CONTENT -->
# task send

Send input to a task

## Usage

```console
coder task send [flags] <task> [<input> | --stdin]
```

## Description

```console
  - Send direct input to a task.:

     $ coder task send task1 "Please also add unit tests"

  - Send input from stdin to a task.:

     $ echo "Please also add unit tests" | coder task send task1 --stdin
```

## Options

### --stdin

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Reads the input from stdin.
