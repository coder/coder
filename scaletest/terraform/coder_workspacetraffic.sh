#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
	echo "Usage: $0 <loadtest name>"
	exit 1
fi

# Allow toggling verbose output
[[ -n ${VERBOSE:-} ]] && set -x

LOADTEST_NAME="$1"
CODER_TOKEN=$(./coder_shim.sh tokens create)
CODER_URL="http://coder.coder-${LOADTEST_NAME}.svc.cluster.local"
export KUBECONFIG="${PWD}/.coderv2/${LOADTEST_NAME}-cluster.kubeconfig"

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: coder-scaletest-workspace-traffic
  namespace: coder-${LOADTEST_NAME}
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
            - ${LOADTEST_NAME}-misc
  containers:
  - command:
    - sh
    - -c
    - "curl -fsSL $CODER_URL/bin/coder-linux-amd64 -o /tmp/coder && chmod +x /tmp/coder && /tmp/coder --url=$CODER_URL --token=$CODER_TOKEN scaletest workspace-traffic"
    env:
    - name: CODER_URL
      value: $CODER_URL
    - name: CODER_TOKEN
      value: $CODER_TOKEN
    - name: CODER_SCALETEST_PROMETHEUS_ADDRESS
      value: "0.0.0.0:21112"
    - name: CODER_SCALETEST_JOB_TIMEOUT
      value: "30m"
    - name: CODER_SCALETEST_CONCURRENCY
      value: "0"
    - name: CODER_SCALETEST_WORKSPACE_TRAFFIC_BYTES_PER_TICK
      value: "2048"
    ports:
    - containerPort: 21112
      name: prometheus-http
      protocol: TCP
    name: cli
    image: docker.io/codercom/enterprise-minimal:ubuntu
---
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  namespace: coder-${LOADTEST_NAME}
  name: coder-workspacetraffic-monitoring
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: coder-scaletest-workspace-traffic
  podMetricsEndpoints:
  - port: prometheus-http
    interval: 15s
EOF
