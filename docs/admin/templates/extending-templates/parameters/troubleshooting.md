---
title: Troubleshoot parameters
description: Diagnose and resolve common problems with parameter forms.
---

## Parameters look the same after you turn off compatibility mode

Confirm that the template uses version [2.5.3](https://github.com/coder/terraform-provider-coder/releases/tag/v2.5.3) or later of `coder/terraform-provider-coder`.

Turning off [compatibility mode](./compatibility-mode.md) on an existing template requires administrators to publish a new template version.
This will resolve the necessary template metadata to render the form.
To publish one without editing the template's Terraform, [refresh the template data](../../managing-templates/index.md#refresh-template-data).

## Template variables not showing up

Parameter forms support template variables.
If a template variable doesn't work as expected in the form, [file an issue](https://github.com/coder/coder/issues/new?labels=parameters) in the Coder GitHub repository with the `parameters` label.

## Registry modules

Registry modules are supported in parameter forms.
Unless explicitly mentioned, no registry module requires the features in this section, so you can safely build new parameters on top of the functionality provided in the registry.

## Module not loaded errors

The parameter form requires Terraform modules to be archived and stored in the database.
Coder limits module archives to **20&nbsp;MB total** to prevent database bloat.
If your template uses modules that exceed this limit, some modules will be unavailable for parameter declarations.

**Symptoms:**

Warnings may exist in the provisioner logs:

```txt
[API] 2026-01-29 22:00:22.691 [warn]  provisionerd-nixos-0.executor: some (or all) terraform modules were not archived, template will have reduced function  skipped_modules=large:git::https://github.com/coder/large-module.git
```

In this case, reduce the size of the module by removing unnecessary files.

You can get the same error for a different reason.
If the active template version has no cached module archive at all, the workspace creation form shows a warning for every module in the template, for example:

```txt
Module not loaded. Did you run `terraform init`?
Module 'jetbrains' in file "main.tf:149,1-19" cannot be resolved. This module will be ignored.
```

This happens for template versions published before Coder started archiving modules for parameter evaluation.
**Workspace builds still succeed**, since Terraform fetches modules from their original sources during the build regardless of the cache; only the form's ability to evaluate module-backed parameter values is affected.
To fix it, [publish a new template version](../../managing-templates/index.md#refresh-template-data), which re-runs `terraform init` and populates the archive for that version.

This archive is the same one Coder reuses across workspace builds to avoid re-downloading modules.
Refer to [module caching](../modules.md#module-caching) for how to disable that behavior for a template.
