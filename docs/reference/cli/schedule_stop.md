<!-- DO NOT EDIT | GENERATED CONTENT -->

# schedule stop

Edit workspace stop schedule

## Usage

```console
coder schedule stop <workspace-name> { <duration> | manual }
```

## Description

```console
Schedules a workspace to stop after a given duration has elapsed.
  * Workspace runtime is measured from the time that the workspace build completed.
  * The minimum scheduled stop time is 1 minute.
  * The workspace template may place restrictions on the maximum shutdown time.
  * Changes to workspace schedules only take effect upon the next build of the workspace,
    and do not affect a running instance of a workspace.

When enabling scheduled stop, enter a duration in one of the following formats:
  * 3h2m (3 hours and two minutes)
  * 3h   (3 hours)
  * 2m   (2 minutes)
  * 2    (2 minutes)

 $ coder schedule stop my-workspace 2h30m
```
