#!/usr/bin/env bash
# Copy only reviewed public inputs into a clean directory for template upload.
set -euo pipefail
umask 077

if [[ $# != 1 || $1 == -h || $1 == --help ]]; then
	printf 'Usage: ./prepare.sh /path/to/new-template-directory\n'
	exit 2
fi
source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
destination=$1
[[ ! -e $destination && ! -L $destination ]] || {
	printf 'Destination must not already exist.\n' >&2
	exit 1
}
mkdir -m 0700 -- "$destination"
for name in main.tf variables.tf bootstrap.sh prepare-state.sh prepare.sh cloud-config.yaml.tftpl launch-bootstrap.tftpl README.md .gitignore; do
	[[ -f $source_dir/$name && ! -L $source_dir/$name ]] || {
		printf 'Missing regular recipe input: %s\n' "$name" >&2
		exit 1
	}
	cp -- "$source_dir/$name" "$destination/$name"
done
if [[ -f $source_dir/.terraform.lock.hcl && ! -L $source_dir/.terraform.lock.hcl ]]; then
	cp -- "$source_dir/.terraform.lock.hcl" "$destination/.terraform.lock.hcl"
fi
printf 'Prepared public template inputs in %s. Pass configuration with coder templates push --variable; keep credentials on the provisioner.\n' "$destination"
