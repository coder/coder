# Claude Desktop

Claude Desktop (Cowork and Code tabs) can route all model inference through
AI Gateway using its third-party inference mode. This mode is configured via
the built-in setup UI or through MDM for fleet-wide deployment.

> [!NOTE]
> Cowork on third-party platforms is a
> [Research Preview](https://claude.com/docs/cowork/3p/overview).
> Features and configuration keys may change.

## Prerequisites

- Claude Desktop installed ([download](https://claude.com/download))
- A **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)** for authentication with AI Gateway

## Centralized API Key

### Single-machine setup (Developer Mode)

1. Open Claude Desktop (you do not need to sign in).
1. Go to **Help** > **Troubleshooting** > **Enable Developer Mode**.
1. Go to **Developer** > **Configure Third-Party Inference**.
1. In the **Connection** section, configure:
   - **Inference provider**: Select **Gateway (Anthropic-compatible)**.
   - **Gateway base URL**: Enter
     `https://coder.example.com/api/v2/aibridge/anthropic`.
   - **Gateway auth scheme**: Leave as **bearer**.
   - **Gateway API key**: Enter your
     **[Coder API token](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)**.
1. In the **Models** section, add the models you want to use
   (for example `claude-sonnet-4-6`, `claude-opus-4-7`).
1. Click **Apply locally**, then quit and reopen Claude Desktop.
1. On the sign-in screen, choose **Continue with Gateway**.

### MDM deployment

For fleet-wide rollout, distribute a managed configuration profile via
Jamf, Intune, or Group Policy. The key fields are:

```json
{
  "inferenceProvider": "gateway",
  "inferenceGatewayBaseUrl": "https://coder.example.com/api/v2/aibridge/anthropic",
  "inferenceGatewayApiKey": "<coder-api-token>",
  "inferenceGatewayAuthScheme": "bearer",
  "inferenceModels": ["claude-sonnet-4-6", "claude-opus-4-7", "claude-haiku-4-5"]
}
```

Replace `coder.example.com` with your Coder deployment URL.

You can export a ready-to-deploy profile from the setup UI:

- **macOS**: Click **Export** to save a `.mobileconfig` file for Jamf or
  Kandji.
- **Windows**: Click **Export** to save a `.reg` file for Intune or Group
  Policy.

When the app launches and finds a managed configuration, it enters
third-party mode automatically without requiring users to sign in.

## Limitations

- **BYOK is not supported.** Claude Desktop's gateway mode sends a single
  credential (`inferenceGatewayApiKey`) on every request. There is no
  built-in mechanism to separate the gateway authentication token from a
  personal provider API key, so Bring Your Own Key mode is not available
  through this client.
- **Claude Code CLI configuration is separate.** Claude Desktop and the
  Claude Code CLI read their own configuration independently. If you also
  use Claude Code in the terminal, configure it separately with
  [environment variables](./claude-code.md).

**References:**
[Cowork on third-party platforms](https://claude.com/docs/cowork/3p/overview),
[Install and configure](https://support.claude.com/en/articles/14680741-install-and-configure-claude-cowork-with-third-party-platforms)
