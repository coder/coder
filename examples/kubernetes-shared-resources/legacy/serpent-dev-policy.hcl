module "coder_network_policy" {
  source = "./module"

  # Git and Go dependencies use HTTPS only.
  allowed_https_hosts = ["github.com", "proxy.golang.org", "sum.golang.org"]
  dns_query_names     = []

  # Replace this local demo address with the exact pod-reachable Coder address.
  control_plane_allowances = [{
    cidrs = ["172.20.0.3/32"]
    ports = [{ port = 3100, protocol = "TCP" }]
  }]
  relay_allowances   = []
  direct_p2p_enabled = false
}
