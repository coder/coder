# JFrog Xray Integration

JFrog Xray is a security and compliance scanning tool that analyzes container images and other artifacts for vulnerabilities, license compliance, and policy violations. This guide shows how to integrate Xray vulnerability scanning results into your Coder workspace metadata.

## Overview

Coder provides two approaches for integrating with JFrog Xray:

1. **Terraform Module (Recommended)**: Uses the `jfrog-xray` module from the Coder registry to display vulnerability counts directly in workspace metadata
2. **External Service**: Uses the `coder-xray` utility for Kubernetes-based workspaces

This guide focuses on the Terraform module approach, which offers several advantages:

- Works with all workspace types (not just Kubernetes)
- No additional service deployment required
- Real-time vulnerability information during workspace provisioning
- Native integration with Terraform templates

## Prerequisites

- **JFrog Artifactory**: Container images must be stored in JFrog Artifactory
- **JFrog Xray**: Xray must be configured to scan your repositories
- **Access Token**: Valid JFrog access token with Xray read permissions
- **Scanned Images**: Images must have been scanned by Xray

## Setup

### 1. Configure JFrog Xray

Ensure your JFrog Xray instance is configured to scan the repositories containing your workspace images:

1. **Create Xray Policies**: Define security policies for vulnerability scanning
2. **Configure Watches**: Set up watches to monitor your Docker repositories
3. **Verify Scans**: Ensure your container images are being scanned

### 2. Generate Access Token

Create a JFrog access token with Xray read permissions:

1. Log into your JFrog platform
2. Go to **Administration** → **User Management** → **Access Tokens**
3. Create a new token with the following scopes:
   - `applied-permissions/groups:readers`
   - `applied-permissions/groups:xray-readers`

### 3. Add Module to Workspace Template

Add the JFrog Xray module to your workspace template:

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

variable "jfrog_access_token" {
  description = "JFrog access token for Xray API"
  type        = string
  sensitive   = true
}

data "coder_workspace" "me" {}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "example.jfrog.io/docker-local/codercom/enterprise-base:latest"
  name  = "coder-${data.coder_workspace.me.owner}-${data.coder_workspace.me.name}"
  
  # Container configuration...
}

# Add Xray vulnerability scanning
module "jfrog_xray" {
  source      = "registry.coder.com/modules/jfrog-xray/coder"
  version     = "1.0.0"
  
  resource_id = docker_container.workspace[0].id
  xray_url    = "https://example.jfrog.io/xray"
  xray_token  = var.jfrog_access_token
  image       = "docker-local/codercom/enterprise-base:latest"
}
```

### 4. Configure Template Variables

When creating or updating your template, provide the JFrog access token:

```bash
coder templates push mytemplate \
  --variable jfrog_access_token="your-access-token-here"
```

Alternatively, use environment variables or external secret management:

```bash
export TF_VAR_jfrog_access_token="your-access-token-here"
coder templates push mytemplate
```

## Module Configuration

The `jfrog-xray` module supports several configuration options:

### Required Variables

| Variable      | Description                       | Example                            |
|---------------|-----------------------------------|------------------------------------|
| `resource_id` | Resource ID to attach metadata to | `docker_container.workspace[0].id` |
| `xray_url`    | JFrog Xray instance URL           | `https://example.jfrog.io/xray`    |
| `xray_token`  | JFrog access token                | `var.jfrog_access_token`           |
| `image`       | Container image to scan           | `docker-local/myapp:latest`        |

### Optional Variables

| Variable       | Description                        | Default                    |
|----------------|------------------------------------|----------------------------|
| `repo`         | Artifactory repository name        | Auto-extracted from image  |
| `repo_path`    | Repository path with image and tag | Auto-extracted from image  |
| `display_name` | Metadata section display name      | "Security Vulnerabilities" |
| `icon`         | Metadata section icon              | "/icon/security.svg"       |

### Advanced Configuration

```hcl
module "jfrog_xray" {
  source      = "registry.coder.com/modules/jfrog-xray/coder"
  version     = "1.0.0"
  
  resource_id  = docker_container.workspace[0].id
  xray_url     = "https://example.jfrog.io/xray"
  xray_token   = var.jfrog_access_token
  
  # Specify repo and path separately for more control
  repo         = "docker-local"
  repo_path    = "/codercom/enterprise-base:v2.1.0"
  
  display_name = "Container Security Scan"
  icon         = "/icon/shield.svg"
}
```

## Workspace Display

Once configured, vulnerability information appears in the workspace metadata:

![Xray Vulnerability Display](../images/guides/xray-integration/vulnerability-display.png)

The metadata shows:

- **Image**: The scanned container image
- **Total Vulnerabilities**: Total count of all vulnerabilities
- **Critical**: Count of critical severity vulnerabilities
- **High**: Count of high severity vulnerabilities
- **Medium**: Count of medium severity vulnerabilities
- **Low**: Count of low severity vulnerabilities

## Multiple Images

For workspaces using multiple container images, add a separate module block for each image:

```hcl
# Scan main workspace image
module "xray_workspace" {
  source      = "registry.coder.com/modules/jfrog-xray/coder"
  version     = "1.0.0"
  
  resource_id  = docker_container.workspace[0].id
  xray_url     = var.jfrog_xray_url
  xray_token   = var.jfrog_access_token
  image        = "docker-local/workspace:latest"
  display_name = "Workspace Security"
}

# Scan database image
module "xray_database" {
  source      = "registry.coder.com/modules/jfrog-xray/coder"
  version     = "1.0.0"
  
  resource_id  = docker_container.database[0].id
  xray_url     = var.jfrog_xray_url
  xray_token   = var.jfrog_access_token
  image        = "docker-local/postgres:14"
  display_name = "Database Security"
}
```

## Troubleshooting

### Common Issues

#### "No scan results found"

- Verify the image exists in Artifactory
- Check that Xray has scanned the image
- Confirm the image path format is correct
- Review Xray watch configuration

#### "Authentication failed"

- Verify the access token is valid and not expired
- Check token permissions include Xray read access
- Ensure the Xray URL is correct and accessible

#### "Module fails to apply"

- Verify network connectivity from Coder to JFrog instance
- Check Terraform provider versions are compatible
- Review Coder logs for detailed error messages
- Ensure the Xray Terraform provider is available

### Debugging

Enable detailed Terraform logging to troubleshoot issues:

```bash
export TF_LOG=DEBUG
coder templates plan <template-name>
```

Check Coder provisioner logs:

```bash
coder server logs --follow
```

### Network Requirements

Ensure Coder can reach your JFrog instance:

- **Outbound HTTPS (443)**: For API communication
- **DNS Resolution**: JFrog hostname must be resolvable
- **Firewall Rules**: Allow traffic from Coder to JFrog

## Security Considerations

### Token Management

- **Use Terraform Variables**: Never hardcode tokens in templates
- **External Secrets**: Consider using HashiCorp Vault or similar
- **Token Rotation**: Regularly rotate access tokens
- **Minimal Permissions**: Grant only necessary Xray read permissions

### Network Security

- **TLS/HTTPS**: Always use encrypted connections
- **Network Segmentation**: Restrict network access where possible
- **VPN/Private Networks**: Use private connectivity when available

## Alternative: External Service Approach

For Kubernetes-based workspaces, you can also use the external `coder-xray` service:

1. **Deploy Service**: Install `coder-xray` in your Kubernetes cluster
2. **Configure Scanning**: Set up namespace-based scanning
3. **View Results**: Results appear in the Coder dashboard

See the [coder-xray repository](https://github.com/coder/coder-xray) for detailed setup instructions.

## Related Resources

- [JFrog Artifactory Integration](./jfrog-artifactory.md)
- [Coder Metadata Resource](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/metadata)
- [JFrog Xray Terraform Provider](https://registry.terraform.io/providers/jfrog/xray/latest)
- [JFrog Xray Documentation](https://jfrog.com/help/r/jfrog-security-documentation)
