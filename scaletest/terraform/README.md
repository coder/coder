# Load Test Terraform

This folder contains Terraform code and scripts to aid in performing load tests of Coder.
It does the following:

- Creates a GCP VPC.
- Creates a CloudSQL instance with a global peering rule so it's accessible inside the VPC.
- Creates a GKE cluster inside the VPC with separate nodegroups for Coder and workspaces.
- Installs Coder in a new namespace, using the CloudSQL instance.

## Usage

> You must have an existing Google Cloud project available.

1. Create a file named `override.tfvars` with the following content, modifying as appropriate:

```terraform
name = "some_unique_identifier"
project_id = "some_google_project_id"
```

1. Inspect `vars.tf` and override any other variables you deem necessary.

1. Run `terraform init`.

1. Run `terraform plan -var-file=override.tfvars` and inspect the output.
   If you are not satisfied, modify `override.tfvars` until you are.

1. Run `terraform apply -var-file=override.tfvars`. This will spin up a pre-configured environment
   and emit the Coder URL as an output.

1. Run `coder_init.sh <coder_url>` to setup an initial user and a pre-configured Kubernetes
   template. It will also download the Coder CLI from the Coder instance locally.

1. Do whatever you need to do with the Coder instance.

   > To run Coder commands against the instance, you can use `coder_shim.sh <command>`.
   > You don't need to run `coder login` yourself.

1. When you are finished, you can run `terraform destroy -var-file=override.tfvars`.
