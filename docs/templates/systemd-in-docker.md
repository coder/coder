I am not really sure what to put here yet.

```hcl
resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = var.docker_image
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace.me.owner}-${lower(data.coder_workspace.me.name)}"
  runtime = "sysbox-runc"
  env     = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  # Run as root in order to start systemd
  user    = "0:0"
  command = ["sh", "-c", <<EOF

    # Start the Coder agent as the "coder" user
    # once systemd has started up
    sudo -u coder --preserve-env=CODER_AGENT_TOKEN /bin/bash -- <<-'    EOT' &
    while [ $(systemctl is-system-running) != running ] && [ $(systemctl is-system-running) != degraded ]
    do
      echo "Waiting for system to start... $(systemctl is-system-running)"
      sleep 1
    done
    ${coder_agent.main.init_script}
    EOT

    exec /sbin/init
    EOF
    ,
  ]
}

resource "coder_agent" "main" {
  arch = data.coder_provisioner.me.arch
  os   = "linux"
}
```
