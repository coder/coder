#!/usr/bin/env bash
# Run through Coder SSH after readiness, separately from the first-command timer.
set -euo pipefail

smoke_dir=$(mktemp -d /workspace/smoke.XXXXXX)
trap 'rm -rf "$smoke_dir"' EXIT
cd "$smoke_dir"

git init --quiet source
cd source
git config user.email sandbox-test@example.invalid
git config user.name Sandbox
cat >go.mod <<'EOF'
module sandbox-smoke

go 1.26.0
EOF
cat >main.go <<'EOF'
package main

func add(a, b int) int { return a + b }
func main() {}
EOF
cat >main_test.go <<'EOF'
package main

import "testing"

func TestAdd(t *testing.T) {
	t.Parallel()
	if add(2, 3) != 5 {
		t.Fatal("unexpected sum")
	}
}
EOF
git add .
git commit --quiet -m 'smoke fixture'
cd ..
git clone --quiet source clone
cd clone
go test ./...
go build ./...
printf '#include <stdio.h>\nint main(void) { puts("compiler OK"); }\n' >compiler.c
cc compiler.c -o compiler
./compiler
node -e 'require("assert").strictEqual(2 + 3, 5)'
python3 -c 'assert 2 + 3 == 5'
test -w /workspace
test ! -S /run/containerd/containerd.sock
printf 'Sandbox smoke passed\n'
