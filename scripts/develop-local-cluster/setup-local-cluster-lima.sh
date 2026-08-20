#!/usr/bin/env bash

# Bootstrap Coder local Kubernetes development inside a fresh Lima Linux VM.
#
# License input, in priority order:
#   CODER_DEV_LICENSE_FILE=/path/to/license.jwt
#   CODER_DEV_LICENSE=<license JWT>
#   hidden interactive prompt
#
# Common overrides:
#   CODER_REPO_URL=https://github.com/coder/coder
#   CODER_REPO_REF=pawel/develop-local-cluster
#   CODER_REPO_DIR=$HOME/src/coder
#   CODER_DEV_CLUSTER_NAME=coder-local
#   CODER_DEV_CLUSTER_MTLS=true

set -euo pipefail

CODER_REPO_URL="${CODER_REPO_URL:-https://github.com/coder/coder}"
CODER_REPO_REF="${CODER_REPO_REF:-pawel/develop-local-cluster}"
CODER_REPO_DIR="${CODER_REPO_DIR:-${HOME}/src/coder}"
CODER_DEV_CLUSTER_NAME="${CODER_DEV_CLUSTER_NAME:-coder-local}"
CODER_DEV_CLUSTER_NAMESPACE="${CODER_DEV_CLUSTER_NAMESPACE:-coder}"
CODER_DEV_CLUSTER_GATEWAY_PORT="${CODER_DEV_CLUSTER_GATEWAY_PORT:-4001}"
CODER_DEV_CLUSTER_BUILD_JOBS="${CODER_DEV_CLUSTER_BUILD_JOBS:-2}"
CODER_DEV_CLUSTER_MTLS="${CODER_DEV_CLUSTER_MTLS:-false}"
CODER_DEV_LICENSE_FILE="${CODER_DEV_LICENSE_FILE:-}"
coder_dev_license="${CODER_DEV_LICENSE:-}"
unset CODER_DEV_LICENSE
KUBECTL_VERSION="${KUBECTL_VERSION:-v1.35.0}"
K9S_VERSION="${K9S_VERSION:-}"
MTLS_CERT_DAYS="${MTLS_CERT_DAYS:-30}"
MTLS_VALIDATION_PORT="${MTLS_VALIDATION_PORT:-30443}"

export CODER_DEV_CLUSTER_NAME CODER_DEV_CLUSTER_NAMESPACE CODER_DEV_CLUSTER_GATEWAY_PORT CODER_DEV_CLUSTER_BUILD_JOBS
export PATH="${HOME}/.local/bin:${PATH}"

log() {
	printf '\n==> %s\n' "$*"
}

normalize_bool() {
	case "${1,,}" in
	1 | true | yes) printf 'true\n' ;;
	0 | false | no) printf 'false\n' ;;
	*) fail "expected a boolean value, got: $1" ;;
	esac
}

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

install_base_packages() {
	local packages=(
		build-essential
		ca-certificates
		curl
		git
		gzip
		jq
		less
		make
		openssl
		tar
		unzip
		vim
		zstd
	)
	local missing=()
	local package

	command -v apt-get >/dev/null 2>&1 || fail "this script currently requires an apt-based Lima guest"
	for package in "${packages[@]}"; do
		if dpkg-query -W -f='${Status}' "${package}" 2>/dev/null | grep -q 'install ok installed'; then
			printf 'already installed: %s\n' "${package}"
		else
			missing+=("${package}")
		fi
	done

	if ((${#missing[@]} == 0)); then
		return
	fi

	log "Installing base packages: ${missing[*]}"
	sudo apt-get update
	sudo DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing[@]}"
}

machine_arch() {
	case "$(uname -m)" in
	x86_64) printf 'amd64\n' ;;
	aarch64 | arm64) printf 'arm64\n' ;;
	*) fail "unsupported machine architecture: $(uname -m)" ;;
	esac
}

install_mise() {
	local arch
	local checksum
	local target
	local tmp
	local version="2026.5.12"

	if command -v mise >/dev/null 2>&1; then
		printf 'already installed: mise (%s)\n' "$(mise --version)"
		return
	fi

	case "$(machine_arch)" in
	amd64)
		target="linux-x64"
		checksum="a238972a3162d710b85b28c324372e96ca4e4b486c81fe78695000d9fbc77c48"
		;;
	arm64)
		target="linux-arm64"
		checksum="fd2d5227a8ad0b1e359c70527a8345a9ada72077f8dcbb559371653c3d95464f"
		;;
	esac
	arch="mise-v${version}-${target}"

	log "Installing mise ${version}"
	tmp="$(mktemp -d)"
	curl --fail --location --output "${tmp}/mise" "https://github.com/jdx/mise/releases/download/v${version}/${arch}"
	printf '%s  %s\n' "${checksum}" "${tmp}/mise" | sha256sum --check --status
	install -Dm755 "${tmp}/mise" "${HOME}/.local/bin/mise"
	rm -rf "${tmp}"
}

configure_shell_path() {
	local file
	local marker="# Coder local cluster development tools"

	for file in "${HOME}/.profile" "${HOME}/.bashrc"; do
		if grep -Fqx "${marker}" "${file}" 2>/dev/null; then
			printf 'already configured: developer tool PATH in %s\n' "${file}"
			continue
		fi

		log "Configuring developer tool PATH in ${file}"
		cat >>"${file}" <<'EOF'

# Coder local cluster development tools
export PATH="${HOME}/.local/bin:${HOME}/.local/share/mise/shims:${PATH}"
EOF
	done
}

install_kubectl() {
	local arch="$1"
	local version="${KUBECTL_VERSION}"
	local tmp

	if command -v kubectl >/dev/null 2>&1; then
		printf 'already installed: kubectl (%s)\n' "$(kubectl version --client -o json | jq -r '.clientVersion.gitVersion')"
		return
	fi

	if [[ -z "${version}" ]]; then
		version="$(curl --fail --silent --show-error --location https://dl.k8s.io/release/stable.txt)"
	fi
	[[ "${version}" == v* ]] || version="v${version}"

	log "Installing kubectl ${version}"
	tmp="$(mktemp -d)"
	curl --fail --location --output "${tmp}/kubectl" "https://dl.k8s.io/release/${version}/bin/linux/${arch}/kubectl"
	curl --fail --location --output "${tmp}/kubectl.sha256" "https://dl.k8s.io/release/${version}/bin/linux/${arch}/kubectl.sha256"
	printf '%s  %s\n' "$(cat "${tmp}/kubectl.sha256")" "${tmp}/kubectl" | sha256sum --check --status
	install -Dm755 "${tmp}/kubectl" "${HOME}/.local/bin/kubectl"
	rm -rf "${tmp}"
}

install_k9s() {
	local arch="$1"
	local version="${K9S_VERSION}"
	local archive
	local tmp

	if command -v k9s >/dev/null 2>&1; then
		printf 'already installed: k9s (%s)\n' "$(k9s version --short 2>/dev/null | head -1)"
		return
	fi

	if [[ -z "${version}" ]]; then
		version="$(curl --fail --silent --show-error --location https://api.github.com/repos/derailed/k9s/releases/latest | jq -r '.tag_name')"
	fi
	[[ "${version}" == v* ]] || version="v${version}"
	archive="k9s_Linux_${arch}.tar.gz"

	log "Installing k9s ${version}"
	tmp="$(mktemp -d)"
	curl --fail --location --output "${tmp}/${archive}" "https://github.com/derailed/k9s/releases/download/${version}/${archive}"
	curl --fail --location --output "${tmp}/checksums.sha256" "https://github.com/derailed/k9s/releases/download/${version}/checksums.sha256"
	(
		cd "${tmp}"
		grep "  ${archive}$" checksums.sha256 | sha256sum --check --status
		tar -xzf "${archive}" k9s
	)
	install -Dm755 "${tmp}/k9s" "${HOME}/.local/bin/k9s"
	rm -rf "${tmp}"
}

check_docker() {
	command -v docker >/dev/null 2>&1 || fail "Docker is missing. Start the VM with Lima's template:docker-rootful template"
	docker info >/dev/null 2>&1 || fail "Docker is not accessible. This script expects a rootful Docker Lima VM"
	printf 'available: Docker (%s)\n' "$(docker version --format '{{.Server.Version}}')"
}

checkout_coder() {
	mkdir -p "$(dirname "${CODER_REPO_DIR}")"

	if [[ ! -e "${CODER_REPO_DIR}" ]]; then
		log "Cloning Coder into ${CODER_REPO_DIR}"
		git clone --filter=blob:none "${CODER_REPO_URL}" "${CODER_REPO_DIR}"
	elif [[ ! -d "${CODER_REPO_DIR}/.git" ]]; then
		fail "${CODER_REPO_DIR} exists but is not a Git repository"
	else
		printf 'already cloned: %s\n' "${CODER_REPO_DIR}"
	fi

	cd "${CODER_REPO_DIR}"
	if [[ -n "$(git status --short)" ]]; then
		fail "${CODER_REPO_DIR} has local changes; clean or preserve them before rerunning"
	fi

	log "Checking out ${CODER_REPO_REF}"
	git fetch origin "refs/heads/${CODER_REPO_REF}:refs/remotes/origin/${CODER_REPO_REF}"
	if git show-ref --verify --quiet "refs/heads/${CODER_REPO_REF}"; then
		git switch "${CODER_REPO_REF}"
		git merge --ff-only FETCH_HEAD
	else
		git switch --create "${CODER_REPO_REF}" --track "origin/${CODER_REPO_REF}"
	fi

	git config core.hooksPath scripts/githooks

	if grep -q 'coderService:' scripts/develop-local-cluster/main.go || ! grep -q 'name: CODER_URL' scripts/develop-local-cluster/main.go; then
		fail "${CODER_REPO_REF} does not contain the AI Gateway CODER_URL chart fix"
	fi
}

install_repo_tools() {
	log "Installing repository-pinned build tools"
	mise trust mise.toml
	mise install --locked \
		go \
		node \
		pnpm \
		helm \
		kind \
		terraform \
		protoc \
		protoc-gen-go
	eval "$(mise activate bash)"

	# Go-backed mise tools invoke go during installation, so install them only
	# after the repository-pinned Go version is active.
	mise install --locked \
		go:storj.io/drpc/cmd/protoc-gen-go-drpc \
		go:github.com/coder/sqlc/cmd/sqlc

	printf 'available: Go (%s)\n' "$(go version)"
	printf 'available: Node.js (%s)\n' "$(node --version)"
	printf 'available: pnpm (%s)\n' "$(pnpm --version)"
	printf 'available: Helm (%s)\n' "$(helm version --short)"
	printf 'available: kind (%s)\n' "$(kind version)"
	printf 'available: Terraform (%s)\n' "$(terraform version -json | jq -r '.terraform_version')"
	printf 'available: protoc (%s)\n' "$(protoc --version)"
	printf 'available: protoc-gen-go (%s)\n' "$(protoc-gen-go --version)"
	printf 'available: protoc-gen-go-drpc\n'
	printf 'available: sqlc (%s)\n' "$(sqlc version)"

	# Git does not preserve mtimes, so a fresh checkout can make committed
	# generated files appear older than their inputs and trigger code generation
	# during an ordinary build.
	make gen/mark-fresh
}

cluster_command() {
	./scripts/develop-local-cluster.sh --cluster-name "${CODER_DEV_CLUSTER_NAME}" "$@"
}

add_license_if_needed() {
	local existing
	local prompted_license=""

	existing="$(cluster_command coder licenses list --output json)"
	if [[ "$(jq 'length' <<<"${existing}")" -gt 0 ]]; then
		printf 'already configured: Coder license\n'
		return
	fi

	if [[ -n "${CODER_DEV_LICENSE_FILE}" && -n "${coder_dev_license}" ]]; then
		fail "set only one of CODER_DEV_LICENSE_FILE or CODER_DEV_LICENSE"
	fi

	if [[ -n "${CODER_DEV_LICENSE_FILE}" ]]; then
		[[ -r "${CODER_DEV_LICENSE_FILE}" ]] || fail "cannot read CODER_DEV_LICENSE_FILE: ${CODER_DEV_LICENSE_FILE}"
		cluster_command coder licenses add --file "${CODER_DEV_LICENSE_FILE}"
		return
	fi

	if [[ -n "${coder_dev_license}" ]]; then
		printf '%s\n' "${coder_dev_license}" | cluster_command coder licenses add --file -
		coder_dev_license=""
		unset coder_dev_license
		return
	fi

	[[ -t 0 ]] || fail "no license is installed; set CODER_DEV_LICENSE_FILE or CODER_DEV_LICENSE"
	printf 'Coder license: ' >&2
	IFS= read -r -s prompted_license
	printf '\n' >&2
	[[ -n "${prompted_license}" ]] || fail "license cannot be empty"
	printf '%s\n' "${prompted_license}" | cluster_command coder licenses add --file -
	prompted_license=""
}

generate_mtls_files() {
	local mtls_dir="$1"
	local server_name="coder.${CODER_DEV_CLUSTER_NAMESPACE}.svc.cluster.local"

	if [[ -s "${mtls_dir}/ca.crt" && -s "${mtls_dir}/ca.key" && -s "${mtls_dir}/server.crt" && -s "${mtls_dir}/server.key" && -s "${mtls_dir}/gateway.crt" && -s "${mtls_dir}/gateway.key" ]] &&
		openssl x509 -checkend 86400 -noout -in "${mtls_dir}/ca.crt" >/dev/null &&
		openssl x509 -checkend 86400 -noout -in "${mtls_dir}/server.crt" >/dev/null &&
		openssl x509 -checkend 86400 -noout -in "${mtls_dir}/gateway.crt" >/dev/null &&
		openssl verify -CAfile "${mtls_dir}/ca.crt" "${mtls_dir}/server.crt" >/dev/null &&
		openssl verify -CAfile "${mtls_dir}/ca.crt" "${mtls_dir}/gateway.crt" >/dev/null &&
		openssl x509 -in "${mtls_dir}/server.crt" -noout -ext subjectAltName | grep -Fq "DNS:${server_name}"; then
		printf 'already generated: mTLS certificates in %s\n' "${mtls_dir}"
		return
	fi

	log "Generating mTLS certificates"
	rm -rf "${mtls_dir}"
	mkdir -p "${mtls_dir}"
	chmod 700 "${mtls_dir}"

	openssl genrsa -out "${mtls_dir}/ca.key" 4096
	openssl req -x509 -new -sha256 -days "${MTLS_CERT_DAYS}" \
		-key "${mtls_dir}/ca.key" \
		-subj '/CN=Coder local development CA' \
		-out "${mtls_dir}/ca.crt"

	cat >"${mtls_dir}/server.ext" <<EOF
subjectAltName=DNS:coder,DNS:coder.${CODER_DEV_CLUSTER_NAMESPACE},DNS:coder.${CODER_DEV_CLUSTER_NAMESPACE}.svc,DNS:${server_name}
extendedKeyUsage=serverAuth
EOF
	openssl genrsa -out "${mtls_dir}/server.key" 2048
	openssl req -new -key "${mtls_dir}/server.key" -subj "/CN=${server_name}" -out "${mtls_dir}/server.csr"
	openssl x509 -req -sha256 -days "${MTLS_CERT_DAYS}" \
		-in "${mtls_dir}/server.csr" \
		-CA "${mtls_dir}/ca.crt" -CAkey "${mtls_dir}/ca.key" -CAcreateserial \
		-extfile "${mtls_dir}/server.ext" \
		-out "${mtls_dir}/server.crt"

	cat >"${mtls_dir}/gateway.ext" <<'EOF'
extendedKeyUsage=clientAuth
EOF
	openssl genrsa -out "${mtls_dir}/gateway.key" 2048
	openssl req -new -key "${mtls_dir}/gateway.key" -subj '/CN=coder-ai-gateway' -out "${mtls_dir}/gateway.csr"
	openssl x509 -req -sha256 -days "${MTLS_CERT_DAYS}" \
		-in "${mtls_dir}/gateway.csr" \
		-CA "${mtls_dir}/ca.crt" -CAkey "${mtls_dir}/ca.key" -CAcreateserial \
		-extfile "${mtls_dir}/gateway.ext" \
		-out "${mtls_dir}/gateway.crt"

	chmod 600 "${mtls_dir}"/*.key
}

write_mtls_values() {
	local mtls_dir="$1"

	cat >"${mtls_dir}/coder-values.yaml" <<'EOF'
coder:
  tls:
    secretNames:
      - coder-server-tls
  certs:
    secrets:
      - name: coder-client-ca
        key: ca.crt
  envFrom:
    - configMapRef:
        name: coder-mtls-config
EOF

	cat >"${mtls_dir}/gateway-values.yaml" <<EOF
coder:
  env:
    - name: CODER_URL
      value: https://coder.${CODER_DEV_CLUSTER_NAMESPACE}.svc.cluster.local:443

aigateway:
  coderTLS:
    caSecret:
      name: coder-client-ca
      key: ca.crt
    clientSecret:
      name: gateway-client-tls
      certKey: tls.crt
      keyKey: tls.key
EOF
}

apply_mtls_resources() {
	local context="$1"
	local mtls_dir="$2"
	local namespace="${CODER_DEV_CLUSTER_NAMESPACE}"

	log "Applying mTLS Secrets and configuration"
	kubectl --context "${context}" --namespace "${namespace}" create secret tls coder-server-tls \
		--cert "${mtls_dir}/server.crt" --key "${mtls_dir}/server.key" \
		--dry-run=client -o yaml | kubectl --context "${context}" apply -f -
	kubectl --context "${context}" --namespace "${namespace}" create secret generic coder-client-ca \
		--from-file=ca.crt="${mtls_dir}/ca.crt" \
		--dry-run=client -o yaml | kubectl --context "${context}" apply -f -
	kubectl --context "${context}" --namespace "${namespace}" create secret tls gateway-client-tls \
		--cert "${mtls_dir}/gateway.crt" --key "${mtls_dir}/gateway.key" \
		--dry-run=client -o yaml | kubectl --context "${context}" apply -f -
	kubectl --context "${context}" --namespace "${namespace}" create configmap coder-mtls-config \
		--from-literal=CODER_TLS_CLIENT_AUTH=require-and-verify \
		--from-literal=CODER_TLS_CLIENT_CA_FILE=/etc/ssl/certs/coder-client-ca.crt \
		--dry-run=client -o yaml | kubectl --context "${context}" apply -f -
}

remove_mtls_resources() {
	local context="$1"
	local namespace="${CODER_DEV_CLUSTER_NAMESPACE}"

	kubectl --context "${context}" --namespace "${namespace}" delete \
		secret/coder-server-tls \
		secret/coder-client-ca \
		secret/gateway-client-tls \
		configmap/coder-mtls-config \
		--ignore-not-found
}

validate_mtls() (
	local context="$1"
	local mtls_dir="$2"
	local server_name="coder.${CODER_DEV_CLUSTER_NAMESPACE}.svc.cluster.local"
	local port_forward_log="${mtls_dir}/port-forward.log"
	local port_forward_pid=""

	cleanup_port_forward() {
		if [[ -n "${port_forward_pid}" ]]; then
			kill "${port_forward_pid}" 2>/dev/null || true
			wait "${port_forward_pid}" 2>/dev/null || true
		fi
	}
	trap cleanup_port_forward EXIT

	log "Validating that Coder rejects clients without a certificate"
	kubectl --context "${context}" --namespace "${CODER_DEV_CLUSTER_NAMESPACE}" \
		port-forward --address 127.0.0.1 service/coder "${MTLS_VALIDATION_PORT}:443" >"${port_forward_log}" 2>&1 &
	port_forward_pid=$!

	for _ in {1..30}; do
		if grep -q 'Forwarding from' "${port_forward_log}"; then
			break
		fi
		kill -0 "${port_forward_pid}" 2>/dev/null || {
			cat "${port_forward_log}" >&2
			fail "Coder HTTPS port-forward exited"
		}
		sleep 1
	done
	grep -q 'Forwarding from' "${port_forward_log}" || fail "timed out waiting for Coder HTTPS port-forward"

	if curl --fail --silent --show-error --noproxy '*' \
		--cacert "${mtls_dir}/ca.crt" \
		--resolve "${server_name}:${MTLS_VALIDATION_PORT}:127.0.0.1" \
		"https://${server_name}:${MTLS_VALIDATION_PORT}/healthz" >/dev/null 2>&1; then
		fail "Coder accepted an HTTPS request without a client certificate"
	fi

	log "Validating Coder with the AI Gateway client certificate"
	curl --fail --silent --show-error --noproxy '*' \
		--cacert "${mtls_dir}/ca.crt" \
		--cert "${mtls_dir}/gateway.crt" \
		--key "${mtls_dir}/gateway.key" \
		--resolve "${server_name}:${MTLS_VALIDATION_PORT}:127.0.0.1" \
		"https://${server_name}:${MTLS_VALIDATION_PORT}/healthz" >/dev/null

	cleanup_port_forward
	port_forward_pid=""

	log "Validating standalone AI Gateway readiness"
	curl --fail --silent --show-error --retry 30 --retry-delay 2 --retry-connrefused \
		"http://127.0.0.1:${CODER_DEV_CLUSTER_GATEWAY_PORT}/readyz" >/dev/null
)

main() {
	local arch
	local cluster_existed=false
	local context="kind-${CODER_DEV_CLUSTER_NAME}"
	local mtls_dir

	CODER_DEV_CLUSTER_MTLS="$(normalize_bool "${CODER_DEV_CLUSTER_MTLS}")"

	cleanup_failed_bootstrap() {
		local exit_code="$?"
		if [[ "${exit_code}" -ne 0 && "${cluster_existed:-false}" == false ]] && kind get clusters 2>/dev/null | grep -Fxq "${CODER_DEV_CLUSTER_NAME}"; then
			printf '\nBootstrap failed, removing newly created cluster %s\n' "${CODER_DEV_CLUSTER_NAME}" >&2
			cluster_command down || true
		fi
		exit "${exit_code}"
	}

	install_base_packages
	arch="$(machine_arch)"
	install_mise
	configure_shell_path
	install_kubectl "${arch}"
	install_k9s "${arch}"
	check_docker
	checkout_coder
	install_repo_tools
	if kind get clusters 2>/dev/null | grep -Fxq "${CODER_DEV_CLUSTER_NAME}"; then
		cluster_existed=true
	fi
	trap cleanup_failed_bootstrap EXIT

	log "Creating the local cluster and Coder control plane"
	cluster_command up --no-license-prompt
	add_license_if_needed

	mtls_dir="${CODER_REPO_DIR}/.coderv2/clusters/${CODER_DEV_CLUSTER_NAME}/mtls"
	if [[ "${CODER_DEV_CLUSTER_MTLS}" == true ]]; then
		generate_mtls_files "${mtls_dir}"
		write_mtls_values "${mtls_dir}"
		apply_mtls_resources "${context}" "${mtls_dir}"

		log "Deploying premium components with gateway-to-Coder mTLS"
		cluster_command \
			--coder-values "${mtls_dir}/coder-values.yaml" \
			--gateway-values "${mtls_dir}/gateway-values.yaml" \
			up --no-license-prompt

		log "Restarting Coder and AI Gateway to load the applied certificates"
		kubectl --context "${context}" --namespace "${CODER_DEV_CLUSTER_NAMESPACE}" rollout restart \
			deployment/coder deployment/coder-ai-gateway
		kubectl --context "${context}" --namespace "${CODER_DEV_CLUSTER_NAMESPACE}" rollout status \
			deployment/coder --timeout=5m
		kubectl --context "${context}" --namespace "${CODER_DEV_CLUSTER_NAMESPACE}" rollout status \
			deployment/coder-ai-gateway --timeout=5m

		validate_mtls "${context}" "${mtls_dir}"
	else
		log "Deploying premium components without mTLS"
		cluster_command up --no-license-prompt
		remove_mtls_resources "${context}"
		rm -rf "${mtls_dir}"
	fi

	log "Local cluster is ready"
	cluster_command info
	printf 'Load developer tools in the current shell with: export PATH=%q\n' "${HOME}/.local/bin:${HOME}/.local/share/mise/shims:${PATH}"
	printf 'Open k9s with: k9s --context %s\n' "${context}"
	printf 'Remove everything with: cd %q && ./scripts/develop-local-cluster.sh --cluster-name %q down\n' \
		"${CODER_REPO_DIR}" "${CODER_DEV_CLUSTER_NAME}"
	trap - EXIT
}

main "$@"
