#!/usr/bin/env bash

# This script lists all dependencies of a given package, including dependencies
# of test files.

# Usage: list_dependencies <package>

set -euo pipefail

if [[ "$#" -ne 1 ]]; then
	echo "Usage: $0 <package>"
	exit 1
fi

package="$1"
all_deps=$(go list -f '{{join .Deps "\n"}}' "$package")
test_imports=$(go list -f '{{ join .TestImports " " }}' "$package")
xtest_imports=$(go list -f '{{ join .XTestImports " " }}' "$package")

for pkg in $test_imports $xtest_imports; do
	deps=$(go list -f '{{join .Deps "\n"}}' "$pkg")
	all_deps+=$'\n'"$deps"
done

echo "$all_deps" | sort | uniq
