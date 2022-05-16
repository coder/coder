#!/usr/bin/env bash

# Usage:
# ./create_migration name of migration
# ./create_migration "name of migration"
# ./create_migration name_of_migration

set -euo pipefail

cd "$(dirname "$0")"

# if migration name is an empty string exit
[[ -z "${*}" ]] && (echo "Must provide a migration name" && exit 1)

# " " && "-" -> "_"
title="$(echo "${@}" | tr "[:upper:]" "[:lower:]" | sed -E -e "s/( |-)/_/g")"

migrate create -ext sql -dir . -seq "$title"

echo "Run \"make gen\" to generate models."
