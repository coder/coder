job "workspace" {
  datacenters = ["dc1"]
  namespace = "${workspace_tag}"
  type = "service"
  group "workspace" {
    volume "home_volume" {
      type   = "csi"
      source = "${home_volume_name}"
      read_only = false
      attachment_mode = "file-system"
      access_mode     = "single-node-writer"
    }
    network {
      port "http" {}
    }
    task "workspace" {
      driver = "docker"
      config {
        image = "codercom/enterprise-base:ubuntu"
        ports = ["http"]
        labels {
          name = "${workspace_tag}"
          managed_by = "coder"
        }
        hostname = "${workspace_name}"
        entrypoint = ["sh", "-c", "sudo chown coder:coder -R /home/coder && echo '${base64encode(coder_init_script)}' | base64 --decode | sh"]
      }
      volume_mount {
        volume      = "home_volume"
        destination = "/home/coder"
      }
      resources {
        cores  = ${cores}
        memory = ${memory_mb}
      }
      env {
        CODER_AGENT_TOKEN = "${coder_agent_token}"
      }
      meta {
        tag = "${workspace_tag}"
        managed_by = "coder"
      }
    }
    meta {
      tag = "${workspace_tag}"
      managed_by = "coder"
    }
  }
  meta {
    tag = "${workspace_tag}"
    managed_by = "coder"
  }
}
