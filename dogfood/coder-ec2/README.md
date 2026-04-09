# coder-ec2: EC2 Dogfood Template

EC2-based dogfood template for exercising the "cloud VM + devcontainer"
deployment pattern. Companion to
`coder-k8s` (EKS/Kubernetes) — together they cover the two most common
production deployment models.

## Architecture

```
EC2 instance (m5d.xlarge)
├── /            (root EBS, 50 GB, ephemeral — OS, Docker, agent)
├── /home/coder  (data EBS, 128 GB, persistent — code, user data)
├── swap         (NVMe instance store, ~150 GB)
├── coder agent  (host — monitoring, escape hatch)
└── devcontainer (codercom/oss-dogfood:latest)
    ├── docker-in-docker (nested daemon via DinD feature)
    └── coder sub-agent  (IDE connections route here)
```

### Storage design


Docker and the Coder host agent live on the **root volume**, not
the data volume. This is a deliberate design decision:

- The devcontainer is the primary workspace entrypoint. If the
  data volume (`/home/coder`) fills up, Docker and the host agent
  must keep running so the user can connect to the "parent" host
  agent and clean up.
- The host agent is the escape hatch. It runs as a systemd service
  on the root volume and remains accessible even when Docker or
  the devcontainer is down.
- Docker images and layers are ephemeral. They are lost on a
  workspace rebuild (root is `delete_on_termination = true`) but
  persist across stop/start. The `docker system prune` in the
  shutdown script keeps root volume usage bounded.

The devcontainer uses the repo's own `.devcontainer/devcontainer.json`
(cloned automatically by the `git-clone` module) — no
template-specific devcontainer config to maintain.


## Required IAM Permissions

The Coder provisioner (the entity running `terraform apply`) needs
an IAM policy with these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EC2Instances",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:StartInstances",
        "ec2:StopInstances",
        "ec2:DescribeInstances",
        "ec2:DescribeInstanceStatus",
        "ec2:DescribeInstanceTypes",
        "ec2:ModifyInstanceAttribute"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": ["eu-west-1", "us-east-2"]
        }
      }
    },
    {
      "Sid": "EBSVolumes",
      "Effect": "Allow",
      "Action": [
        "ec2:CreateVolume",
        "ec2:DeleteVolume",
        "ec2:AttachVolume",
        "ec2:DetachVolume",
        "ec2:ModifyVolume",
        "ec2:DescribeVolumes",
        "ec2:DescribeVolumeStatus"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": ["eu-west-1", "us-east-2"]
        }
      }
    },
    {
      "Sid": "Tagging",
      "Effect": "Allow",
      "Action": [
        "ec2:CreateTags",
        "ec2:DeleteTags",
        "ec2:DescribeTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ReadOnly",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeImages",
        "ec2:DescribeAvailabilityZones",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeVpcs",
        "ec2:DescribeIamInstanceProfileAssociations",
        "iam:GetInstanceProfile"
      ],
      "Resource": "*"
    }
  ]
}
```

### Condition notes

- The region condition restricts instance and volume creation to the
  regions offered as workspace parameters. Update this list if you add
  regions.
- For tighter control, replace `"Resource": "*"` with ARN patterns
  scoped to the VPC, subnet, and security group used by the template.
- `iam:GetInstanceProfile` is only needed if the
  `iam_instance_profile` variable is set.

## Template Variables

These are set at the template level (by an admin), not per workspace:

| Variable               | Required | Description                          |
|------------------------|----------|--------------------------------------|
| `vpc_subnet_id`        | Yes      | Subnet to launch instances in        |
| `security_group_id`    | Yes      | SG for instances (outbound-only OK)  |
| `iam_instance_profile` | No       | Optional instance profile name       |

## Security Group

The Coder agent dials out to the Coder server — no inbound rules
are required for normal operation. A minimal security group:

```hcl
resource "aws_security_group" "coder_workspace" {
  name_prefix = "coder-workspace-"
  vpc_id      = var.vpc_id

  # All outbound traffic (agent -> Coder server, package repos, etc.)
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Optional: SSH for debugging.
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/8"]  # Internal only
  }
}
```

## Quota

Storage quota uses the same unit as the `coder-k8s` template:
**1 daily_cost = 1 GB provisioned EBS**. Default workspace =
128 units (data volume). The root volume is not metered
(ephemeral, destroyed on rebuild).
