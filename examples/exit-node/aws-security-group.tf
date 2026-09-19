# Attach this security group to workspace network interfaces.
# Replace every documentation CIDR and port with values from your deployment.
resource "aws_security_group" "workspace_exit_node_boundary" {
  name        = "workspace-exit-node-boundary"
  description = "Allow workspace egress only to DNS, Coder control plane, and exit nodes"
  vpc_id      = var.vpc_id

  # Allow UDP DNS to the VPC resolver or your approved DNS resolver CIDR.
  egress {
    description = "UDP DNS to approved resolvers"
    protocol    = "udp"
    from_port   = 53
    to_port     = 53
    cidr_blocks = ["10.0.0.2/32"] # Replace with the approved resolver CIDR.
  }

  # Allow TCP DNS for large responses and the agent's exempt-host lookups.
  egress {
    description = "TCP DNS to approved resolvers"
    protocol    = "tcp"
    from_port   = 53
    to_port     = 53
    cidr_blocks = ["10.0.0.2/32"] # Replace with the approved resolver CIDR.
  }

  # Allow HTTPS to coderd and any DERP relay reached on the same approved CIDR.
  egress {
    description = "Coder control plane and DERP HTTPS"
    protocol    = "tcp"
    from_port   = 443
    to_port     = 443
    cidr_blocks = ["192.0.2.10/32"] # Replace or split for coderd and DERP CIDRs.
  }

  # Allow STUN when the deployment's DERP configuration uses it.
  egress {
    description = "DERP STUN"
    protocol    = "udp"
    from_port   = 3478
    to_port     = 3478
    cidr_blocks = ["198.51.100.20/32"] # Replace with the STUN endpoint CIDR, or remove if unused.
  }

  # Allow WireGuard only to the advertised exit node endpoints.
  egress {
    description = "Exit node WireGuard endpoints"
    protocol    = "udp"
    from_port   = 51820
    to_port     = 51820
    cidr_blocks = ["203.0.113.10/32", "203.0.113.11/32"]
  }
}
