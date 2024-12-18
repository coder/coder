locals {
  pprof_interval = "30s"
  pprof_duration = "30m"

  pprof_ports = {
    primary = 6061
    europe  = 7061
    asia    = 8061
  }
}

resource "local_file" "kubeconfig" {
  for_each = local.deployments

  content = templatefile("${path.module}/kubeconfig.tftpl", {
    name                   = google_container_cluster.cluster[each.key].name
    endpoint               = "https://${google_container_cluster.cluster[each.key].endpoint}"
    cluster_ca_certificate = google_container_cluster.cluster[each.key].master_auth[0].cluster_ca_certificate
    access_token           = data.google_client_config.default.access_token
  })
  filename = "${path.module}/.coderv2/kubeconfig/${each.key}.yaml"
}

resource "null_resource" "pprof" {
  for_each = {
    primary = {}
  }
  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOF
set -e

pids=()
ports=()
declare -A pods=()
next_port=${local.pprof_ports[each.key]}
export KUBECONFIG="${path.module}/.coderv2/kubeconfig/${each.key}.yaml"

for pod in $(kubectl get pods --kubeconfig="$${KUBECONFIG}" -n coder -l app.kubernetes.io/name=coder -o jsonpath='{.items[*].metadata.name}'); do
  echo "Port forwarding cluster ${each.key} $${pod} to $${next_port}"
  kubectl --kubeconfig="$${KUBECONFIG}" -n coder port-forward "$${pod}" "$${next_port}:6060" &
  pids+=($!)
  ports+=("$${next_port}")
  pods[$${next_port}]="$${pod}"
  next_port=$((next_port + 1))
done

trap 'trap - EXIT; kill -INT "$${pids[@]}"' INT EXIT

mkdir -p ${path.module}/.coderv2/pprof
{
  while :; do
    sleep ${local.pprof_interval}
    start="$(date +%s)"
    for port in "$${ports[@]}"; do
      echo "Fetching pprof data for ${each.key}-$${start}-$${pods[$${port}]} on port $${port}"
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-allocs.pprof.gz" http://localhost:$${port}/debug/pprof/allocs
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-block.pprof.gz" http://localhost:$${port}/debug/pprof/block
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-heap.pprof.gz" http://localhost:$${port}/debug/pprof/heap
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-goroutine.pprof.gz" http://localhost:$${port}/debug/pprof/goroutine
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-mutex.pprof.gz" http://localhost:$${port}/debug/pprof/mutex
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-profile_seconds_10.pprof.gz" http://localhost:$${port}/debug/pprof/profile?seconds=10
      curl --silent --fail --output "${path.module}/.coderv2/pprof/${each.key}-$${start}-$${pods[$${port}]}-trace_seconds_5.pprof.gz" http://localhost:$${port}/debug/pprof/trace?seconds=5
    done
  done
} &
pprof_pid=$!

sleep ${local.pprof_duration}

kill -INT $pprof_pid
EOF
  }

  depends_on = [time_sleep.wait_baseline, local_file.kubeconfig]
}
