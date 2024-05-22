#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

for d in */; do
	pushd "$d"
	name=$(basename "$(pwd)")

	# This needs care to update correctly.
	if [[ $name == "kubernetes-metadata" ]]; then
		popd
		continue
	fi

	# This directory is used for a different purpose (quick workaround).
	if [[ $name == "cleanup-stale-plugins" ]]; then
		popd
		continue
	fi

	terraform init -upgrade
	terraform plan -out terraform.tfplan
	terraform show -json ./terraform.tfplan | jq >"$name".tfplan.json
	terraform graph -type=plan >"$name".tfplan.dot
	rm terraform.tfplan
	terraform apply -auto-approve
	terraform show -json ./terraform.tfstate | jq >"$name".tfstate.json
	rm terraform.tfstate
	terraform graph -type=plan >"$name".tfstate.dot
	popd
done

terraform version -json | jq -r '.terraform_version' >version.txt
