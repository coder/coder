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
  sha256sum /usr/local/bin/coder-cache > "$profile_root/adapter-sha256.txt"
  test "$(sha256sum /usr/local/bin/coder-cache | cut -d' ' -f1)" = e3520e1bb30e4d09f527afc18864b5e274b8305b6bc396f5397ad9312cebde4d
  test -z "${CODER_CACHE_GO_PLUGIN:-}"
fi
