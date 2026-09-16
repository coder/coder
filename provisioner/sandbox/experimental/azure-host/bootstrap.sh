#!/usr/bin/env bash
# Dedicated experimental host inside a disposable Linux VM.
# This installer requires root and must run only on the dedicated host.
set +x
set -Eeuo pipefail
umask 077
# Inner experiment commands must never inherit an outer Coder session.
unset CODER_URL CODER_SESSION_TOKEN CODER_AGENT_TOKEN CODER_AGENT_URL CODER_AGENT_AUTH
unset PGPASSWORD PGHOST PGPORT PGUSER PGDATABASE

CONTAINERD_VERSION=2.2.8
GVISOR_VERSION=20260907.0
CNI_VERSION=1.9.1
GO_VERSION=1.26.5
CONTAINERD_SHA256=27b74a4f85c9bf5ffcedcbc4aaa7408ae3c5e7079a6130aeda5c252288d59d67
GVISOR_SHA256=81416511897ab8abd4e723d66823c5b0461a2ee3311cfa70d152404ef9b860cf
CNI_SHA256=b98f74a0f8522f0a83867178729c1aa70f2158f90c45a2ca8fa791db1c76b303
GO_SHA256=5c2c3b16caefa1d968a94c1daca04a7ca301a496d9b086e17ad77bb81393f053
ASSETS=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
STATE=/var/lib/coder-sandbox-host
TOOLS=/opt/coder-sandbox-host
CONFIG=/etc/coder-sandbox-host
RUN=/run/coder-sandbox-host
SOCKET=$RUN/containerd.sock
DB_USER=coder-sandbox-db
DB_PORT=55432
HTTP_PORT=3020
PHASE=${1:-help}
STEP=initialization

record_status() {
  if [[ $EUID == 0 && $(uname -s) == Linux ]]; then
    install -d -m 0755 "$STATE"
    printf '%s\n' "$*" > "$STATE/status"
  fi
}
log() {
  printf '[sandbox-host] %s\n' "$*"
  if [[ $PHASE != status ]]; then record_status "running: $PHASE: $STEP"; fi
}
fail() { record_status "failed: $PHASE: $STEP: $*"; printf '[sandbox-host] ERROR: %s\n' "$*" >&2; exit 1; }
on_error() {
  local rc=$1 line=$2
  record_status "failed: $PHASE: $STEP: line $line, exit $rc"
  printf '[sandbox-host] Phase %s failed at %s (line %s, exit %s). No capability restrictions were disabled.\n' "$PHASE" "$STEP" "$line" "$rc" >&2
  exit "$rc"
}
trap 'on_error "$?" "$LINENO"' ERR

usage() {
  cat <<'HELP'
Usage: sudo ./bootstrap.sh {preflight|install|build|image|start|validate|status|all}

Place source.json beside this script with a public GitHub repository and
full 40-character commit SHA. The Coder template delivers this file.
Phases: install host tools; build pinned source; prepare cached image;
start the isolated database/containerd/Coder; validate fresh create/SSH/delete.
all runs those phases in order, including a real disposable sandbox smoke test.

Optional first-start inputs: SANDBOX_HOST_IP, SANDBOX_SUBNET (10.203.0.0/24),
SANDBOX_DNS (1.1.1.1). They are persisted; changing them later is rejected.
image accepts SANDBOX_IMAGE_ARCHIVE=/absolute/path/image.tar to avoid Docker.
validate accepts KEEP_SANDBOX=1 to leave the successful test sandbox running.
HELP
}

require_root() {
  [[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || fail 'Linux/amd64 is required.'
  [[ $EUID == 0 ]] || fail 'Run with sudo; this experiment needs mounts, CNI and writable cgroups.'
}

preflight() {
  STEP='host capability preflight'
  require_root
  command -v systemctl >/dev/null || fail 'systemd is required by these host service scripts.'
  systemctl show-environment >/dev/null || fail 'systemd is unavailable; a normal container is insufficient.'
  command -v unshare >/dev/null || fail 'Install util-linux for the network-namespace capability check.'
  unshare --net true || fail 'Cannot create a network namespace. This host configuration cannot host the runtime.'
  [[ -d /sys/fs/cgroup ]] || fail 'No cgroup filesystem is mounted.'
  grep -qw overlay /proc/filesystems || modprobe overlay || fail 'The overlayfs snapshotter needs kernel overlayfs support.'
  log "Host: $(uname -srmo); virtualization: $(systemd-detect-virt 2>/dev/null || true)"
  log 'Preflight succeeded. Actual gVisor, mount and cgroup enforcement are checked during validate.'
}

directories() {
  install -d -m 0755 "$STATE" "$TOOLS" "$CONFIG" "$RUN" "$STATE/downloads" "$TOOLS/bin" "$TOOLS/cni/bin"
  install -d -m 0700 "$STATE/secrets" "$STATE/logs" "$STATE/cli" "$STATE/runtime" "$STATE/results"
  touch "$STATE/logs/containerd.log" "$STATE/logs/postgres.log" "$STATE/logs/server.log"
  chmod 0600 "$STATE/logs/containerd.log" "$STATE/logs/postgres.log" "$STATE/logs/server.log"
  export PATH="$TOOLS/bin:$TOOLS/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
}

download() {
  local url=$1 digest=$2 destination=$3
  if [[ ! -f $destination ]] || ! printf '%s  %s\n' "$digest" "$destination" | sha256sum --check --status; then
    curl --fail --silent --show-error --location --retry 3 --connect-timeout 20 "$url" -o "$destination.partial"
    printf '%s  %s\n' "$digest" "$destination.partial" | sha256sum --check --status || fail "Checksum mismatch for $(basename "$destination")."
    mv -- "$destination.partial" "$destination"
  fi
  log "Verified $(basename "$destination")"
}

install_tools() {
  preflight
  directories
  STEP='installing OS dependencies'
  command -v apt-get >/dev/null || fail 'The installer currently supports Debian/Ubuntu apt-based hosts.'
  DEBIAN_FRONTEND=noninteractive apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates curl git jq python3 tar gzip bzip2 xz-utils openssl \
    iproute2 iptables util-linux kmod postgresql postgresql-client
  if ! command -v docker >/dev/null && [[ -z ${SANDBOX_IMAGE_ARCHIVE:-} ]]; then
    STEP='installing Docker for image construction on this new host'
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends docker.io docker-buildx
    systemctl enable --now docker.service
  fi
  if [[ -z ${SANDBOX_IMAGE_ARCHIVE:-} ]] && ! docker buildx version >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends docker-buildx
  fi
  STEP='installing checksum-pinned runtime tools'
  download "https://github.com/containerd/containerd/releases/download/v$CONTAINERD_VERSION/containerd-$CONTAINERD_VERSION-linux-amd64.tar.gz" "$CONTAINERD_SHA256" "$STATE/downloads/containerd.tar.gz"
  download "https://github.com/google/gvisor/releases/download/release-$GVISOR_VERSION/gvisor-x86_64.tar.bz2" "$GVISOR_SHA256" "$STATE/downloads/gvisor.tar.bz2"
  download "https://github.com/containernetworking/plugins/releases/download/v$CNI_VERSION/cni-plugins-linux-amd64-v$CNI_VERSION.tgz" "$CNI_SHA256" "$STATE/downloads/cni.tgz"
  # All runtime binaries live under our own prefix, never /usr/bin/containerd.
  tar -xzf "$STATE/downloads/containerd.tar.gz" -C "$TOOLS"
  local stage runsc_binary
  stage=$(mktemp -d "$STATE/downloads/gvisor.XXXXXX")
  tar -xjf "$STATE/downloads/gvisor.tar.bz2" -C "$stage"
  runsc_binary=$(find "$stage" -maxdepth 3 -type f -name runsc -print -quit)
  [[ -n $runsc_binary && -d $(dirname "$runsc_binary")/gvisor-bin ]] || fail 'Full gVisor archive is missing runsc or its sidecars.'
  cp -a "$(dirname "$runsc_binary")/." "$TOOLS/bin/"
  rm -rf -- "$stage"
  tar -xzf "$STATE/downloads/cni.tgz" -C "$TOOLS/cni/bin"
  # runsc reexecutes as an unprivileged user; parent paths and sidecars must be traversable.
  chmod 0755 "$TOOLS" "$TOOLS/bin"
  STEP='installing pinned Go toolchain'
  if ! command -v go >/dev/null || [[ $(go env GOVERSION) != go$GO_VERSION ]]; then
    download "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz" "$GO_SHA256" "$STATE/downloads/go.tar.gz"
    tar -xzf "$STATE/downloads/go.tar.gz" -C "$TOOLS"
  fi
  go version
  "$TOOLS/bin/containerd" --version
  "$TOOLS/bin/runsc" --version
  log 'Installed dedicated tools. Preexisting Docker/containerd services were not restarted or reconfigured.'
}

build() {
  require_root
  directories
  STEP='checking public source repository and pinned commit'
  [[ -f $ASSETS/source.json && ! -L $ASSETS/source.json ]] || fail 'Missing adjacent regular source.json.'
  [[ $(go env GOVERSION) == go$GO_VERSION ]] || fail 'Run install first to provide Go 1.26.5.'
  # Accept only public GitHub clone URLs; never accept embedded authentication.
  python3 - "$ASSETS/source.json" <<'PYCODE'
import json, re, sys
with open(sys.argv[1]) as source:
    config = json.load(source)
assert set(config) == {'repository', 'commit'}, 'Unexpected source configuration fields'
assert isinstance(config['repository'], str) and re.fullmatch(
    r'https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\.git', config['repository']), 'Expected a public GitHub clone URL without credentials'
assert isinstance(config['commit'], str) and re.fullmatch(r'[a-f0-9]{40}', config['commit']), 'Expected a full lowercase source commit SHA'
PYCODE
  local source_repository source_commit source final_source
  source_repository=$(jq -er '.repository' "$ASSETS/source.json")
  source_commit=$(jq -er '.commit' "$ASSETS/source.json")
  final_source=$STATE/source-$source_commit
  source=$final_source
  export GIT_TERMINAL_PROMPT=0 GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
  if [[ ! -e $source && ! -L $source ]]; then
    source=$(mktemp -d "$STATE/.source-$source_commit.XXXXXX")
    log "Preparing source in $source; failed staging directories are retained for inspection."
    git init -q "$source"
    git -C "$source" remote add origin "$source_repository"
    git -C "$source" -c credential.helper= -c core.askPass= fetch --depth 1 origin "$source_commit"
    git -C "$source" checkout --detach -q FETCH_HEAD
  fi
  [[ ! -L $source && $(git -C "$source" rev-parse HEAD) == "$source_commit" ]] || fail 'Existing checkout does not match the pinned source commit.'
  [[ $(git -C "$source" remote get-url origin) == "$source_repository" ]] || fail 'Existing checkout has a different source repository.'
  git -C "$source" diff --quiet || fail 'Existing source has unstaged edits; preserve them and use a different commit directory.'
  git -C "$source" diff --cached --quiet || fail 'Existing source has staged edits; preserve them and use a different commit directory.'
  [[ -z $(git -C "$source" ls-files --others --exclude-standard) ]] || fail 'Existing source has unknown untracked files.'
  [[ -f $source/provisioner/sandbox/runtime_linux.go && \
     -f $source/provisioner/sandbox/testhost/Dockerfile && \
     -f $source/provisioner/sandbox/testhost/smoke.sh ]] || fail 'Pinned source must contain the experimental native sandbox provisioner and testhost image.'
  if [[ $source != "$final_source" ]]; then
    # Both paths share a filesystem. Never replace a checkout created by another run.
    mv -T --no-clobber "$source" "$final_source"
    [[ ! -e $source ]] || fail "Source destination appeared during preparation; staging is retained at $source."
    source=$final_source
  fi
  printf '%s\n' "$source" > "$STATE/source-path"
  printf '%s\n' "$source_commit" > "$STATE/source-commit"
  printf '%s\n' "$source_repository" > "$STATE/source-repository"
  export GOCACHE=$STATE/go-build GOMODCACHE=$STATE/go-mod GOTOOLCHAIN=local CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=
  STEP='building experimental server and slim agent'
  (cd "$source" && go build -p 2 -trimpath -buildvcs=false -o "$TOOLS/bin/coder" ./cmd/coder)
  install -d "$source/build"
  (cd "$source" && go build -p 2 -trimpath -buildvcs=false -tags=slim -o build/coder-sandbox-agent ./cmd/coder)
  log "Built exact source commit $source_commit. Server is headless (no frontend assets)."
}

containerd_service() {
  STEP='starting isolated containerd 2'
  cat > "$CONFIG/containerd.toml" <<EOF
version = 3
root = "$STATE/containerd"
state = "$RUN/containerd"
disabled_plugins = ["io.containerd.cri.v1.images", "io.containerd.cri.v1.runtime"]
[grpc]
  address = "$SOCKET"
EOF
  cat > /etc/systemd/system/coder-sandbox-containerd.service <<EOF
[Unit]
Description=Dedicated native Coder sandbox containerd
After=network.target
[Service]
Type=notify
StandardOutput=append:$STATE/logs/containerd.log
StandardError=inherit
Environment=PATH=$TOOLS/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=$TOOLS/bin/containerd --config $CONFIG/containerd.toml
Restart=on-failure
Delegate=yes
KillMode=process
TasksMax=infinity
LimitNOFILE=1048576
RuntimeDirectory=coder-sandbox-host
RuntimeDirectoryPreserve=yes
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now coder-sandbox-containerd.service
  for _ in {1..60}; do
    if "$TOOLS/bin/ctr" --address "$SOCKET" version >/dev/null 2>&1; then return; fi
    sleep 1
  done
  fail "Dedicated containerd did not start; inspect $STATE/logs/containerd.log."
}

ctr() { "$TOOLS/bin/ctr" --address "$SOCKET" --namespace coder-sandbox "$@"; }

image_phase() {
  require_root
  directories
  containerd_service
  STEP='building or importing curated image'
  local archive source image_name image_ref image_listing
  image_name=example.invalid/coder/sandbox
  archive=${SANDBOX_IMAGE_ARCHIVE:-$STATE/sandbox-image.tar}
  if [[ -z ${SANDBOX_IMAGE_ARCHIVE:-} ]]; then
    if ! command -v docker >/dev/null || ! docker info >/dev/null 2>&1; then
      fail 'Provide SANDBOX_IMAGE_ARCHIVE, or a working Docker daemon for image construction. Existing Docker configuration is never changed.'
    fi
    source=$(cat "$STATE/source-path")
    docker build --platform linux/amd64 -f "$source/provisioner/sandbox/testhost/Dockerfile" -t "$image_name:experiment" "$source"
    docker save "$image_name:experiment" -o "$archive"
  fi
  [[ -f $archive ]] || fail "Image archive not found: $archive"
  ctr images import --digests --platform linux/amd64 --snapshotter overlayfs "$archive"
  # WithDigestRef may produce only sha256:<digest>; make the required canonical named record explicitly.
  # Capture the complete, checked listing: an early pipe consumer can SIGPIPE ctr under pipefail.
  image_listing=$(ctr images list) || return $?
  image_ref=$(awk -v name="$image_name:experiment" '
    $1 == name { matches++; digest = $3 }
    END { if (matches != 1) exit 1; print digest }
  ' <<< "$image_listing") || fail "Archive must contain exactly one $image_name:experiment image."
  [[ $image_ref =~ ^sha256:[a-f0-9]{64}$ ]] || fail "Archive must contain $image_name:experiment; could not determine its manifest digest."
  if ! awk -v name="$image_name@$image_ref" '
    $1 == name { found = 1 }
    END { exit !found }
  ' <<< "$image_listing"; then
    ctr images tag "$image_name:experiment" "$image_name@$image_ref"
  fi
  printf '%s@%s\n' "$image_name" "$image_ref" > "$STATE/image-ref"
  ctr images check
  log "Cached image: $image_name@$image_ref"
}

network_config() {
  STEP='configuring dedicated sandbox bridge'
  local host_ip subnet dns
  host_ip=${SANDBOX_HOST_IP:-$(ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1);exit}}')}
  subnet=${SANDBOX_SUBNET:-10.203.0.0/24}
  dns=${SANDBOX_DNS:-1.1.1.1}
  python3 - "$host_ip" "$subnet" "$dns" <<'PY'
import ipaddress, json, subprocess, sys
host, network, dns = ipaddress.ip_address(sys.argv[1]), ipaddress.ip_network(sys.argv[2]), ipaddress.ip_address(sys.argv[3])
assert host.version == network.version == dns.version == 4, 'IPv4 host, subnet and DNS are required'
assert not host.is_loopback and not dns.is_loopback and not host.is_unspecified and not dns.is_unspecified
for route in json.loads(subprocess.check_output(['ip', '-j', '-4', 'route', 'show'])):
    if route.get('dst') in (None, 'default') or route.get('dev') == 'coder-sbx0':
        continue
    assert not network.overlaps(ipaddress.ip_network(route['dst'], strict=False)), 'Sandbox subnet overlaps an existing route: ' + str(route)
PY
  if [[ -f $STATE/network-settings ]]; then
    [[ $(cat "$STATE/network-settings") == "$host_ip $subnet $dns" ]] || fail 'Network settings changed; preserve existing allocations and resolve explicitly before restarting.'
  fi
  printf '%s %s %s\n' "$host_ip" "$subnet" "$dns" > "$STATE/network-settings"
  printf 'http://%s:%s\n' "$host_ip" "$HTTP_PORT" > "$STATE/access-url"
  install -d -m 0755 "$CONFIG/cni"
  jq -n --arg subnet "$subnet" --arg dns "$dns" '{cniVersion:"1.0.0",name:"coder-sandbox",plugins:[{type:"bridge",bridge:"coder-sbx0",isGateway:true,ipMasq:true,dns:{nameservers:[$dns]},ipam:{type:"host-local",ranges:[[{subnet:$subnet}]],routes:[{dst:"0.0.0.0/0"}]}}]}' > "$CONFIG/cni/10-coder-sandbox.conflist"
  sysctl -w net.ipv4.ip_forward=1 >/dev/null || fail 'IPv4 forwarding is unavailable in this outer workspace.'
  printf 'net.ipv4.ip_forward=1\n' > /etc/sysctl.d/90-coder-sandbox-host.conf
  # Keep Docker's chains and global policies intact; add only bridge-scoped rules.
  iptables -C FORWARD -i coder-sbx0 -m comment --comment coder-sandbox-host -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -i coder-sbx0 -m comment --comment coder-sandbox-host -j ACCEPT
  iptables -C FORWARD -o coder-sbx0 -m conntrack --ctstate RELATED,ESTABLISHED -m comment --comment coder-sandbox-host -j ACCEPT 2>/dev/null || iptables -I FORWARD 1 -o coder-sbx0 -m conntrack --ctstate RELATED,ESTABLISHED -m comment --comment coder-sandbox-host -j ACCEPT
}

database_service() {
  STEP='initializing isolated PostgreSQL'
  local pg_bin
  if [[ -n $(ss -H -ltn "sport = :$DB_PORT") ]] && ! systemctl is-active --quiet coder-sandbox-postgres.service; then
    fail "Port $DB_PORT is already occupied by a different service."
  fi
  pg_bin=$(find /usr/lib/postgresql -maxdepth 3 -type f -name postgres | sort -V | tail -1)
  [[ -n $pg_bin ]] || fail 'Install PostgreSQL server tools first.'
  pg_bin=$(dirname "$pg_bin")
  id "$DB_USER" >/dev/null 2>&1 || useradd --system --home-dir "$STATE/postgres" --shell /usr/sbin/nologin "$DB_USER"
  # The persistent database can outlive its OS disk and system-user allocation.
  if [[ -f $STATE/postgres/PG_VERSION ]] && \
    [[ $(stat -c '%u:%g' "$STATE/postgres") != "$(id -u "$DB_USER"):$(id -g "$DB_USER")" ]]; then
    chown -hR "$DB_USER:$DB_USER" "$STATE/postgres"
  fi
  [[ -f $STATE/secrets/db-password ]] || openssl rand -hex 32 > "$STATE/secrets/db-password"
  if [[ ! -f $STATE/postgres/PG_VERSION ]]; then
    install -d -m 0700 -o "$DB_USER" -g "$DB_USER" "$STATE/postgres"
    install -m 0600 -o "$DB_USER" -g "$DB_USER" "$STATE/secrets/db-password" "$RUN/pg-init-password"
    runuser -u "$DB_USER" -- "$pg_bin/initdb" -D "$STATE/postgres" --username=coder --pwfile="$RUN/pg-init-password" --auth-host=scram-sha-256 --auth-local=peer > "$STATE/logs/initdb.log" 2>&1
    rm -f "$RUN/pg-init-password"
  fi
  cat > /etc/systemd/system/coder-sandbox-postgres.service <<EOF
[Unit]
Description=Isolated database for native Coder sandbox experiment
After=network.target
[Service]
User=$DB_USER
StandardOutput=append:$STATE/logs/postgres.log
StandardError=inherit
ExecStart=$pg_bin/postgres -D $STATE/postgres -h 127.0.0.1 -p $DB_PORT -k $STATE/postgres
Restart=on-failure
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now coder-sandbox-postgres.service
  for _ in {1..60}; do
    if "$pg_bin/pg_isready" -h 127.0.0.1 -p "$DB_PORT" >/dev/null; then break; fi
    sleep 1
  done
  export PGPASSFILE=$STATE/secrets/pgpass
  printf '127.0.0.1:%s:*:coder:%s\n' "$DB_PORT" "$(cat "$STATE/secrets/db-password")" > "$PGPASSFILE"
  if [[ $("$pg_bin/psql" -h 127.0.0.1 -p "$DB_PORT" -U coder -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='coder'") != 1 ]]; then
    "$pg_bin/createdb" -h 127.0.0.1 -p "$DB_PORT" -U coder coder
  fi
}

authenticate() {
  STEP='bootstrapping private inner Coder admin'
  local url first_status
  url=$(cat "$STATE/access-url")
  [[ -f $STATE/secrets/admin-password ]] || openssl rand -hex 32 > "$STATE/secrets/admin-password"
  jq -n --rawfile password "$STATE/secrets/admin-password" '{email:"sandbox-admin@example.invalid",username:"sandbox-admin",name:"Sandbox experiment",password:($password|rtrimstr("\n")),trial:false}' > "$STATE/secrets/first-user.json"
  first_status=$(curl -sS -o "$STATE/secrets/first-user-response.json" -w '%{http_code}' "$url/api/v2/users/first")
  if [[ $first_status == 404 ]]; then
    curl -fsS -H 'Content-Type: application/json' --data-binary @"$STATE/secrets/first-user.json" "$url/api/v2/users/first" -o "$STATE/secrets/first-user-response.json"
  elif [[ $first_status != 200 ]]; then
    fail "Unexpected inner Coder first-user response: HTTP $first_status"
  fi
  jq '{email,password}' "$STATE/secrets/first-user.json" > "$STATE/secrets/login.json"
  curl -fsS -H 'Content-Type: application/json' --data-binary @"$STATE/secrets/login.json" "$url/api/v2/users/login" -o "$STATE/secrets/login-response.json"
  jq -er '.session_token' "$STATE/secrets/login-response.json" > "$STATE/cli/session"
  printf '%s\n' "$url" > "$STATE/cli/url"
  rm -f "$STATE/secrets/login-response.json" "$STATE/secrets/login.json" "$STATE/secrets/first-user.json" "$STATE/secrets/first-user-response.json"
  log 'Inner admin authenticated; credentials remain in root-only files.'
}

start() {
  preflight
  directories
  [[ -x $TOOLS/bin/coder && -f $STATE/image-ref ]] || fail 'Run build and image before start.'
  containerd_service
  network_config
  database_service
  STEP='starting experimental Coder with four built-in sandbox workers'
  if [[ -n $(ss -H -ltn "sport = :$HTTP_PORT") ]] && ! systemctl is-active --quiet coder-sandbox-server.service; then
    fail "Port $HTTP_PORT is already occupied by a different service."
  fi
  cat > "$STATE/secrets/server.env" <<EOF
CODER_PG_CONNECTION_URL=postgres://coder:$(cat "$STATE/secrets/db-password")@127.0.0.1:$DB_PORT/coder?sslmode=disable
CODER_HTTP_ADDRESS=0.0.0.0:$HTTP_PORT
CODER_ACCESS_URL=$(cat "$STATE/access-url")
CODER_PROVISIONER_TYPES=sandbox
CODER_PROVISIONER_DAEMONS=4
CODER_API_RATE_LIMIT=20000
CODER_TELEMETRY_ENABLE=false
CODER_SANDBOX_CONTAINERD_ADDRESS=$SOCKET
CODER_SANDBOX_STATE_DIRECTORY=$STATE/runtime
CODER_SANDBOX_CNI_CONFIG_DIRECTORY=$CONFIG/cni
CODER_SANDBOX_CNI_BIN_DIRECTORY=$TOOLS/cni/bin
EOF
  cat > /etc/systemd/system/coder-sandbox-server.service <<EOF
[Unit]
Description=Experimental headless Coder with four native sandbox workers
Requires=coder-sandbox-containerd.service coder-sandbox-postgres.service
After=coder-sandbox-containerd.service coder-sandbox-postgres.service network.target
[Service]
StandardOutput=append:$STATE/logs/server.log
StandardError=inherit
EnvironmentFile=$STATE/secrets/server.env
Environment=PATH=$TOOLS/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=$TOOLS/bin/coder --global-config $STATE/server-config server
Restart=on-failure
Delegate=yes
KillMode=process
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable coder-sandbox-server.service
  systemctl restart coder-sandbox-server.service
  local ready=false url
  url=$(cat "$STATE/access-url")
  for _ in {1..120}; do
    if curl -fsS "$url/healthz" >/dev/null 2>&1; then ready=true; break; fi
    sleep 1
  done
  [[ $ready == true ]] || fail "Inner Coder did not become healthy. Inspect $STATE/logs/server.log; no credentials are printed automatically."
  authenticate
  install -d -m 0700 "$STATE/template"
  cat > "$STATE/template/sandbox.yaml" <<EOF
version: 1
image: $(cat "$STATE/image-ref")
cpu: 1
memory_mib: 2048
workdir: /workspace
daily_cost: 1
EOF
  "$TOOLS/bin/coder" --global-config "$STATE/cli" templates push native-sandbox --provisioner sandbox --directory "$STATE/template" --yes
  log "Ready at $url. Run validate from inside this outer workspace."
}

# Cancel a still-running create before deleting this smoke test's workspace.
# Keep only scoped identifiers/status in diagnostics; journals contain credentials.
cleanup_smoke_workspace() (
  local workspace=$1 workspace_id='' status job deadline current runtime_ids
  current=$STATE/results/$workspace-workspace.json
  runtime_ids=$STATE/results/$workspace-runtime-ids.txt
  : > "$runtime_ids" || return

  capture_smoke_workspace() {
    local found build
    timeout --kill-after=5s 15 "$TOOLS/bin/coder" --global-config "$STATE/cli" list \
      --search "owner:me name:$workspace" --output json |
      jq --arg name "$workspace" '[.[] | select(.name == $name) |
        {id,name,template_name,job_id:.latest_build.job.id,job_status:.latest_build.job.status,
         build_id:.latest_build.id}] |
        if length > 1 then error("ambiguous smoke workspace identity") else .[0] // null end' > "$current" || return
    found=$(jq -r '.id // empty' "$current") || return
    if [[ -n $found ]]; then
      [[ -z $workspace_id || $workspace_id == "$found" ]] || return 1
      [[ $(jq -r .template_name "$current") == native-sandbox ]] || return 1
      workspace_id=$found
      build=$(jq -r .build_id "$current") || return
      printf 'coder-sandbox-%s\n' "$build" >> "$runtime_ids" || return
    fi
  }

  assert_smoke_inventory_empty() {
    [[ -n $workspace_id ]] || return 1
    local containers tasks snapshots runsc_ids
    local runsc_errors=$STATE/logs/runsc-$workspace.log
    containers=$(timeout --kill-after=5s 30 "$TOOLS/bin/ctr" --address "$SOCKET" --namespace coder-sandbox \
      containers list --quiet "labels.\"com.coder.sandbox.managed\"==\"true\",labels.\"com.coder.sandbox.workspace-id\"==\"$workspace_id\"") || return
    [[ -z $containers ]] || { log 'Smoke container metadata remains.' >&2; return 1; }
    tasks=$(timeout --kill-after=5s 30 "$TOOLS/bin/ctr" --address "$SOCKET" --namespace coder-sandbox tasks list) || return
    snapshots=$(timeout --kill-after=5s 30 "$TOOLS/bin/ctr" --address "$SOCKET" --namespace coder-sandbox snapshots --snapshotter overlayfs list) || return
    # A canceled containerd task creation can lose its record while runsc still
    # owns the sandbox. Its namespace-specific inventory must also be empty.
    runsc_ids=$(timeout --kill-after=5s 30 "$TOOLS/bin/runsc" --root /run/containerd/runsc/coder-sandbox list --format=json 2> "$runsc_errors" |
      jq -cer '(. // []) | if type == "array" and all(.[]; type == "object" and (.id | type == "string"))
        then map(.id) else error("invalid runsc inventory") end') || return
    [[ ! -s $runsc_errors ]] || { log 'runsc inventory emitted diagnostics; cleanup could not be verified.' >&2; return 1; }
    python3 - "$STATE/runtime" "$workspace_id" "$runtime_ids" "$tasks" "$snapshots" "$runsc_ids" <<'PYCODE'
import ipaddress, json, pathlib, sys, uuid
root, workspace, ids_file, tasks, snapshots, runsc_ids = sys.argv[1:]
assert str(uuid.UUID(workspace)) == workspace, 'invalid workspace identity'
runtime_ids = set(pathlib.Path(ids_file).read_text().splitlines())
for path in (pathlib.Path(root) / 'operations' / workspace).glob('*.json'):
    state = json.loads(path.read_text())
    assert state['workspace_id'] == workspace, 'journal workspace mismatch'
    if state.get('runtime_id'):
        runtime_ids.add(state['runtime_id'])
assert runtime_ids, 'no smoke allocation identity captured'
for runtime_id in runtime_ids:
    assert runtime_id.startswith('coder-sandbox-'), 'invalid allocation identity'
    suffix = runtime_id.removeprefix('coder-sandbox-')
    assert str(uuid.UUID(suffix)) == suffix, 'invalid allocation identity'
    assert not (pathlib.Path(root) / 'network' / runtime_id).exists(), 'network allocation remains'
for listing in (tasks, snapshots):
    assert not runtime_ids.intersection(line.split()[0] for line in listing.splitlines()[1:] if line.split()), 'task or writable snapshot remains'
assert not runtime_ids.intersection(json.loads(runsc_ids)), 'runsc sandbox allocation remains'
for path in pathlib.Path('/var/lib/cni/networks/coder-sandbox').glob('*'):
    try:
        ipaddress.ip_address(path.name)
    except ValueError:
        continue
    if path.is_file():
        assert path.read_text().splitlines()[0] not in runtime_ids, 'CNI address allocation remains'
PYCODE
  }

  deadline=$((SECONDS + 90))
  capture_smoke_workspace || return
  # A timed-out create may still commit. Without its identity we cannot prove
  # runtime cleanup, so wait boundedly and fail clearly if it never appears.
  while [[ -z $workspace_id ]] && (( SECONDS < deadline )); do
    sleep 1
    capture_smoke_workspace || return
  done
  [[ -n $workspace_id ]] || { log 'Could not identify the smoke workspace after uncertain create.' >&2; return 1; }
  status=$(jq -r '.job_status // "gone"' "$current") || return
  if [[ $status == pending || $status == running || $status == canceling ]]; then
    job=$(jq -r .job_id "$current") || return
    if [[ $status != canceling ]]; then
      # A job may finish between the list and cancel. Accept that race only if
      # a fresh read proves it is terminal; otherwise preserve cancellation failure.
      if ! timeout --kill-after=5s 30 "$TOOLS/bin/coder" --global-config "$STATE/cli" provisioner jobs cancel "$job"; then
        capture_smoke_workspace || return
        status=$(jq -r '.job_status // "gone"' "$current") || return
        [[ $status != pending && $status != running && $status != canceling ]] || return 1
      fi
    fi
    deadline=$((SECONDS + 90))
    while (( SECONDS < deadline )); do
      capture_smoke_workspace || return
      status=$(jq -r '.job_status // "gone"' "$current") || return
      [[ $status == pending || $status == running || $status == canceling ]] || break
      sleep 1
    done
  fi
  case "$status" in
    succeeded|failed|canceled)
      timeout --kill-after=5s 180 "$TOOLS/bin/coder" --global-config "$STATE/cli" delete "$workspace" --yes || return
      capture_smoke_workspace || return
      ;;
    gone) ;;
    *) log 'Smoke provisioning job did not reach a terminal state.' >&2; return 1 ;;
  esac
  [[ $(jq -r '.id // empty' "$current") == "" ]] || return 1
  assert_smoke_inventory_empty || return
)

validate() (
  require_root
  directories
  STEP='fresh sandbox create, authenticated command and cleanup'
  local workspace suffix created=false keep=false started finished
  suffix=$(tr -d '-' < /proc/sys/kernel/random/uuid)
  workspace=sbx-smoke-${suffix:0:16}
  # shellcheck disable=SC2329 # Invoked by this subshell's EXIT trap.
  cleanup_smoke() {
    local rc=$?
    trap - EXIT ERR
    if [[ $created == true && $keep != true ]]; then
      if ! cleanup_smoke_workspace "$workspace" > "$STATE/logs/delete-$workspace.log" 2>&1; then
        record_status "failed: validate: cleanup of $workspace"
        printf '[sandbox-host] Cleanup incomplete for %s; inspect private log %s before retrying cancellation/deletion.\n' "$workspace" "$STATE/logs/delete-$workspace.log" >&2
        exit 1
      fi
    fi
    exit "$rc"
  }
  trap cleanup_smoke EXIT
  started=$(date +%s%N)
  # Mark before the API request so failures after allocation still trigger deletion.
  created=true
  timeout --kill-after=5s 180 "$TOOLS/bin/coder" --global-config "$STATE/cli" create "$workspace" --template native-sandbox --yes
  timeout --kill-after=5s 180 "$TOOLS/bin/coder" --global-config "$STATE/cli" ssh "$workspace" -- true
  finished=$(date +%s%N)
  timeout --kill-after=5s 180 "$TOOLS/bin/coder" --global-config "$STATE/cli" ssh "$workspace" -- sandbox-smoke
  jq -n --arg workspace "$workspace" --arg image "$(cat "$STATE/image-ref")" --arg source_commit "$(cat "$STATE/source-commit")" --argjson ready_ms "$(((finished-started)/1000000))" '{workspace:$workspace,image:$image,source_commit:$source_commit,create_to_first_command_ms:$ready_ms,smoke_passed:true,note:"One smoke sample, not percentile or concurrency evidence"}' > "$STATE/results/$workspace.json"
  if [[ ${KEEP_SANDBOX:-0} == 1 ]]; then keep=true; fi
  log "Smoke passed; result: $STATE/results/$workspace.json"
  if [[ $keep == true ]]; then log "Sandbox retained: $workspace"; fi
)

status() {
  require_root
  directories
  systemctl is-active coder-sandbox-containerd coder-sandbox-postgres coder-sandbox-server || true
  [[ ! -f $STATE/access-url ]] || log "Inner URL: $(cat "$STATE/access-url")"
  [[ ! -f $STATE/image-ref ]] || log "Image: $(cat "$STATE/image-ref")"
  [[ ! -x $TOOLS/bin/ctr ]] || ctr tasks list
}

run_phase() {
  PHASE=$1
  record_status "running: $PHASE"
  "$2"
  record_status "complete: $PHASE"
}

case "$PHASE" in
  preflight) run_phase preflight preflight ;;
  install) run_phase install install_tools ;;
  build) run_phase build build ;;
  image) run_phase image image_phase ;;
  start) run_phase start start ;;
  validate) run_phase validate validate ;;
  status) status ;;
  all)
    run_phase install install_tools
    run_phase build build
    run_phase image image_phase
    run_phase start start
    run_phase validate validate
    record_status success
    ;;
  help|-h|--help) usage ;;
  *) usage; exit 2 ;;
esac
