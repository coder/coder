# Secrets

<blockquote class="info">
This article explains how to use secrets in a workspace. To authenticate the
workspace provisioner, see <a href="./templates/authentication">this</a>.
</blockquote>

Coder takes an unopinionated stance to workspace secrets.

## Wait a minute...

Your first stab at secrets with Coder should be your local method.
You can do everything you can locally and more with your Coder workspace, so
whatever workflow and tools you already use to manage secrets can be brought
over.

For most, this workflow is simply:

1. Give your users their secrets in advance
1. They write them to a persistent file after
   they've built a workspace

<a href="./templates#parameters">Template parameters</a> are a dangerous way to accept secrets.
We show parameters in cleartext around the product. Assume anyone with view
access to your workspace can also see parameters.

## Dynamic Secrets

Dynamic secrets are attached to the workspace lifecycle and require no setup by
the end user.

They can be implemented in native Terraform like so:

```hcl
resource "twilio_iam_api_key" "api_key" {
  account_sid   = "ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
  friendly_name = "Test API Key"
}

resource "coder_agent" "dev" {
  # ...
  env = {
    # Let users access the secret via #TWILIO_API_SECRET
    TWILIO_API_SECRET = "${twilio_iam_api_key.api_key.secret}"
  }
}
```

This method is limited to [services with Terraform providers](https://registry.terraform.io/browse/providers).

A catch-all variation of this approach is dynamically provisioning a cloud service account (e.g [GCP](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/google_service_account_key#private_key))
for each workspace and then make the relevant secrets available via the cloud's secret management
system.

## Coder SSH Key

Coder automatically inserts an account-wide SSH key into each workspace. In MacOS
and Linux this key is at `~/.ssh/id_ecdsa`. You can view and
regenerate the key in the dashboard at Settings > SSH keys.
