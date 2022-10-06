terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.28"
    }
    coder = {
      source  = "coder/coder"
      version = "0.5.0"
    }
  }
}

variable "ecs-cluster" {
  description = "Input the ECS cluster ARN to host the workspace"
  default     = ""
}
variable "cpu" {
  default = "1024"
}

variable "memory" {
  default = "2048"
}

# configure AWS provider with creds present on Coder server host
provider "aws" {
  shared_config_files      = ["$HOME/.aws/config"]
  shared_credentials_files = ["$HOME/.aws/credentials"]
}

# coder workspace, created as an ECS task definition
resource "aws_ecs_task_definition" "workspace" {
  family = "coder"

  requires_compatibilities = ["EC2"]
  cpu                      = var.cpu
  memory                   = var.memory
  container_definitions = jsonencode([
    {
      name      = "coder-workspace-${data.coder_workspace.me.id}"
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
          sourceVolume = "home-dir-${data.coder_workspace.me.id}"
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
    name = "home-dir-${data.coder_workspace.me.id}"

    docker_volume_configuration {
      # "shared" ensures that the disk is persisted upon workspace restart
      scope         = "shared"
      autoprovision = true
      driver        = "local"
    }
  }
}

resource "aws_ecs_service" "workspace" {
  name            = "workspace-${data.coder_workspace.me.id}"
  cluster         = var.ecs-cluster
  task_definition = aws_ecs_task_definition.workspace.arn
  # scale the service to zero when the workspace is stopped
  desired_count = data.coder_workspace.me.start_count
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
  agent_id  = coder_agent.coder.id
  name      = "code-server"
  icon      = "/icon/code.svg"
  url       = "http://localhost:13337?folder=/home/coder"
  subdomain = false

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 3
    threshold = 10
  }
}
