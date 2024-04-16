#!/usr/bin/env bash

[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
# shellcheck source=scripts/lib.sh
source "${PROJECT_ROOT}/scripts/lib.sh"

DRY_RUN="${DRY_RUN:-0}"
SCALETEST_NAME="${SCALETEST_NAME:-}"
SCALETEST_NUM_WORKSPACES="${SCALETEST_NUM_WORKSPACES:-}"
SCALETEST_SCENARIO="${SCALETEST_SCENARIO:-}"
SCALETEST_PROJECT="${SCALETEST_PROJECT:-}"
SCALETEST_PROMETHEUS_REMOTE_WRITE_USER="${SCALETEST_PROMETHEUS_REMOTE_WRITE_USER:-}"
SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD="${SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD:-}"
SCALETEST_CODER_LICENSE="${SCALETEST_CODER_LICENSE:-}"
SCALETEST_SKIP_CLEANUP="${SCALETEST_SKIP_CLEANUP:-0}"
SCALETEST_CREATE_CONCURRENCY="${SCALETEST_CREATE_CONCURRENCY:-10}"
SCALETEST_TRAFFIC_BYTES_PER_TICK="${SCALETEST_TRAFFIC_BYTES_PER_TICK:-1024}"
SCALETEST_TRAFFIC_TICK_INTERVAL="${SCALETEST_TRAFFIC_TICK_INTERVAL:-10s}"
SCALETEST_DESTROY="${SCALETEST_DESTROY:-0}"

script_name=$(basename "$0")
args="$(getopt -o "" -l create-concurrency:,destroy,dry-run,help,name:,num-workspaces:,project:,scenario:,skip-cleanup,traffic-bytes-per-tick:,traffic-tick-interval:, -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--create-concurrency)
		SCALETEST_CREATE_CONCURRENCY="$2"
		shift 2
		;;
	--destroy)
		SCALETEST_DESTROY=1
		shift
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--help)
		echo "Usage: $script_name --name <name> --project <project> --num-workspaces <num-workspaces> --scenario <scenario> [--create-concurrency <create-concurrency>] [--destroy] [--dry-run] [--skip-cleanup] [--traffic-bytes-per-tick <number>] [--traffic-tick-interval <duration>]"
		exit 1
		;;
	--name)
		SCALETEST_NAME="$2"
		shift 2
		;;
	--num-workspaces)
		SCALETEST_NUM_WORKSPACES="$2"
		shift 2
		;;
	--project)
		SCALETEST_PROJECT="$2"
		shift 2
		;;
	--scenario)
		SCALETEST_SCENARIO="$2"
		shift 2
		;;
	--skip-cleanup)
		SCALETEST_SKIP_CLEANUP=1
		shift
		;;
	--traffic-bytes-per-tick)
		SCALETEST_TRAFFIC_BYTES_PER_TICK="$2"
		shift 2
		;;
	--traffic-tick-interval)
		SCALETEST_TRAFFIC_TICK_INTERVAL="$2"
		shift 2
		;;
	--)
		shift
		break
		;;
	*)
		error "Unrecognized option: $1"
		;;
	esac
done

dependencies gcloud kubectl terraform

if [[ -z "${SCALETEST_NAME}" ]]; then
	echo "Must specify --name"
	exit 1
fi

if [[ -z "${SCALETEST_PROJECT}" ]]; then
	echo "Must specify --project"
	exit 1
fi

if [[ -z "${SCALETEST_NUM_WORKSPACES}" ]]; then
	echo "Must specify --num-workspaces"
	exit 1
fi

if [[ -z "${SCALETEST_SCENARIO}" ]]; then
	echo "Must specify --scenario"
	exit 1
fi

if [[ -z "${SCALETEST_PROMETHEUS_REMOTE_WRITE_USER}" ]] || [[ -z "${SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD}" ]]; then
	echo "SCALETEST_PROMETHEUS_REMOTE_WRITE_USER or SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD not specified."
	echo "No prometheus metrics will be collected!"
	read -p "Continue (y/N)? " -n1 -r
	if [[ "${REPLY}" != [yY] ]]; then
		exit 1
	fi
fi

SCALETEST_SCENARIO_VARS="${PROJECT_ROOT}/scaletest/terraform/scenario-${SCALETEST_SCENARIO}.tfvars"
if [[ ! -f "${SCALETEST_SCENARIO_VARS}" ]]; then
	echo "Scenario ${SCALETEST_SCENARIO_VARS} not found."
	echo "Please create it or choose another scenario:"
	find "${PROJECT_ROOT}/scaletest/terraform" -type f -name 'scenario-*.tfvars'
	exit 1
fi

if [[ "${SCALETEST_SKIP_CLEANUP}" == 1 ]]; then
	log "WARNING: you told me to not clean up after myself, so this is now your job!"
fi

CONFIG_DIR="${PROJECT_ROOT}/scaletest/.coderv2"
if [[ -d "${CONFIG_DIR}" ]] && files=$(ls -qAH -- "${CONFIG_DIR}") && [[ -z "$files" ]]; then
	echo "Cleaning previous configuration"
	maybedryrun "$DRY_RUN" rm -fv "${CONFIG_DIR}/*"
fi
maybedryrun "$DRY_RUN" mkdir -p "${CONFIG_DIR}"

SCALETEST_SCENARIO_VARS="${PROJECT_ROOT}/scaletest/terraform/scenario-${SCALETEST_SCENARIO}.tfvars"
SCALETEST_SECRETS="${PROJECT_ROOT}/scaletest/terraform/secrets.tfvars"
SCALETEST_SECRETS_TEMPLATE="${PROJECT_ROOT}/scaletest/terraform/secrets.tfvars.tpl"

log "Writing scaletest secrets to file."
SCALETEST_NAME="${SCALETEST_NAME}" \
	SCALETEST_PROJECT="${SCALETEST_PROJECT}" \
	SCALETEST_PROMETHEUS_REMOTE_WRITE_USER="${SCALETEST_PROMETHEUS_REMOTE_WRITE_USER}" \
	SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD="${SCALETEST_PROMETHEUS_REMOTE_WRITE_PASSWORD}" \
	envsubst <"${SCALETEST_SECRETS_TEMPLATE}" >"${SCALETEST_SECRETS}"

pushd "${PROJECT_ROOT}/scaletest/terraform"

echo "Initializing terraform."
maybedryrun "$DRY_RUN" terraform init

echo "Setting up infrastructure."
maybedryrun "$DRY_RUN" terraform apply --var-file="${SCALETEST_SCENARIO_VARS}" --var-file="${SCALETEST_SECRETS}" --var state=started --auto-approve

if [[ "${DRY_RUN}" != 1 ]]; then
	SCALETEST_CODER_URL=$(<"${CONFIG_DIR}/url")
else
	SCALETEST_CODER_URL="http://coder.dryrun.local:3000"
fi
KUBECONFIG="${PROJECT_ROOT}/scaletest/.coderv2/${SCALETEST_NAME}-cluster.kubeconfig"
echo "Waiting for Coder deployment at ${SCALETEST_CODER_URL} to become ready"
max_attempts=10
for attempt in $(seq 1 $max_attempts); do
	maybedryrun "$DRY_RUN" curl --silent --fail --output /dev/null "${SCALETEST_CODER_URL}/api/v2/buildinfo"
	curl_status=$?
	if [[ $curl_status -eq 0 ]]; then
		break
	fi
	if attempt -eq $max_attempts; then
		echo
		echo "Coder deployment failed to become ready in time!"
		exit 1
	fi
	echo "Coder deployment not ready yet (${attempt}/${max_attempts}), sleeping 3 seconds"
	maybedryrun "$DRY_RUN" sleep 3
done

echo "Initializing Coder deployment."
DRY_RUN="$DRY_RUN" "${PROJECT_ROOT}/scaletest/lib/coder_init.sh" "${SCALETEST_CODER_URL}"

if [[ -n "${SCALETEST_CODER_LICENSE}" ]]; then
	echo "Applying Coder Enterprise License"
	DRY_RUN="$DRY_RUN" "${PROJECT_ROOT}/scaletest/lib/coder_shim.sh" license add -l "${SCALETEST_CODER_LICENSE}"
fi

echo "Creating ${SCALETEST_NUM_WORKSPACES} workspaces."
DRY_RUN="$DRY_RUN" "${PROJECT_ROOT}/scaletest/lib/coder_shim.sh" exp scaletest create-workspaces \
	--count "${SCALETEST_NUM_WORKSPACES}" \
	--template=kubernetes \
	--concurrency "${SCALETEST_CREATE_CONCURRENCY}" \
	--no-cleanup

echo "Sleeping 10 minutes to establish a baseline measurement."
maybedryrun "$DRY_RUN" sleep 600

echo "Sending traffic to workspaces"
maybedryrun "$DRY_RUN" "${PROJECT_ROOT}/scaletest/lib/coder_workspacetraffic.sh" \
	--name "${SCALETEST_NAME}" \
	--traffic-bytes-per-tick "${SCALETEST_TRAFFIC_BYTES_PER_TICK}" \
	--traffic-tick-interval "${SCALETEST_TRAFFIC_TICK_INTERVAL}"
maybedryrun "$DRY_RUN" kubectl --kubeconfig="${KUBECONFIG}" -n "coder-${SCALETEST_NAME}" wait pods coder-scaletest-workspace-traffic --for condition=Ready

echo "Sleeping 15 minutes for traffic generation"
maybedryrun "$DRY_RUN" sleep 900

echo "Starting pprof"
maybedryrun "$DRY_RUN" kubectl -n "coder-${SCALETEST_NAME}" port-forward deployment/coder 6061:6060 &
pfpid=$!
maybedryrun "$DRY_RUN" trap "kill $pfpid" EXIT

echo "Waiting for pprof endpoint to become available"
pprof_attempt_counter=0
while ! maybedryrun "$DRY_RUN" timeout 1 bash -c "echo > /dev/tcp/localhost/6061"; do
	if [[ $pprof_attempt_counter -eq 10 ]]; then
		echo
		echo "pprof failed to become ready in time!"
		exit 1
	fi
	((pprof_attempt_counter += 1))
	maybedryrun "$DRY_RUN" sleep 3
done

echo "Taking pprof snapshots"
maybedryrun "$DRY_RUN" curl --silent --fail --output "${SCALETEST_NAME}-heap.pprof.gz" http://localhost:6061/debug/pprof/heap
maybedryrun "$DRY_RUN" curl --silent --fail --output "${SCALETEST_NAME}-goroutine.pprof.gz" http://localhost:6061/debug/pprof/goroutine
# No longer need to port-forward
maybedryrun "$DRY_RUN" kill "$pfpid"
maybedryrun "$DRY_RUN" trap - EXIT

if [[ "${SCALETEST_SKIP_CLEANUP}" == 1 ]]; then
	echo "Leaving resources up for you to inspect."
	echo "Please don't forget to clean up afterwards:"
	echo "cd terraform && terraform destroy --var-file=${SCALETEST_SCENARIO_VARS} --var-file=${SCALETEST_SECRETS} --auto-approve"
	exit 0
fi

if [[ "${SCALETEST_DESTROY}" == 1 ]]; then
	echo "Destroying infrastructure"
	maybedryrun "$DRY_RUN" terraform destroy --var-file="${SCALETEST_SCENARIO_VARS}" --var-file="${SCALETEST_SECRETS}" --auto-approve
else
	echo "Scaling down infrastructure"
	maybedryrun "$DRY_RUN" terraform apply --var-file="${SCALETEST_SCENARIO_VARS}" --var-file="${SCALETEST_SECRETS}" --var state=stopped --auto-approve
fi
