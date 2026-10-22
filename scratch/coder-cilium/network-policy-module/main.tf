locals {
  policy_name = "coder-policy-${var.policy_id}"
  dns_names   = sort(tolist(setunion(var.allowed_https_hosts, var.dns_query_names)))

  dns_egress = length(local.dns_names) == 0 ? [] : [{
    toEndpoints = [{
      matchLabels = {
        "k8s:io.kubernetes.pod.namespace" = "kube-system"
        "k8s:k8s-app"                     = "kube-dns"
      }
    }]
    toPorts = [{
      ports = [
        { port = "53", protocol = "UDP" },
        { port = "53", protocol = "TCP" },
      ]
      rules = { dns = [for host in local.dns_names : { matchName = host }] }
    }]
  }]

  https_egress = length(var.allowed_https_hosts) == 0 ? [] : [{
    toFQDNs = [for host in sort(tolist(var.allowed_https_hosts)) : { matchName = host }]
    toPorts = [{
      ports       = [{ port = "443", protocol = "TCP" }]
      serverNames = sort(tolist(var.allowed_https_hosts))
    }]
  }]

  dependency_egress = [for rule in concat(var.control_plane_allowances, var.relay_allowances) : merge(
    length(rule.endpoint_selectors) > 0 ? { toEndpoints = [for labels in rule.endpoint_selectors : { matchLabels = labels }] } : {},
    length(rule.cidrs) > 0 ? { toCIDR = sort(tolist(rule.cidrs)) } : {},
    { toPorts = [{ ports = [for port in rule.ports : { port = tostring(port.port), protocol = port.protocol }] }] },
  )]

  p2p_ports = [{ ports = [{ port = "1024", endPort = 65535, protocol = "UDP" }] }]
  p2p_egress = var.direct_p2p_enabled ? [{
    toCIDR  = sort(tolist(var.direct_p2p_cidrs))
    toPorts = local.p2p_ports
  }] : []
  p2p_ingress = var.direct_p2p_enabled ? [{
    fromCIDR = sort(tolist(var.direct_p2p_cidrs))
    toPorts  = local.p2p_ports
  }] : []

  manifest = {
    apiVersion = "cilium.io/v2"
    kind       = "CiliumNetworkPolicy"
    metadata = {
      name      = local.policy_name
      namespace = var.namespace
      labels    = { "coder.policy_id" = var.policy_id }
    }
    spec = {
      description = "Coder template policy ${var.policy_id}"
      endpointSelector = {
        matchExpressions = [{
          key      = "k8s:coder.template_id"
          operator = "In"
          values   = sort(tolist(var.template_ids))
        }]
      }
      enableDefaultDeny = { ingress = true, egress = true }
      ingress           = local.p2p_ingress
      egress            = concat(local.dns_egress, local.https_egress, local.dependency_egress, local.p2p_egress)
    }
  }
}

resource "kubernetes_manifest" "policy" {
  # No resource is safer than emitting an empty selector that could match all pods.
  for_each = length(var.template_ids) == 0 ? {} : { active = true }
  manifest = local.manifest
}
