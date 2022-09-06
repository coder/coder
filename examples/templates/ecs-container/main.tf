terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.28"
    }
    coder = {
      source  = "coder/coder"
      version = "~> 0.4.9"
    }
    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = "2.2.0"
    }
  }
}

# required for multi-user data config
provider "cloudinit" {}

# configure AWS provider with creds present on Coder server host
provider "aws" {
  region                   = "us-east-1"
  shared_config_files      = ["/home/coder/.aws/config"]
  shared_credentials_files = ["/home/coder/.aws/credentials"]
}

locals {

  # User data is used to stop/start AWS instances. See:
  # https://github.com/hashicorp/terraform-provider-aws/issues/22

  user_data_start = <<EOT
Content-Type: multipart/mixed; boundary="//"
MIME-Version: 1.0

--//
Content-Type: text/cloud-config; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="cloud-config.txt"

#cloud-config
cloud_final_modules:
- [scripts-user, always]
hostname: ${lower(data.coder_workspace.me.name)}

--//
Content-Type: text/x-shellscript; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="userdata.txt"

#!/bin/bash
sudo -u ubuntu sh -c '${coder_agent.coder.init_script}'
--//--
EOT

  user_data_end = <<EOT
Content-Type: multipart/mixed; boundary="//"
MIME-Version: 1.0
--//
Content-Type: text/cloud-config; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="cloud-config.txt"
#cloud-config
cloud_final_modules:
- [scripts-user, always]
--//
Content-Type: text/x-shellscript; charset="us-ascii"
MIME-Version: 1.0
Content-Transfer-Encoding: 7bit
Content-Disposition: attachment; filename="userdata.txt"
#!/bin/bash
sudo shutdown -h now
--//--
EOT
}

# user data definitions, for both ECS & Coder
data "cloudinit_config" "main" {
  base64_encode = true
  gzip          = false

  part {
    content = data.coder_workspace.me.transition == "start" ? local.user_data_start : local.user_data_end
  }
  # required for the EC2 instances to join the ECS cluster
  part {
    content = "#!/bin/bash\necho ECS_CLUSTER=${aws_ecs_cluster.main.name} >> /etc/ecs/ecs.config"
  }
}

# set launch template for EC2 auto-scaling group
resource "aws_launch_template" "coder-oss-ubuntu" {
  name_prefix = "coder-oss"
  # ECS optimized AMI - https://github.com/awsdocs/amazon-ecs-developer-guide/blob/master/doc_source/ecs-optimized_AMI.md
  image_id      = "ami-03f8a7b55051ae0d4"
  instance_type = "t2.medium"
  iam_instance_profile {
    arn = "arn:aws:iam::816024705881:instance-profile/coder-oss-ecs-role"
  }
  user_data = data.cloudinit_config.main.rendered
}

# provision auto-scaling group to host ECS task definitions
resource "aws_autoscaling_group" "main" {
  name                  = "coder-ecs-auto-scaling-group"
  min_size              = 1
  max_size              = 1
  desired_capacity      = 1
  availability_zones    = ["us-east-1a"]
  protect_from_scale_in = true

  launch_template {
    id      = aws_launch_template.coder-oss-ubuntu.id
    version = "$Latest"
  }

  tag {
    key                 = "AmazonECSManaged"
    value               = true
    propagate_at_launch = true
  }
}

# create AWS ECS cluster
resource "aws_ecs_cluster" "main" {
  name = "coder-oss-ecs"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

# create capacity provider & tie in autoscaling group
resource "aws_ecs_capacity_provider" "main" {
  name = "coder-oss-capacity-provider"

  auto_scaling_group_provider {
    auto_scaling_group_arn         = aws_autoscaling_group.main.arn
    managed_termination_protection = "ENABLED"

    managed_scaling {
      maximum_scaling_step_size = 1000
      minimum_scaling_step_size = 1
      status                    = "ENABLED"
      target_capacity           = 10
    }
  }
}

# attach capacity provider to cluster
resource "aws_ecs_cluster_capacity_providers" "main" {
  cluster_name = aws_ecs_cluster.main.name

  capacity_providers = [aws_ecs_capacity_provider.main.name]
}

# coder workspace, created as an ECS task definition
resource "aws_ecs_task_definition" "workspace" {
  family = "coder"

  requires_compatibilities = ["EC2"]
  cpu                      = 1024
  memory                   = 2048
  container_definitions = jsonencode([
    {
      name      = "coder-workspace-1"
      image     = "codercom/enterprise-base:ubuntu"
      cpu       = 1024
      memory    = 2048
      essential = true
      user      = "coder"
      command   = ["sh", "-c", coder_agent.coder.init_script]
      environment = [
        {
          "name"  = "CODER_AGENT_TOKEN"
          "value" = coder_agent.coder.token
        }
      ]
      mountPoints = [
        {
          # the name of the volume to mount
          sourceVolume = "home-dir"
          # path on the container to mount the volume at
          containerPath = "/home/coder"
        }
      ]
      portMappings = [
        {
          containerPort = 80
          hostPort      = 80
        }
      ]
    }
  ])

  # workspace persistent volume definition
  volume {
    name = "home-dir"

    docker_volume_configuration {
      # "shared" ensures that the disk is persisted upon workspace restart
      scope         = "shared"
      autoprovision = true
      driver        = "local"
    }
  }
}

resource "aws_ecs_service" "workspace" {
  name            = "workspace"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.workspace.arn
  desired_count   = 1
}

data "coder_workspace" "me" {}

resource "coder_agent" "coder" {
  arch           = "amd64"
  auth           = "token"
  os             = "linux"
  dir            = "/home/coder"
  startup_script = <<EOT
    #!/bin/bash
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh  | tee code-server-install.log
    code-server --auth none --port 13337 | tee code-server-install.log &
  EOT
}

resource "coder_app" "code-server" {
  agent_id      = coder_agent.coder.id
  name          = "code-server"
  icon          = "/icon/code.svg"
  url           = "http://localhost:13337?folder=/home/coder"
  relative_path = true
}
