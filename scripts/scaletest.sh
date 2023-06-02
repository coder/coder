#!/usr/bin/env bash

if [[ $# -lt 2 ]]; then
	echo "Usage: $0 <name> <scenario> <num_workspaces>"
fi

[[ -n ${VERBOSE:-} ]] && set -x
set -euo pipefail

SCALETEST_NAME="$1"
SCALETEST_SCENARIO="$2"
SCALETEST_NUM_WORKSPACES="$3"
SCALETEST_PROJECT="${SCALETEST_PROJECT:-}"
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
SCALETEST_SCENARIO_VARS="${PROJECT_ROOT}/scaletest/terraform/scenario-${SCALETEST_SCENARIO}.tfvars"
SCALETEST_SECRETS="${PROJECT_ROOT}/scaletest/terraform/secrets.tfvars"
SCALETEST_SECRETS_TEMPLATE="${PROJECT_ROOT}/scaletest/terraform/secrets.tfvars.tpl"
SCALETEST_SKIP_CLEANUP="${SCALETEST_SKIP_CLEANUP:-}"

if [[ "${SCALETEST_SKIP_CLEANUP}" == "true" ]]; then
	echo "WARNING: you told me to not clean up after myself, so this is now your job!"
fi

if [[ -z "${SCALETEST_PROJECT}" ]]; then
	echo "Environment variable SCALETEST_PROJECT not set. Please set it and try again."
	exit 1
fi

if [[ ! -f "${SCALETEST_SCENARIO_VARS}" ]] ; then
	echo "No definition for scenario ${SCALETEST_SCENARIO} exists. Please create it and try again"
	exit 1
fi

echo "Writing scaletest secrets to file."
SCALETEST_NAME="${SCALETEST_NAME}" envsubst < "${SCALETEST_SECRETS_TEMPLATE}" > "${SCALETEST_SECRETS}"

pushd "${PROJECT_ROOT}/scaletest/terraform"

echo "Initializing terraform."
terraform init

echo "Setting up infrastructure."
terraform plan --var-file="${SCALETEST_SCENARIO_VARS}" --var-file="${SCALETEST_SECRETS}" -out=scaletest.tfplan
terraform apply -auto-approve scaletest.tfplan

SCALETEST_CODER_URL=$(<./.coderv2/url)
attempt_counter=0
max_attempts=6 # 60 seconds
echo -n "Waiting for Coder deployment at ${SCALETEST_CODER_URL} to become ready"
until curl --output /dev/null --silent --fail "${SCALETEST_CODER_URL}/healthz"; do
	if [[ $attempt_counter -eq $max_attempts ]]; then
		echo
		echo "Max attempts reached."
		exit 1
	fi

	echo -n '.'
	attempt_counter=$((attempt_counter+1))
	sleep 10
done

echo "Initializing Coder deployment."
./coder_init.sh "${SCALETEST_CODER_URL}"

echo "Creating ${SCALETEST_NUM_WORKSPACES} workspaces."
./coder_shim.sh scaletest create-workspaces \
	--count "${SCALETEST_NUM_WORKSPACES}" \
	--template=kubernetes \
	--concurrency 10 \
	--no-cleanup

echo "Sleeping 10 minutes to establish a baseline measurement."
sleep 600

echo "Sending traffic to workspaces"
./coder_workspacetraffic.sh "${SCALETEST_NAME}"
export KUBECONFIG="${PWD}/.coderv2/${SCALETEST_NAME}-cluster.kubeconfig"
kubectl -n "coder-${SCALETEST_NAME}" wait pods coder-scaletest-workspace-traffic --for condition=Ready
kubectl -n "coder-${SCALETEST_NAME}" logs -f pod/coder-scaletest-workspace-traffic

echo "Starting pprof"
kubectl -n "coder-${SCALETEST_NAME}" port-forward deployment/coder 6061:6060 &
pfpid=$!
trap 'kill $pfpid' EXIT

echo -n "Waiting for pprof endpoint to become available"
pprof_attempt_counter=0
while ! timeout 1 bash -c "echo > /dev/tcp/localhost/6061"; do
	if [[ $pprof_attempt_counter -eq 10 ]]; then
		echo
		echo "pprof failed to become ready in time!"
		exit 1
	fi
	sleep 3
	echo -n "."
done
echo "Taking pprof snapshots"
curl --silent --fail --output "${SCALETEST_NAME}-heap.pprof.gz" http://localhost:6061/debug/pprof/heap
curl --silent --fail --output "${SCALETEST_NAME}-goroutine.pprof.gz" http://localhost:6061/debug/pprof/goroutine
kill $pfpid

if [[ "${SCALETEST_SKIP_CLEANUP}" == "true" ]]; then
	echo "Leaving resources up for you to inspect."
	echo "Please don't forget to clean up afterwards!"
	exit 0
fi

echo "Cleaning up"
terraform apply --destroy --var-file="${SCALETEST_SCENARIO_VARS}" --var-file="${SCALETEST_SECRETS}" --auto-approve
