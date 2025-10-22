# Provisioning with OpenTofu

<!-- Keeping this in as a placeholder for supporting OpenTofu. We should fix support for custom terraform binaries ASAP. -->

> [!IMPORTANT]
> This guide is a work in progress. We do not officially support using custom
> Terraform binaries in your Coder deployment. To track progress on the work,
> see this related [GitHub Issue](https://github.com/coder/coder/issues/12009).

Coder deployments support any custom Terraform binary, including
[OpenTofu](https://opentofu.org/docs/) - an open source alternative to
Terraform.

You can read more about OpenTofu and Hashicorp's licensing in our
[blog post](https://coder.com/blog/hashicorp-license) on the Terraform licensing changes.

## Using a custom Terraform binary

You can change your deployment custom Terraform binary as long as it is in
`PATH` and is within the
[supported versions](https://github.com/coder/coder/blob/f57ce97b5aadd825ddb9a9a129bb823a3725252b/provisioner/terraform/install.go#L22-L25).
The hardcoded version check ensures compatibility with our
[example templates](https://github.com/coder/coder/tree/main/examples/templates).
