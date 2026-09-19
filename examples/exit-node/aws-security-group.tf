# Attach this security group to workspace network interfaces.
# Replace every example CIDR and port with deployment values.
resource "aws_security_group" "workspace_exit_node_boundary" {
  name        = "workspace-exit-node-boundary"
  description = "Allow workspace egress only to DNS, Coder control plane, and exit nodes"
  vpc_id      = var.vpc_id

  egress {
    description = "UDP DNS to approved resolvers"
    protocol    = "udp"
    from_port   = 53
    to_port     = 53
    cidr_blocks = ["10.0.0.2/32"]
  }

  egress {
    description = "TCP DNS to approved resolvers"
    protocol    = "tcp"
    from_port   = 53
    to_port     = 53
    cidr_blocks = ["10.0.0.2/32"]
  }

  # Split this rule if Coder and DERP use different ports or CIDRs.
  egress {
    description = "Coder control plane and DERP HTTPS"
    protocol    = "tcp"
    from_port   = 443
    to_port     = 443
    cidr_blocks = ["192.0.2.10/32"]
  }

  # Remove this rule if the DERP configuration does not use STUN.
  egress {
    description = "DERP STUN"
    protocol    = "udp"
    from_port   = 3478
    to_port     = 3478
    cidr_blocks = ["198.51.100.20/32"]
  }

  egress {
    description = "Exit node WireGuard endpoints"
    protocol    = "udp"
    from_port   = 51820
    to_port     = 51820
    cidr_blocks = ["203.0.113.10/32", "203.0.113.11/32"]
  }
}
