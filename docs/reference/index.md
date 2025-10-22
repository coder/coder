# Reference

## Automation

All actions possible through the Coder dashboard can also be automated. There
are several ways to extend/automate Coder:

- [coderd Terraform Provider](https://registry.terraform.io/providers/coder/coderd/latest)
- [CLI](../reference/cli/index.md)
- [REST API](../reference/api/index.md)
- [Coder SDK](https://pkg.go.dev/github.com/coder/coder/v2/codersdk)
- [Agent API](../reference/agent-api/index.md)

## Quickstart

Generate a token on your Coder deployment by visiting:

```shell
https://coder.example.com/settings/tokens
```

List your workspaces

```shell
# CLI
coder ls \
  --url https://coder.example.com \
  --token <your-token> \
  --output json

# REST API (with curl)
curl https://coder.example.com/api/v2/workspaces?q=owner:me \
  -H "Coder-Session-Token: <your-token>"
```

## Documentation

We publish an [API reference](../reference/api/index.md) in our documentation.
You can also enable a
[Swagger endpoint](../reference/cli/server.md#--swagger-enable) on your Coder
deployment.

## Use cases

We strive to keep the following use cases up to date, but please note that
changes to API queries and routes can occur. For the most recent queries and
payloads, we recommend checking the relevant documentation.

### Users & Groups

- [Manage Users via Terraform](https://registry.terraform.io/providers/coder/coderd/latest/docs/resources/user)
- [Manage Groups via Terraform](https://registry.terraform.io/providers/coder/coderd/latest/docs/resources/group)

### Templates

- [Manage templates via Terraform or CLI](../admin/templates/managing-templates/change-management.md):
  Store all templates in git and update them in CI/CD pipelines.

### Workspace agents

Workspace agents have a special token that can send logs, metrics, and workspace
activity.

- [Custom workspace logs](../reference/api/agents.md#patch-workspace-agent-logs):
  Expose messages prior to the Coder init script running (e.g. pulling image, VM
  starting, restoring snapshot).
  [coder-logstream-kube](https://github.com/coder/coder-logstream-kube) uses
  this to show Kubernetes events, such as image pulls or ResourceQuota
  restrictions.

  ```shell
  curl -X PATCH https://coder.example.com/api/v2/workspaceagents/me/logs \
  -H "Coder-Session-Token: $CODER_AGENT_TOKEN" \
  -d "{
  \"logs\": [
    {
      \"created_at\": \"$(date -u +'%Y-%m-%dT%H:%M:%SZ')\",
      \"level\": \"info\",
      \"output\": \"Restoring workspace from snapshot: 05%...\"
    }
  ]
  }"
  ```

- [Manually send workspace activity](../reference/api/workspaces.md#extend-workspace-deadline-by-id):
  Keep a workspace "active," even if there is not an open connection (e.g. for a
  long-running machine learning job).

  ```shell
  #!/bin/bash
  # Send workspace activity as long as the job is still running

  while true
  do
  if pgrep -f "my_training_script.py" > /dev/null
  then
    curl -X PUT "https://coder.example.com/api/v2/workspaces/$WORKSPACE_ID/extend" \
    -H "Coder-Session-Token: $CODER_AGENT_TOKEN" \
    -d '{
      "deadline": "2019-08-24T14:15:22Z"
    }'

    # Sleep for 30 minutes (1800 seconds) if the job is running
    sleep 1800
  else
    # Sleep for 1 minute (60 seconds) if the job is not running
    sleep 60
  fi
  done
  ```
