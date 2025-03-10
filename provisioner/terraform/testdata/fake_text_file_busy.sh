#!/bin/sh

VERSION=$1
shift 1

# json_print() {
# 	echo "{\"@level\":\"error\",\"@message\":\"$*\"}"
# }

case "$1" in
version)
	cat <<-EOF
		{
			"terraform_version": "${VERSION}",
			"platform": "linux_amd64",
			"provider_selections": {},
			"terraform_outdated": false
		}
	EOF
	exit 0
	;;
init)
	echo "init"
	echo >&2 "Error: Failed to install provider"
	echo >&2 "    Error while installing coder/coder v1.0.4: open"
	echo >&2 "    /home/coder/.cache/coder/provisioner-0/tf/registry.terraform.io/coder/coder/1.0.3/linux_amd64/terraform-provider-coder_v1.0.4:"
	echo >&2 "    text file busy"
	exit 1
	;;
plan)
	echo "plan not supported"
	exit 1
	;;
apply)
	echo "apply not supported"
	exit 1
	;;
esac

exit 10
