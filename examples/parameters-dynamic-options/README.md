---
name: Sample Template with Dynamic Parameter Options
description: Review the sample template and introduce dynamic parameter options to your template
tags: [local, docker, parameters]
icon: /icon/docker.png
---

# Overview

This Coder template presents use of [dynamic](https://developer.hashicorp.com/terraform/language/expressions/dynamic-blocks) [parameter options](https://coder.com/docs/v2/latest/templates/parameters#options) and Terraform [locals](https://developer.hashicorp.com/terraform/language/values/locals).

## Use case

The Coder template makes use of Docker containers to provide workspace users with programming language SDKs like Go, Java, and more.
The template administrator wants to make sure that only certain versions of these programming environments are available,
without allowing users to manually change them.

Workspace users should simply choose the programming environment they prefer. When the template admin upgrades SDK versions,
during the next workspace update the chosen environment will automatically be upgraded to the latest version without causing any disruption or prompts.

The references to Docker images are represented by Terraform variables, and they're passed via the configuration file like this:

```yaml
go_image: "bitnami/golang:1.20-debian-11"
java_image: "bitnami/java:1.8-debian-11
```

The template admin needs to update image references, publish a new version of the template, and then either handle workspace updates on their own or let workspace users take care of it.

## Development

Update the template and push it using the following command:

```bash
./scripts/coder-dev.sh templates push examples-parameters-dynamic-options \
  -d examples/parameters-dynamic-options \
  --variables-file examples/parameters-dynamic-options/variables.yml \
  -y
```
