# Automation

All actions possible through the Coder dashboard can also be automated as it utilizes the same public REST API. There are several ways to extend/automate Coder:

- [CLI](../cli.md)
- [REST API](../api/)
- [Coder SDK](https://pkg.go.dev/github.com/coder/coder/codersdk)

## Quickstart

Generate a token on your Coder deployment by visiting:

```sh
https://coder.example.com/settings/tokens
```

List your workspaces

```sh
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

We publish an [API reference](../api/index.md) in our documentation. You can also enable a [Swagger endpoint](../cli/server.md#--swagger-enable) on your Coder deployment.

## Use cases

We strive to keep the following use cases up to date, but please note that changes to API queries and routes can occur. For the most recent queries and payloads, we recommend checking the CLI and API documentation.

### Templates

- [Update templates in CI](../templates/change-management.md): Store all templates and git and update templates in CI/CD pipelines.

### Workspace agents

Workspace agents have a special token that can send logs, metrics, and workspace activity.

- [Custom workspace logs](../api/agents.md#patch-workspace-agent-logs): Expose messages prior to the Coder init script running (e.g. pulling image, VM starting, restoring snapshot). [coder-logstream-kube](https://github.com/coder/coder-logstream-kube) uses this to show Kubernetes events, such as image pulls or ResourceQuota restrictions.

  ```sh
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

- [Manually send workspace activity](../api/agents.md#submit-workspace-agent-stats): Keep a workspace "active," even if there is not an open connection (e.g. for a long-running machine learning job).

  ```sh
  #!/bin/bash
  # Send workspace activity as long as the job is still running

  while true
  do
    if pgrep -f "my_training_script.py" > /dev/null
    then
      curl -X POST "https://coder.example.com/api/v2/workspaceagents/me/report-stats" \
      -H "Coder-Session-Token: $CODER_AGENT_TOKEN" \
      -d '{
        "connection_count": 1
      }'

      # Sleep for 30 minutes (1800 seconds) if the job is running
      sleep 1800
    else
      # Sleep for 1 minute (60 seconds) if the job is not running
      sleep 60
    fi
  done
  ```
