#!/usr/bin/env bash
set -euo pipefail
profile_root="$RUNNER_TEMP/cache-profile"
mkdir -p "$profile_root"
printf 'CACHE_PROFILE_ROOT=%s\n' "$profile_root" >> "$GITHUB_ENV"
sudo apt-get -o DPkg::Lock::Timeout=300 install -y sysstat time > "$profile_root/apt.log" 2>&1
lscpu --json > "$profile_root/lscpu.json"
lsblk --json -o NAME,TYPE,SIZE,FSTYPE,MOUNTPOINTS > "$profile_root/lsblk.json"
findmnt --json > "$profile_root/mounts.json"
free -b > "$profile_root/memory.txt"
go version > "$profile_root/go-version.txt"
if [[ "$PROFILE_PROVIDER" == coder ]]; then
  test -S "$CODER_CACHE_JOB_DIR/go-agent.sock"
  test "$(stat -f -c %T "$CODER_CACHE_JOB_DIR")" != tmpfs
  findmnt -T "$CODER_CACHE_JOB_DIR" -o TARGET,SOURCE,FSTYPE > "$profile_root/cache-storage.txt"
  sha256sum /usr/local/bin/coder-cache > "$profile_root/adapter-sha256.txt"
  test "$(sha256sum /usr/local/bin/coder-cache | cut -d' ' -f1)" = 7ba6a3747fcf647f9195c68687878fa3f5d6a3d4851408ca046ee6dfd068a6f2
  test -z "${CODER_CACHE_GO_PLUGIN:-}"
fi
