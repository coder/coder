output "coder_db_url" {
  description = "URL of the database for Coder."
  value       = local.coder_db_url
  sensitive   = true
}

output "coder_address" {
  description = "IP address to use for the Coder service."
  value       = google_compute_address.coder.address
}

output "kubernetes_kubeconfig_path" {
  description = "Kubeconfig path."
  value       = local.cluster_kubeconfig_path
}

output "kubernetes_nodepool_coder" {
  description = "Name of the nodepool on which to run Coder."
  value       = google_container_node_pool.coder.name
}

output "kubernetes_nodepool_misc" {
  description = "Name of the nodepool on which to run everything else."
  value       = google_container_node_pool.misc.name
}

output "kubernetes_nodepool_workspaces" {
  description = "Name of the nodepool on which to run workspaces."
  value       = google_container_node_pool.workspaces.name
}

output "prometheus_external_label_cluster" {
  description = "Value for the Prometheus external label named cluster."
  value       = google_container_cluster.primary.name
}

output "prometheus_postgres_dbname" {
  description = "Name of the database for Prometheus to monitor."
  value       = google_sql_database.coder.name
}

output "prometheus_postgres_host" {
  description = "Hostname of the database for Prometheus to connect to."
  value       = google_sql_database_instance.db.private_ip_address
}

output "prometheus_postgres_password" {
  description = "Postgres password for Prometheus."
  value       = random_password.prometheus-postgres-password.result
  sensitive   = true
}

output "prometheus_postgres_user" {
  description = "Postgres username for Prometheus."
  value       = google_sql_user.prometheus.name
}

resource "local_file" "outputs" {
  filename = "${path.module}/../../.coderv2/infra_outputs.tfvars"
  content  = <<EOF
  coder_db_url = "${local.coder_db_url}"
  coder_address = "${google_compute_address.coder.address}"
  kubernetes_kubeconfig_path = "${local.cluster_kubeconfig_path}"
  kubernetes_nodepool_coder = "${google_container_node_pool.coder.name}"
  kubernetes_nodepool_misc = "${google_container_node_pool.misc.name}"
  kubernetes_nodepool_workspaces = "${google_container_node_pool.workspaces.name}"
  prometheus_external_label_cluster = "${google_container_cluster.primary.name}"
  prometheus_postgres_dbname = "${google_sql_database.coder.name}"
  prometheus_postgres_host = "${google_sql_database_instance.db.private_ip_address}"
  prometheus_postgres_password = "${random_password.prometheus-postgres-password.result}"
  prometheus_postgres_user = "${google_sql_user.prometheus.name}"
EOF
}
