variable "policy_id" {
  description = "Coder-injected immutable policy ID; used in the namespaced resource name."
  type        = string
  nullable    = false
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]{0,45}[a-z0-9])?$", var.policy_id))
    error_message = "policy_id must be a lowercase DNS label of at most 47 characters."
  }
}

variable "namespace" {
  description = "Coder-injected target Kubernetes namespace; it must already exist."
  type        = string
  nullable    = false
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$", var.namespace))
    error_message = "namespace must be a lowercase DNS label of at most 63 characters."
  }
}

variable "template_ids" {
  description = "Coder-injected template IDs matching pod label coder.template_id. Empty means no managed policy."
  type        = set(string)
  nullable    = false
  validation {
    condition = alltrue([
      for id in var.template_ids : can(regex("^[a-zA-Z0-9]([a-zA-Z0-9_.-]{0,61}[a-zA-Z0-9])?$", id))
    ])
    error_message = "template_ids must be nonempty Kubernetes label values of at most 63 characters."
  }
}

variable "allowed_https_hosts" {
  description = "Exact lowercase DNS names permitted for DNS queries and TCP 443 with matching TLS SNI. No URLs, wildcards, IP literals, or trailing dots."
  type        = set(string)
  default     = []
  nullable    = false
  validation {
    condition = alltrue([for host in var.allowed_https_hosts :
      length(host) <= 253 && can(regex("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$", host)) && !can(cidrhost("${host}/32", 0))
    ])
    error_message = "allowed_https_hosts must contain only exact lowercase fully qualified DNS names, not IP addresses."
  }
}

variable "dns_query_names" {
  description = "Additional exact DNS query names for explicit control-plane/relay dependencies. DNS permission alone does not grant connectivity."
  type        = set(string)
  default     = []
  nullable    = false
  validation {
    condition = alltrue([for host in var.dns_query_names :
      length(host) <= 253 && can(regex("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$", host)) && !can(cidrhost("${host}/32", 0))
    ])
    error_message = "dns_query_names must contain only exact lowercase DNS names."
  }
}

variable "control_plane_allowances" {
  description = "Explicit egress destinations. Each entry selects either namespaced endpoint label maps or external CIDRs, with nonempty TCP/UDP ports."
  type = list(object({
    endpoint_selectors = optional(list(map(string)), [])
    cidrs              = optional(set(string), [])
    ports = list(object({
      port     = number
      protocol = optional(string, "TCP")
    }))
  }))
  default  = []
  nullable = false
  validation {
    condition = alltrue([for rule in var.control_plane_allowances :
      (length(rule.endpoint_selectors) > 0) != (length(rule.cidrs) > 0) &&
      alltrue([for selector in rule.endpoint_selectors : length(selector) >= 2 && try(length(selector["k8s:io.kubernetes.pod.namespace"]) > 0, false)]) &&
      alltrue([for cidr in rule.cidrs : can(cidrhost(cidr, 0))]) &&
      length(rule.ports) > 0 && alltrue([for port in rule.ports : port.port >= 1 && port.port <= 65535 && port.port != 53 && floor(port.port) == port.port && contains(["TCP", "UDP"], port.protocol)])
    ])
    error_message = "Each control-plane allowance needs either endpoint selectors (explicit namespace plus workload label) or valid CIDRs, and explicit TCP/UDP ports 1..65535 excluding reserved DNS port 53."
  }
}

variable "relay_allowances" {
  description = "Explicit relay egress destinations, with the same shape and constraints as control_plane_allowances."
  type = list(object({
    endpoint_selectors = optional(list(map(string)), [])
    cidrs              = optional(set(string), [])
    ports = list(object({
      port     = number
      protocol = optional(string, "TCP")
    }))
  }))
  default  = []
  nullable = false
  validation {
    condition = alltrue([for rule in var.relay_allowances :
      (length(rule.endpoint_selectors) > 0) != (length(rule.cidrs) > 0) &&
      alltrue([for selector in rule.endpoint_selectors : length(selector) >= 2 && try(length(selector["k8s:io.kubernetes.pod.namespace"]) > 0, false)]) &&
      alltrue([for cidr in rule.cidrs : can(cidrhost(cidr, 0))]) &&
      length(rule.ports) > 0 && alltrue([for port in rule.ports : port.port >= 1 && port.port <= 65535 && port.port != 53 && floor(port.port) == port.port && contains(["TCP", "UDP"], port.protocol)])
    ])
    error_message = "Each relay allowance needs either endpoint selectors (explicit namespace plus workload label) or valid CIDRs, and explicit TCP/UDP ports 1..65535 excluding reserved DNS port 53."
  }
}

variable "direct_p2p_enabled" {
  description = "Allow UDP 1024..65535 in both directions to direct_p2p_cidrs, including STUN 3478. This deliberately broad exception is not application-aware."
  type        = bool
  default     = true
  nullable    = false
}

variable "direct_p2p_cidrs" {
  description = "External peer CIDRs for direct UDP connectivity. CIDR rules do not select in-cluster endpoints by default."
  type        = set(string)
  default     = ["0.0.0.0/0", "::/0"]
  nullable    = false
  validation {
    condition     = length(var.direct_p2p_cidrs) > 0 && alltrue([for cidr in var.direct_p2p_cidrs : can(cidrhost(cidr, 0))])
    error_message = "direct_p2p_cidrs must contain at least one valid IPv4 or IPv6 CIDR."
  }
}
