#!/bin/bash

set -euo pipefail

if [[ $(id -u) -ne 0 ]]; then
	echo "Please run with sudo"
	exit 1
fi

to_delete=$(ip netns list | grep -o 'cdr_.*_.*' | cut -d' ' -f1)
echo "Will delete:"
for ns in $to_delete; do
	echo "- $ns"
done

read -p "Continue? [y/N] " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
	exit 1
fi

for ns in $to_delete; do
	ip netns delete "$ns"
done
