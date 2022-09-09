#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

for d in */; do
	pushd "$d"
	name=$(basename "$(pwd)")
	terraform init -upgrade
	terraform plan -out terraform.tfplan
	terraform show -json ./terraform.tfplan | jq >"$name".tfplan.json
	terraform graph >"$name".tfplan.dot
	rm terraform.tfplan
	terraform apply -auto-approve
	terraform show -json ./terraform.tfstate | jq >"$name".tfstate.json
	rm terraform.tfstate
	terraform graph >"$name".tfstate.dot
	popd
done
