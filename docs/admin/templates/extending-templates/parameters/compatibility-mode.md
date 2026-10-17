---
title: Parameter compatibility mode
---

Parameter compatibility mode keeps a template on the classic parameter flow, which renders a static form without [conditional parameters](./conditional-parameters.md), [identity-aware parameters](./identity-aware-parameters.md), or the newer [form types](./form-types.md).
Coder maintains a separate code path for that flow.
The classic flow is deprecated and will be removed in a future release, so templates that use compatibility mode need to be upgraded before then.

## Find templates that use compatibility mode

Open the **Templates** page and select the **Templates using compatibility mode** filter, or search for `compatibility_mode:true`.

## Upgrade from parameter compatibility mode

For each template:

1. Confirm your versions meet the requirements:

   - `coder/coder`: >= [v2.25.0](https://github.com/coder/coder/releases/tag/v2.25.0)
   - `coder/terraform-provider-coder`: >= [v2.5.3](https://github.com/coder/terraform-provider-coder/releases/tag/v2.5.3)

1. Select **Settings** > **Parameters** on the template, then clear **Use parameter compatibility mode for workspace builds**.

1. Publish a new template version so Coder can resolve the parameter metadata.
   If you don't need to change the Terraform, [refresh the template data](../../managing-templates/index.md#refresh-template-data) to publish a new version from the existing source files.

1. Open the workspace creation form and confirm the parameters render as expected.

You can make the same change through the `use_classic_parameter_flow` field in the [templates API](../../../../reference/api/templates.md#update-template-settings-by-id).

If the form looks the same after the switch, refer to [Parameters look the same after you turn off compatibility mode](./troubleshooting.md#parameters-look-the-same-after-you-turn-off-compatibility-mode).

## Revert to compatibility mode

If a template does not work after the switch, you can opt that template back into the classic flow.
Select **Settings** > **Parameters** on the template, then select **Use parameter compatibility mode for workspace builds**.
You can also set the `use_classic_parameter_flow` field through the [templates API](../../../../reference/api/templates.md#update-template-settings-by-id).

If your template's parameters do not work without compatibility mode, [file an issue](https://github.com/coder/coder/issues/new?labels=parameters) with the `parameters` label.
