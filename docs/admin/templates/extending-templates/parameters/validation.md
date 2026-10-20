---
title: Parameter validation
description: Constrain parameter values with ranges, regular expressions, and rules that depend on other parameters.
---

The `validation` block of a [`coder_parameter`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter) rejects values that your template can't use before Coder starts a build.
Coder supports several validation modes: minimum and maximum values, monotonic numbers, and regular expressions.
A validation rule can also depend on the values of other parameters.

## Number validation

You can limit a `number` parameter to `min` and `max` boundaries.

You can also specify its monotonicity as `increasing` or `decreasing` to verify the current and new values.
Use the `monotonic` attribute for resources that can't be shrunk or grown without implications, like disk volume size.

```tf
data "coder_parameter" "instances" {
  name        = "Instances"
  type        = "number"
  description = "Number of compute instances"
  validation {
    min       = 1
    max       = 8
    monotonic = "increasing"
  }
}
```

It is possible to override the default `error` message for a `number` parameter, along with its associated `min` and/or `max` properties.
The following message placeholders are available `{min}`, `{max}`, and `{value}`.

```tf
data "coder_parameter" "instances" {
  name        = "Instances"
  type        = "number"
  description = "Number of compute instances"
  validation {
    min       = 1
    max       = 4
    error     = "Sorry, we can't provision too many instances - maximum limit: {max}, wanted: {value}."
  }
}
```

A `number` parameter can also define `options`, and validations such as `monotonic` still apply to it.

## String validation

You can validate a `string` parameter to match a regular expression.
The `regex` property requires a corresponding `error` property.

```tf
data "coder_parameter" "project_id" {
  name        = "Project ID"
  description = "Alpha-numeric project ID"
  validation {
    regex = "^[a-z0-9]+$"
    error = "Unfortunately, this isn't a valid project ID"
  }
}
```

## Validation that depends on other parameters

A parameter's validation block can use the values of other parameters.
This example sets both the default and the maximum CPU core count based on the selected repository.

[Try dependent validation in the Parameter Playground](https://playground.coder.app/parameters/sdbzXxagJ4).

```tf
data "coder_parameter" "git_repo" {
  name = "git_repo"
  display_name = "Git repo"
  description = "Select a git repo to work on."
  order = 1
  mutable = true
  type = "string"
  form_type = "dropdown"

  option {
    # A Go-heavy repository
    name = "coder/coder"
    value = "coder/coder"
  }

  option {
    # A python-heavy repository
    name = "coder/mlkit"
    value = "coder/mlkit"
  }
}

data "coder_parameter" "cpu_cores" {
  name         = "cpu_cores"
  display_name = "CPU Cores"
  type         = "number"
  form_type    = "slider"
  order        = 2

  # Dynamically set default
  default      = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? 12 : 6

  validation {
    min = 1

    # Dynamically set max validation
    max = try(data.coder_parameter.git_repo.value, "") == "coder/mlkit" ? 16 : 8
  }
}
```
