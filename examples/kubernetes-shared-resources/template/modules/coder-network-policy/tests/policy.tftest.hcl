mock_provider "kubernetes" {}

variables {
  policy_id           = "11111111-1111-1111-1111-111111111111"
  namespace           = "coder"
  template_ids        = ["22222222-2222-2222-2222-222222222222"]
  allowed_https_hosts = ["api.github.com", "google.com"]
  direct_p2p_enabled  = false
}

run "restricted_https" {
  command = plan

  assert {
    condition     = length(kubernetes_manifest.policy) == 1 && output.manifest.kind == "CiliumNetworkPolicy" && output.manifest.metadata.namespace == "coder"
    error_message = "Exactly one namespace-scoped Cilium policy must be managed."
  }
  assert {
    condition     = output.manifest.spec.enableDefaultDeny.ingress && output.manifest.spec.enableDefaultDeny.egress && length(output.manifest.spec.ingress) == 0
    error_message = "Both directions must default deny with no ingress when P2P is disabled."
  }
  assert {
    condition     = output.manifest.spec.endpointSelector.matchExpressions[0].key == "k8s:coder.template_id" && output.manifest.spec.endpointSelector.matchExpressions[0].operator == "In" && output.manifest.spec.endpointSelector.matchExpressions[0].values == sort(tolist(var.template_ids))
    error_message = "The selector must use attached stable template IDs."
  }
  assert {
    condition     = output.manifest.spec.egress[0].toEndpoints[0].matchLabels["k8s:io.kubernetes.pod.namespace"] == "kube-system" && output.manifest.spec.egress[0].toEndpoints[0].matchLabels["k8s:k8s-app"] == "kube-dns"
    error_message = "DNS must target only trusted kube-system CoreDNS."
  }
  assert {
    condition     = toset([for rule in output.manifest.spec.egress[0].toPorts[0].rules.dns : rule.matchName]) == var.allowed_https_hosts && toset([for rule in output.manifest.spec.egress[1].toFQDNs : rule.matchName]) == toset(output.manifest.spec.egress[1].toPorts[0].serverNames) && output.manifest.spec.egress[1].toPorts[0].serverNames == sort(tolist(var.allowed_https_hosts))
    error_message = "DNS, FQDN, and TLS SNI must share the exact HTTPS hostname allowlist."
  }
  assert {
    condition     = length(output.manifest.spec.egress) == 2 && output.manifest.spec.egress[1].toPorts[0].ports[0].port == "443" && output.manifest.spec.egress[1].toPorts[0].ports[0].protocol == "TCP"
    error_message = "There must be no implicit dependency, HTTP, QUIC, or external DNS allowance."
  }
}

run "empty_templates" {
  command = plan
  variables { template_ids = [] }
  assert {
    condition     = length(kubernetes_manifest.policy) == 0 && output.manifest == null && output.policy_name == null
    error_message = "Detaching all templates must remove the resource, never select all pods."
  }
}

run "empty_allowlist" {
  command = plan
  variables { allowed_https_hosts = [] }
  assert {
    condition     = length(output.manifest.spec.egress) == 0
    error_message = "An empty allowlist must not produce unrestricted DNS or HTTPS rules."
  }
}

run "explicit_dependencies" {
  command = plan
  variables {
    dns_query_names = ["coder.coder.svc.cluster.local"]
    control_plane_allowances = [{
      endpoint_selectors = [{
        "k8s:io.kubernetes.pod.namespace" = "coder"
        "k8s:app.kubernetes.io/name"      = "coder"
      }]
      ports = [{ port = 8080 }]
    }]
    relay_allowances = [{ cidrs = ["192.0.2.10/32"], ports = [{ port = 443 }] }]
  }
  assert {
    condition     = length(output.manifest.spec.egress) == 4 && output.manifest.spec.egress[2].toEndpoints[0].matchLabels["k8s:io.kubernetes.pod.namespace"] == "coder" && output.manifest.spec.egress[2].toPorts[0].ports[0].port == "8080" && output.manifest.spec.egress[3].toCIDR[0] == "192.0.2.10/32"
    error_message = "Dependencies must preserve explicit endpoints/CIDRs and ports."
  }
  assert {
    condition     = length(output.manifest.spec.egress[0].toPorts[0].rules.dns) == 3 && length(output.manifest.spec.egress[1].toFQDNs) == 2
    error_message = "Additional DNS names must not authorize HTTPS."
  }
}

run "p2p_enabled" {
  command = plan
  variables { direct_p2p_enabled = true }
  assert {
    condition     = length(output.manifest.spec.ingress) == 1 && length(output.manifest.spec.egress) == 3 && output.manifest.spec.ingress[0].toPorts[0].ports[0].protocol == "UDP" && output.manifest.spec.ingress[0].toPorts[0].ports[0].port == "1024" && output.manifest.spec.ingress[0].toPorts[0].ports[0].endPort == 65535
    error_message = "P2P must add only the documented UDP range to ingress and egress."
  }
}

run "reject_wildcard" {
  command = plan
  variables { allowed_https_hosts = ["*.google.com"] }
  expect_failures = [var.allowed_https_hosts]
}

run "reject_ip" {
  command = plan
  variables { allowed_https_hosts = ["1.1.1.1"] }
  expect_failures = [var.allowed_https_hosts]
}

run "reject_empty_template_id" {
  command = plan
  variables { template_ids = [""] }
  expect_failures = [var.template_ids]
}

run "reject_unscoped_dependency" {
  command = plan
  variables {
    control_plane_allowances = [{ endpoint_selectors = [{}], ports = [{ port = 443 }] }]
  }
  expect_failures = [var.control_plane_allowances]
}

run "reject_external_dns" {
  command = plan
  variables {
    relay_allowances = [{ cidrs = ["1.1.1.1/32"], ports = [{ port = 53, protocol = "UDP" }] }]
  }
  expect_failures = [var.relay_allowances]
}
