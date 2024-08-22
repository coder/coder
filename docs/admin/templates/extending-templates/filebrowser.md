# File Browser

To access the contents of a workspace directory in a browser, you can use File
Browser. File Browser is a lightweight file manager that allows you to view and
manipulate files in a web browser.

Show and manipulate the contents of the `/home/coder` directory in a browser.

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
  startup_script = <<EOT
#!/bin/bash

curl -fsSL https://raw.githubusercontent.com/filebrowser/get/master/get.sh | bash
filebrowser --noauth --root /home/coder --port 13339 >/tmp/filebrowser.log 2>&1 &

EOT
}

resource "coder_app" "filebrowser" {
  agent_id     = coder_agent.coder.id
  display_name = "file browser"
  slug         = "filebrowser"
  url          = "http://localhost:13339"
  icon         = "https://raw.githubusercontent.com/matifali/logos/main/database.svg"
  subdomain    = true
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13339/healthz"
    interval  = 3
    threshold = 10
  }
}
```

Or alternatively, you can use the
[`filebrowser`](https://registry.coder.com/modules/filebrowser) module from the
Coder registry:

```tf
module "filebrowser" {
  source   = "registry.coder.com/modules/filebrowser/coder"
  version  = "1.0.8"
  agent_id = coder_agent.main.id
}
```

![File Browser](../images/file-browser.png)
