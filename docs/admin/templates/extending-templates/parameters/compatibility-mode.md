---
title: Parameter compatibility mode
description: Find templates that use the deprecated classic parameter flow and upgrade them.
---

Parameter compatibility mode keeps a template on the classic parameter flow, which renders a static form without [conditional parameters](./conditional-parameters.md), [identity-aware parameters](./identity-aware-parameters.md), or the newer [form types](./form-types.md).
Coder maintains a separate code path for that flow.
The classic flow is deprecated and will be removed in a future release, so templates that use compatibility mode need to be upgraded before then.

## Find templates that use compatibility mode

Open the **Templates** page and select the **Templates using compatibility mode** filter, or search for `compatibility_mode:true`.

## Upgrade from parameter compatibility mode

For each template:

1. Confirm that the template uses version [2.5.3](https://github.com/coder/terraform-provider-coder/releases/tag/v2.5.3) or later of `coder/terraform-provider-coder`.

1. Select **Settings** > **Parameters** on the template, then clear the **Use parameter compatibility mode for workspace builds** checkbox.

1. Publish a new template version so Coder can resolve the parameter metadata.

   If you don't need to change the Terraform, [refresh the template data](../../managing-templates/index.md#refresh-template-data) to publish a new version from the existing source files.

1. Open the workspace creation form and confirm the parameters render as expected.

You can make the same change through the `use_classic_parameter_flow` field in the [templates API](../../../../reference/api/templates.md#update-template-settings-by-id).
The request only updates the fields you send, so you can set this field on its own:

```shell
curl -X PATCH "$CODER_URL/api/v2/templates/$TEMPLATE_ID" \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"use_classic_parameter_flow": false}'
```

If the form looks the same after the switch, refer to the [troubleshooting docs on parameters looking the same after you turn off compatibility mode](./troubleshooting.md#parameters-look-the-same-after-you-turn-off-compatibility-mode).

## Revert to compatibility mode

If a template does not work after the switch, you can opt that template back into the classic flow.
Select **Settings** > **Parameters** on the template, then select the **Use parameter compatibility mode for workspace builds** checkbox.
You can also set `use_classic_parameter_flow` to `true` through the [templates API](../../../../reference/api/templates.md#update-template-settings-by-id).

If your template's parameters do not work without compatibility mode, [file an issue](https://github.com/coder/coder/issues/new?labels=parameters) with the `parameters` label.
