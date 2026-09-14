#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
# shellcheck source=scripts/lib.sh
source "${PROJECT_ROOT}/scripts/lib.sh"

# Allow toggling verbose output
[[ -n ${VERBOSE:-} ]] && set -x

SCALETEST_NAME="${SCALETEST_NAME:-}"
SCALETEST_TRAFFIC_BYTES_PER_TICK="${SCALETEST_TRAFFIC_BYTES_PER_TICK:-1024}"
SCALETEST_TRAFFIC_TICK_INTERVAL="${SCALETEST_TRAFFIC_TICK_INTERVAL:-100ms}"

script_name=$(basename "$0")
args="$(getopt -o "" -l help,name:,traffic-bytes-per-tick:,traffic-tick-interval:, -- "$@")"
eval set -- "$args"
while true; do
	case "$1" in
	--help)
		echo "Usage: $script_name --name <name> [--traffic-bytes-per-tick <bytes_per-tick>] [--traffic-tick-interval <ticks_per_second]"
		exit 1
		;;
	--name)
		SCALETEST_NAME="$2"
		shift 2
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

dependencies kubectl

if [[ -z "${SCALETEST_NAME}" ]]; then
	echo "Must specify --name"
	exit 1
fi

CODER_TOKEN=$("${PROJECT_ROOT}/scaletest/lib/coder_shim.sh" tokens create)
CODER_URL="http://coder.coder-${SCALETEST_NAME}.svc.cluster.local"
export KUBECONFIG="${PROJECT_ROOT}/scaletest/.coderv2/${SCALETEST_NAME}-cluster.kubeconfig"

# Clean up any pre-existing pods
kubectl -n "coder-${SCALETEST_NAME}" delete pod coder-scaletest-workspace-traffic --force || true

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: coder-scaletest-workspace-traffic
  namespace: coder-${SCALETEST_NAME}
  labels:
    app.kubernetes.io/name: coder-scaletest-workspace-traffic
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: cloud.google.com/gke-nodepool
            operator: In
            values:
            - ${SCALETEST_NAME}-misc
  containers:
  - command:
    - sh
    - -c
    - "curl -fsSL $CODER_URL/bin/coder-linux-amd64 -o /tmp/coder && chmod +x /tmp/coder && /tmp/coder --verbose --url=$CODER_URL --token=$CODER_TOKEN exp scaletest workspace-traffic --concurrency=0 --bytes-per-tick=${SCALETEST_TRAFFIC_BYTES_PER_TICK} --tick-interval=${SCALETEST_TRAFFIC_TICK_INTERVAL} --scaletest-prometheus-wait=60s"
    env:
    - name: CODER_URL
      value: $CODER_URL
    - name: CODER_TOKEN
      value: $CODER_TOKEN
    - name: CODER_SCALETEST_PROMETHEUS_ADDRESS
      value: "0.0.0.0:21112"
    - name: CODER_SCALETEST_JOB_TIMEOUT
      value: "30m"
    ports:
    - containerPort: 21112
      name: prometheus-http
      protocol: TCP
    name: cli
    image: docker.io/codercom/enterprise-minimal:ubuntu
  restartPolicy: Never
---
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: coder-${SCALETEST_NAME}
  name: coder-workspacetraffic-monitoring
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: coder-scaletest-workspace-traffic
  podMetricsEndpoints:
  - port: prometheus-http
    interval: 15s
EOF
