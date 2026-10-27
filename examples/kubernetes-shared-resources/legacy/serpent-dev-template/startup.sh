#!/usr/bin/env bash
set -euo pipefail

export HOME=/root
export GOPATH="$HOME/go"
export GOCACHE=/workspace/cache/go-build
export GOLANGCI_LINT_CACHE=/workspace/cache/golangci-lint
export PATH="$GOPATH/bin:/usr/local/go/bin:$PATH"
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=local
export GOFLAGS=-p=2
export GOMAXPROCS=2
export GOMEMLIMIT=768MiB
export GIT_TERMINAL_PROMPT=0

mkdir -p /workspace/cache/go "$GOCACHE" "$GOLANGCI_LINT_CACHE"
# The upstream Makefile invokes ~/go/bin/golangci-lint directly.
if [[ -L "$GOPATH" && "$(readlink -- "$GOPATH")" == /workspace/cache/go ]]; then
  :
elif [[ -e "$GOPATH" || -L "$GOPATH" ]]; then
  printf 'Refusing to replace existing %s; expected a link to /workspace/cache/go.\n' "$GOPATH" >&2
  exit 1
else
  ln -s /workspace/cache/go "$GOPATH"
fi

# Debian login shells may replace the image PATH before reading .bashrc.
if ! grep -q '# serpent-dev PATH' "$HOME/.bashrc" 2>/dev/null; then
  cat >> "$HOME/.bashrc" <<'PROFILE'
# serpent-dev PATH
export PATH="$HOME/go/bin:/usr/local/go/bin:$PATH"
PROFILE
fi

ref=40b60bf4c1495caddc0292f862de275da292e6aa
repository=/workspace/serpent
clone_dir="/workspace/.serpent-clone-$$"
mkdir -p /workspace
trap 'status=$?; rm -rf -- "$clone_dir"; if (( status != 0 )); then printf "\nSerpent setup failed. Check the startup log and dedicated egress policy, then restart the workspace.\n" >&2; fi' EXIT

printf '\n🐍 Welcome to serpent dev!\n'
printf 'Your checkout, Go tools, and caches persist across workspace stops.\n'
printf 'Go, Bash, make, Git, curl, and GCC are supplied by the pinned public image.\n'
go version
make --version | sed -n '1p'

if [[ -d "$repository/.git" ]]; then
  printf 'Keeping your existing checkout and local changes.\n'
elif [[ -e "$repository" ]]; then
  printf 'Refusing to overwrite %s because it is not a Git checkout.\n' "$repository" >&2
  exit 1
else
  printf '\nCloning github.com/coder/serpent over HTTPS...\n'
  git -c credential.helper= clone --no-checkout --depth 1 \
    https://github.com/coder/serpent.git "$clone_dir"
  git -C "$clone_dir" -c credential.helper= fetch --depth 1 origin "$ref"
  git -C "$clone_dir" checkout -b demo "$ref"
  mv -- "$clone_dir" "$repository"
fi

cd "$repository"
printf '\nChecking Serpent dependencies using the persistent Go cache...\n'
go mod download
linter_version=""
if [[ -x "$GOPATH/bin/golangci-lint" ]]; then
  linter_version="$("$GOPATH/bin/golangci-lint" version)"
fi
if [[ "$linter_version" == *'version v1.53.3 built with go1.21.13'* ]]; then
  printf '\nReusing the installed golangci-lint v1.53.3.\n'
else
  printf '\nBuilding golangci-lint v1.53.3. The first build can take several minutes.\n'
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.53.3
fi
"$GOPATH/bin/golangci-lint" version
printf '\nRunning the analyzers with the persistent cache (up to 10 minutes)...\n'
lint_status=0
"$GOPATH/bin/golangci-lint" run --timeout=10m || lint_status=$?
case "$lint_status" in
  0) printf '\nAnalyzer cache ready; no lint findings.\n' ;;
  1) printf '\nThe pinned upstream source has lint findings, shown above. Tools are ready; fix these in your checkout.\n' ;;
  *) printf '\nLinter setup failed with exit status %s.\n' "$lint_status" >&2; exit "$lint_status" ;;
esac
printf '\nReady! Open the terminal and run the unmodified target: make lint\n'
printf 'Checkout: %s in %s\n' "$(git rev-parse --short HEAD)" "$repository"
printf 'Go 1.21.13 and golangci-lint v1.53.3 match this demo revision.\n'
