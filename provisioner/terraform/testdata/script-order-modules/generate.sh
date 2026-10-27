#!/usr/bin/env bash

set -euo pipefail

# Requires a coder provider with coder_script_order support. Arguments are
# forwarded to terraform init, e.g. -plugin-dir=/path/to/local/providers.
# The committed outputs use Terraform 1.16.6 and coder provider e851c57ccdf4.
fixture_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_workdir="$(mktemp -d)"
trap 'rm -rf "$fixture_workdir"' EXIT
cp "$fixture_dir/main.tf" "$fixture_workdir/"
cp -R "$fixture_dir/bootstrap" "$fixture_workdir/"
cd "$fixture_workdir"

terraform init -input=false "$@"
terraform plan -input=false -out=terraform.tfplan
terraform show -json terraform.tfplan | jq . >plan.json
terraform graph -type=plan >plan.dot
terraform apply -input=false -auto-approve terraform.tfplan
terraform show -json terraform.tfstate | jq . >state.json
terraform graph -type=plan >state.dot

# Publish the outputs only after every Terraform operation succeeds.
cp plan.json plan.dot state.json state.dot "$fixture_dir/"
