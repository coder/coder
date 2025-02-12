#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

generate() {
	local name="$1"

	echo "=== BEGIN: $name"
	terraform init -upgrade &&
		terraform plan -out terraform.tfplan &&
		terraform show -json ./terraform.tfplan | jq >"$name".tfplan.json &&
		terraform graph -type=plan >"$name".tfplan.dot &&
		rm terraform.tfplan &&
		terraform apply -auto-approve &&
		terraform show -json ./terraform.tfstate | jq >"$name".tfstate.json &&
		rm terraform.tfstate &&
		terraform graph -type=plan >"$name".tfstate.dot
	ret=$?
	echo "=== END: $name"
	if [[ $ret -ne 0 ]]; then
		return $ret
	fi
}

run() {
	d="$1"
	cd "$d"
	name=$(basename "$(pwd)")

	# This needs care to update correctly.
	if [[ $name == "kubernetes-metadata" ]]; then
		echo "== Skipping: $name"
		return 0
	fi

	# This directory is used for a different purpose (quick workaround).
	if [[ $name == "cleanup-stale-plugins" ]]; then
		echo "== Skipping: $name"
		return 0
	fi

	if [[ $name == "timings-aggregation" ]]; then
		echo "== Skipping: $name"
		return 0
	fi

	echo "== Generating test data for: $name"
	if ! out="$(generate "$name" 2>&1)"; then
		echo "$out"
		echo "== Error generating test data for: $name"
		return 1
	fi
	echo "== Done generating test data for: $name"
	exit 0
}

if [[ " $* " == *" --help "* || " $* " == *" -h "* ]]; then
	echo "Usage: $0 [module1 module2 ...]"
	exit 0
fi

declare -a jobs=()
if [[ $# -gt 0 ]]; then
	for d in "$@"; do
		run "$d" &
		jobs+=($!)
	done
else
	for d in */; do
		run "$d" &
		jobs+=($!)
	done
fi

err=0
for job in "${jobs[@]}"; do
	if ! wait "$job"; then
		err=$((err + 1))
	fi
done
if [[ $err -ne 0 ]]; then
	echo "ERROR: Failed to generate test data for $err modules"
	exit 1
fi

terraform version -json | jq -r '.terraform_version' >version.txt
