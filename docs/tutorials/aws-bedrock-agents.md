# Configure AWS Bedrock for Coder Agents

<div>
  <a href="https://github.com/coder" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Coder</span>
  </a>
</div>
March 2026

---

Enterprise teams running Coder often need to route AI agent traffic through AWS
Bedrock to satisfy compliance, data residency, and billing requirements. This
tutorial shows how to configure AWS Bedrock as the LLM provider for coding
agents (such as Claude Code) running in Coder workspaces. You can either
configure Bedrock credentials directly on the workspace agent or use Coder's AI
Bridge as a centralized LLM gateway.

## Prerequisites

- A running Coder deployment (v2.21 or later)
- An AWS account with Amazon Bedrock access
- AWS CLI installed and configured (optional, for credential setup)
- Admin access to create or edit Coder templates
- For AI Bridge: A Premium license with the AI Governance Add-On

## 1. Enable model access in AWS Bedrock

Amazon Bedrock now provides automatic access to most serverless foundation
models, eliminating the previous requirement for manual enablement of each
model. However, Anthropic models still require a one-time use case submission.

1. Sign in to the AWS Console and navigate to **Amazon Bedrock** > **Model
   catalog**.
2. Search for the Claude model you want to use (e.g., Claude Sonnet 4).
3. For Anthropic models, click **Request model access** and submit a brief use
   case description (e.g., "AI-assisted software development using Claude
   Code"). Access is typically granted immediately.

<!-- TODO: Screenshot of AWS Bedrock Model catalog showing Claude models -->

![Bedrock Model Catalog](../images/guides/aws-bedrock-agents/bedrock-model-catalog.png)

> [!NOTE]
> Most Amazon, Meta, and Mistral models are available immediately without
> approval. Anthropic models require the one-time use case submission. Check
> [AWS Bedrock model access](https://docs.aws.amazon.com/bedrock/latest/userguide/model-access.html)
> for the latest requirements.

## 2. Create an IAM policy for Bedrock access

Create an IAM policy that grants the minimum permissions needed to invoke
Bedrock models. This follows the principle of least privilege.

Create the following IAM policy in the AWS Console under **IAM** > **Policies**
> **Create policy**:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "BedrockInvokeModels",
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream"
      ],
      "Resource": "*"
    },
    {
      "Sid": "BedrockListModels",
      "Effect": "Allow",
      "Action": [
        "bedrock:ListFoundationModels",
        "bedrock:ListInferenceProfiles",
        "bedrock:GetFoundationModel"
      ],
      "Resource": "*"
    }
  ]
}
```

> [!TIP]
> For production, scope the `Resource` field to specific model ARNs. For
> example, to restrict access to only Claude Sonnet 4:
> `"Resource": "arn:aws:bedrock:*::foundation-model/anthropic.claude-sonnet-4*"`

## 3. Set up AWS credentials

You have several options for providing AWS credentials to your Coder workspaces.
Choose the method that best fits your organization's security requirements.

### Option A: Bedrock API keys (simplest)

AWS Bedrock provides the ability to generate API keys directly from the Bedrock
console. This is the simplest approach for getting started.

1. Navigate to **Amazon Bedrock** > **API keys** in the AWS Console.
2. Click **Generate** and select an expiry period.
3. Save the generated API key securely.

Use the `AWS_BEARER_TOKEN_BEDROCK` environment variable in your template:

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    CLAUDE_CODE_USE_BEDROCK    = "1"
    AWS_REGION                 = "us-east-1"
    AWS_BEARER_TOKEN_BEDROCK   = var.bedrock_api_key
  }
}
```

### Option B: IAM access keys

Create an IAM user with the Bedrock policy attached, then generate access keys.

1. In the AWS Console, go to **IAM** > **Users** > **Create user**.
2. Attach the Bedrock policy you created in Step 2.
3. Under **Security credentials**, create an access key and select **Application
   running outside AWS**.

```hcl
resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    CLAUDE_CODE_USE_BEDROCK  = "1"
    AWS_REGION               = "us-east-1"
    AWS_ACCESS_KEY_ID        = var.aws_access_key_id
    AWS_SECRET_ACCESS_KEY    = var.aws_secret_access_key
  }
}
```

### Option C: IAM role (recommended for production)

If your Coder workspaces run on AWS infrastructure (EKS, EC2), you can use IAM
roles for service accounts (IRSA) or instance profiles to avoid managing static
credentials entirely.

For EKS-based workspaces, annotate the workspace service account with the IAM
role:

```hcl
resource "kubernetes_service_account" "workspace" {
  metadata {
    name      = "coder-workspace"
    namespace = "coder"
    annotations = {
      "eks.amazonaws.com/role-arn" = "arn:aws:iam::123456789012:role/coder-bedrock-role"
    }
  }
}
```

<!-- TODO: Screenshot of IAM user creation or role setup -->

![IAM Setup](../images/guides/aws-bedrock-agents/iam-setup.png)

## 4. Configure Claude Code with Bedrock in your template

Now configure your Coder template to use Claude Code with AWS Bedrock. The key
is setting the `CLAUDE_CODE_USE_BEDROCK` environment variable and providing AWS
credentials.

Here is a complete template example using the Claude Code module with Bedrock:

```hcl
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.13"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

variable "bedrock_api_key" {
  type        = string
  sensitive   = true
  description = "AWS Bedrock API key"
}

variable "aws_region" {
  type        = string
  default     = "us-east-1"
  description = "AWS region for Bedrock"
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    CLAUDE_CODE_USE_BEDROCK          = "1"
    AWS_REGION                       = var.aws_region
    AWS_BEARER_TOKEN_BEDROCK         = var.bedrock_api_key
    ANTHROPIC_DEFAULT_SONNET_MODEL   = "us.anthropic.claude-sonnet-4-6"
    ANTHROPIC_DEFAULT_HAIKU_MODEL    = "us.anthropic.claude-haiku-4-5-20251001-v1:0"
  }
}

resource "coder_ai_task" "task" {
  count  = data.coder_workspace.me.start_count
  app_id = module.claude-code[count.index].task_app_id
}

data "coder_task" "me" {}

module "claude-code" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/claude-code/coder"
  version  = "4.8.1"
  agent_id = coder_agent.main.id
  workdir  = "/home/coder/project"

  claude_api_key  = "bedrock"   # Placeholder; auth is via env vars above
  ai_prompt       = data.coder_task.me.prompt
  model           = "sonnet"
  permission_mode = "plan"
}
```

> [!IMPORTANT]
> Always pin specific model versions in production deployments. If you use model
> aliases like `sonnet` without pinning via `ANTHROPIC_DEFAULT_SONNET_MODEL`,
> Claude Code may attempt to use a newer model version that isn't available in
> your Bedrock account.

> [!NOTE]
> `AWS_REGION` is a required environment variable when using Bedrock. Claude Code
> does not read from the `.aws` config file for the region setting.

## 5. (Alternative) Use AI Bridge for centralized Bedrock access

If you have a Coder Premium license with the AI Governance Add-On, you can use
AI Bridge as a centralized LLM gateway. This eliminates the need to distribute
AWS credentials to individual workspaces and provides audit trails, token usage
tracking, and centralized MCP administration.

### Enable AI Bridge

```sh
export CODER_AIBRIDGE_ENABLED=true
coder server
```

### Configure the Bedrock provider

Set the following environment variables on the Coder server:

```sh
export CODER_AIBRIDGE_BEDROCK_REGION=us-east-1
export CODER_AIBRIDGE_BEDROCK_ACCESS_KEY=<your-access-key-id>
export CODER_AIBRIDGE_BEDROCK_ACCESS_KEY_SECRET=<your-secret-access-key>
export CODER_AIBRIDGE_BEDROCK_MODEL=us.anthropic.claude-sonnet-4-6
export CODER_AIBRIDGE_BEDROCK_SMALL_FAST_MODEL=us.anthropic.claude-haiku-4-5-20251001-v1:0
coder server
```

> [!NOTE]
> You can use `CODER_AIBRIDGE_BEDROCK_BASE_URL` instead of
> `CODER_AIBRIDGE_BEDROCK_REGION` if you need to specify a custom endpoint, such
> as a proxy between AI Bridge and AWS Bedrock.

### Configure your template to use AI Bridge

Instead of setting AWS credentials on each workspace, point Claude Code at the
AI Bridge endpoint:

```hcl
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  os   = "linux"
  arch = "amd64"

  env = {
    ANTHROPIC_BASE_URL  = "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic"
    ANTHROPIC_API_KEY   = data.coder_workspace_owner.me.session_token
  }
}
```

<!-- TODO: Screenshot of AI Bridge configuration in Coder dashboard -->

![AI Bridge Setup](../images/guides/aws-bedrock-agents/ai-bridge-setup.png)

## 6. Verify the configuration

After pushing your template, create a workspace and verify that the agent can
communicate with Bedrock.

1. Push the template to your Coder deployment:

   ```console
   coder template push my-bedrock-template -d . \
     --variable bedrock_api_key="your-api-key"
   ```

2. Create a new task or workspace from the template.

3. In the task chat or terminal, send a test prompt like "Hello, what model are
   you using?"

4. Verify that the agent responds and check the task status in the Coder
   dashboard.

<!-- TODO: Screenshot of a successful task running with Bedrock -->

![Successful Task](../images/guides/aws-bedrock-agents/task-running.png)

> [!TIP]
> If using AI Bridge, you can verify the connection by checking the AI Bridge
> audit logs in the Coder dashboard under **Deployment** > **AI Bridge**.

## Troubleshooting

### "Invocation of model ID ... with on-demand throughput isn't supported"

AWS Bedrock requires inference profiles for on-demand usage of newer models. Use
a region-prefixed model ID like `us.anthropic.claude-sonnet-4-6` instead of the
base model ID.

### "Unexpected value(s) for the anthropic-beta header"

Set `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` in your agent environment
variables to disable experimental beta headers that may not be supported by your
Bedrock configuration.

### Credentials not working

- Ensure `AWS_REGION` is set as an environment variable. Claude Code does not
  read from `~/.aws/config`.
- If using IAM access keys, verify the user has the Bedrock policy attached.
- If using IRSA, ensure the service account annotation is correct and the trust
  relationship is configured.

### Model not available

- Check that the model is available in your chosen AWS region. Not all models
  are available in every region.
- For Anthropic models, verify you completed the one-time use case submission.

## Next steps

- [Coder Tasks documentation](../ai-coder/tasks.md) for more on running coding
  agents
- [AI Bridge setup](../ai-coder/ai-bridge/setup.md) for centralized LLM gateway
  configuration
- [Agent Boundaries](../ai-coder/agent-boundaries/index.md) for process-level
  safeguards
- [Claude Code on Amazon Bedrock](https://code.claude.com/docs/en/amazon-bedrock)
  for agent-specific configuration
- [Security & Boundaries](../ai-coder/security.md) for best practices on
  securing agent workloads
