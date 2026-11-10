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
  profile_go="$(go env GOROOT)/bin/go"
  profile_module=github.com/tailscale/go-cache-plugin@v0.1.2-0.20260723040932-3031b5d01c50
  GOCACHEPROG= go mod download -json "$profile_module" > "$profile_root/plugin-module.json"
  profile_source=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["Dir"])' "$profile_root/plugin-module.json")
  cp -r "$profile_source" "$RUNNER_TEMP/profile-plugin-source"
  chmod -R u+w "$RUNNER_TEMP/profile-plugin-source"
  cp "$PROFILE_SCRIPTS/profile_http.go" "$RUNNER_TEMP/profile-plugin-source/cmd/go-cache-plugin/profile_http.go"
  python3 - "$RUNNER_TEMP/profile-plugin-source" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1])/'cmd/go-cache-plugin'
f=p/'setup.go';s=f.read_text();old='o.UsePathStyle = flags.S3PathStyle';assert old in s;s=s.replace(old,old+'\n\t\t\to.HTTPClient = profileClient{base: o.HTTPClient}');s=s.replace('Get:         cache.Get,','Get:         profileGet(cache.Get),');f.write_text(s)
f=p/'commands.go';s=f.read_text();old='func runDirect(env *command.Env) error {';assert old in s;s=s.replace(old,old+'\n\tdefer profileStart()()');f.write_text(s)
PY
  (cd "$RUNNER_TEMP/profile-plugin-source"; GOCACHEPROG= GOCACHE="$RUNNER_TEMP/profiler-build-cache" "$profile_go" build -o "$profile_root/go-cache-plugin-profile" ./cmd/go-cache-plugin)
  cp "$PROFILE_SCRIPTS/plugin-launch.sh" "$profile_root/plugin-launch"
  chmod 755 "$profile_root/plugin-launch"
  printf 'CODER_CACHE_GO_PLUGIN=%s\n' "$profile_root/plugin-launch" >> "$GITHUB_ENV"
  printf 'CACHE_PROFILE_PLUGIN=%s\n' "$profile_root/go-cache-plugin-profile" >> "$GITHUB_ENV"
fi
