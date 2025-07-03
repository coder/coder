terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
    docker = {
      source = "kreuzwerker/docker"
    }
  }
}

locals {
  username = data.coder_workspace_owner.me.name

  # Use a workspace image that supports rootless Docker
  # (Docker-in-Docker) and Node.js.
  workspace_image = "codercom/enterprise-node:ubuntu"
}

variable "docker_socket" {
  default     = ""
  description = "(Optional) Docker socket URI"
  type        = string
}

data "coder_parameter" "repo_url" {
  type         = "string"
  name         = "repo_url"
  display_name = "Git Repository"
  description  = "Enter the URL of the Git repository to clone into your workspace. This repository should contain a devcontainer.json file to configure your development environment."
  default      = "https://github.com/coder/coder"
  mutable      = true
}

provider "docker" {
  # Defaulting to null if the variable is an empty string lets us have an optional variable without having to set our own default
  host = var.docker_socket != "" ? var.docker_socket : null
}

data "coder_provisioner" "me" {}
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

resource "coder_agent" "main" {
  arch            = data.coder_provisioner.me.arch
  os              = "linux"
  startup_script  = <<-EOT
    set -e

    # Prepare user home with default files on first start.
    if [ ! -f ~/.init_done ]; then
      cp -rT /etc/skel ~
      touch ~/.init_done
    fi

    if [ "$${CODER_AGENT_URL#*host.docker.internal}" != "$CODER_AGENT_URL" ]; then
      # If the access URL is host.docker.internal, we set up forwarding
      # to the host Docker gateway IP address, which is typically
      # 172.17.0.1, this will allow the devcontainers to access the
      # Coder server even if the access URL has been shadowed by a
      # "docker0" interface. This usually happens if docker is started
      # inside a devcontainer.
      echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward
      sudo iptables -t nat -A POSTROUTING -j MASQUERADE

      # Get the IP address of the host Docker gateway, which is
      # typically 172.17.0.1 and set up port forwarding between this
      # workspace's Docker gateway and the host Docker gateway.
      host_ip=$(getent hosts host.docker.internal | awk '{print $1}')
      port="$${CODER_AGENT_URL##*:}"
      port="$${port%%/*}"
      case "$port" in
        [0-9]*)
          sudo iptables -t nat -A PREROUTING -p tcp --dport $port -j DNAT --to-destination $host_ip:$port
          echo "Forwarded port $port to $host_ip"
          ;;
        *)
          sudo iptables -t nat -A PREROUTING -p tcp --dport 80 -j DNAT --to-destination $host_ip:80
          sudo iptables -t nat -A PREROUTING -p tcp --dport 443 -j DNAT --to-destination $host_ip:443
          echo "Forwarded default ports 80/443 to $host_ip"
          ;;
      esac

      # Start the docker service if it is not running, this will create
      # the "docker0" interface if it does not exist.
      sudo service docker start

      # Since we cannot define "--add-host" for devcontainers, we define
      # a dnsmasq configuration that allows devcontainers to resolve the
      # host.docker.internal URL to this workspace, which is typically
      # 172.18.0.1. Note that we take the second IP address from
      # "hostname -I" because the first one is usually in the range
      # 172.17.0.0/16, which is the host Docker bridge.
      dns_ip=
      while [ -z "$dns_ip" ]; do
        dns_ip=$(hostname -I | awk '{print $2}')
        if [ -z "$dns_ip" ]; then
          echo "Waiting for hostname -I to return a valid second IP address..."
          sleep 1
        fi
      done

      # Create a simple dnsmasq configuration to allow devcontainers to
      # resolve host.docker.internal.
      sudo apt-get update -y
      sudo apt-get install -y dnsmasq

      echo "resolv-file=/etc/resolv.conf" | sudo tee /etc/dnsmasq.conf
      echo "address=/host.docker.internal/$dns_ip" | sudo tee -a /etc/dnsmasq.conf
      echo "no-dhcp-interface=" | sudo tee -a /etc/dnsmasq.conf
      echo "bind-interfaces" | sudo tee -a /etc/dnsmasq.conf
      echo "listen-address=127.0.0.1,$dns_ip" | sudo tee -a /etc/dnsmasq.conf

      # Restart dnsmasq to apply the new configuration.
      sudo service dnsmasq restart

      # Configure Docker to use the dnsmasq server for DNS resolution.
      # This allows devcontainers to resolve host.docker.internal to the
      # IP address of this workspace.
      echo "{\"dns\": [\"$dns_ip\"]}"| sudo tee /etc/docker/daemon.json

      # Restart the Docker service to apply the new configuration.
      sudo service docker restart
    else
      # Start the docker service if it is not running.
      sudo service docker start
    fi

    # Add any commands that should be executed at workspace startup
    # (e.g. install requirements, start a program, etc) here.
  EOT
  shutdown_script = <<-EOT
    set -e

    # Clean up the docker volume from unused resources to keep storage
    # usage low.
    #
    # WARNING! This will remove:
    #   - all stopped containers
    #   - all networks not used by at least one container
    #   - all images without at least one container associated to them
    #   - all build cache
    docker system prune -a -f

    # Stop the Docker service.
    sudo service docker stop
  EOT

  # These environment variables allow you to make Git commits right away after creating a
  # workspace. Note that they take precedence over configuration defined in ~/.gitconfig!
  # You can remove this block if you'd prefer to configure Git manually or using
  # dotfiles. (see docs/dotfiles.md)
  env = {
    GIT_AUTHOR_NAME     = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_AUTHOR_EMAIL    = "${data.coder_workspace_owner.me.email}"
    GIT_COMMITTER_NAME  = coalesce(data.coder_workspace_owner.me.full_name, data.coder_workspace_owner.me.name)
    GIT_COMMITTER_EMAIL = "${data.coder_workspace_owner.me.email}"
  }

  # The following metadata blocks are optional. They are used to display
  # information about your workspace in the dashboard. You can remove them
  # if you don't want to display any information.
  # For basic resources, you can use the `coder stat` command.
  # If you need more control, you can write your own script.
  metadata {
    display_name = "CPU Usage"
    key          = "0_cpu_usage"
    script       = "coder stat cpu"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "RAM Usage"
    key          = "1_ram_usage"
    script       = "coder stat mem"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Home Disk"
    key          = "3_home_disk"
    script       = "coder stat disk --path $${HOME}"
    interval     = 60
    timeout      = 1
  }

  metadata {
    display_name = "CPU Usage (Host)"
    key          = "4_cpu_usage_host"
    script       = "coder stat cpu --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Memory Usage (Host)"
    key          = "5_mem_usage_host"
    script       = "coder stat mem --host"
    interval     = 10
    timeout      = 1
  }

  metadata {
    display_name = "Load Average (Host)"
    key          = "6_load_host"
    # get load avg scaled by number of cores
    script   = <<EOT
      echo "`cat /proc/loadavg | awk '{ print $1 }'` `nproc`" | awk '{ printf "%0.2f", $1/$2 }'
    EOT
    interval = 60
    timeout  = 1
  }

  metadata {
    display_name = "Swap Usage (Host)"
    key          = "7_swap_host"
    script       = <<EOT
      free -b | awk '/^Swap/ { printf("%.1f/%.1f", $3/1024.0/1024.0/1024.0, $2/1024.0/1024.0/1024.0) }'
    EOT
    interval     = 10
    timeout      = 1
  }
}

# See https://registry.coder.com/modules/coder/devcontainers-cli
module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/devcontainers-cli/coder"
  agent_id = coder_agent.main.id

  # This ensures that the latest non-breaking version of the module gets
  # downloaded, you can also pin the module version to prevent breaking
  # changes in production.
  version = "~> 1.0"
}

# See https://registry.coder.com/modules/coder/git-clone
module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/git-clone/coder"
  agent_id = coder_agent.main.id
  url      = data.coder_parameter.repo_url.value
  base_dir = "~"
  # This ensures that the latest non-breaking version of the module gets
  # downloaded, you can also pin the module version to prevent breaking
  # changes in production.
  version = "~> 1.0"
}

# Automatically start the devcontainer for the workspace.
resource "coder_devcontainer" "repo" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.main.id
  workspace_folder = "~/${module.git-clone[0].folder_name}"
}

resource "docker_volume" "home_volume" {
  name = "coder-${data.coder_workspace.me.id}-home"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  # This field becomes outdated if the workspace is renamed but can
  # be useful for debugging or cleaning out dangling volumes.
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_volume" "docker_volume" {
  name = "coder-${data.coder_workspace.me.id}-docker"
  # Protect the volume from being deleted due to changes in attributes.
  lifecycle {
    ignore_changes = all
  }
  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  # This field becomes outdated if the workspace is renamed but can
  # be useful for debugging or cleaning out dangling volumes.
  labels {
    label = "coder.workspace_name_at_creation"
    value = data.coder_workspace.me.name
  }
}

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = local.workspace_image

  # NOTE: The `privileged` mode is one way to run Docker-in-Docker,
  # which is required for the devcontainer to work. IF this is not
  # desired, you can remove this line. However, you will need to ensure
  # that the devcontainer can run Docker commands in some other way.
  # Mounting the host Docker socket is one way to do this, but it is
  # strongly discouraged because workspaces will then compete for
  # control of the devcontainers.
  privileged = true

  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1
  command = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env = [
    "CODER_AGENT_TOKEN=${coder_agent.main.token}"
  ]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }

  # Workspace home volume persists user data across workspace restarts.
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }

  # Workspace docker volume persists Docker data across workspace
  # restarts, allowing the devcontainer cache to be reused.
  volumes {
    container_path = "/var/lib/docker"
    volume_name    = docker_volume.docker_volume.name
    read_only      = false
  }

  # Add labels in Docker to keep track of orphan resources.
  labels {
    label = "coder.owner"
    value = data.coder_workspace_owner.me.name
  }
  labels {
    label = "coder.owner_id"
    value = data.coder_workspace_owner.me.id
  }
  labels {
    label = "coder.workspace_id"
    value = data.coder_workspace.me.id
  }
  labels {
    label = "coder.workspace_name"
    value = data.coder_workspace.me.name
  }
}
