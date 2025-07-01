# JetBrains Fleet

JetBrains Fleet is a code editor and lightweight IDE designed to support various
programming languages and development environments.

[See JetBrains's website](https://www.jetbrains.com/fleet/) to learn more about Fleet.

To connect Fleet to a Coder workspace:

1. [Install Fleet](https://www.jetbrains.com/fleet/download)

1. Install Coder CLI

   ```shell
   curl -L https://coder.com/install.sh | sh
   ```

1. Login and configure Coder SSH.

   ```shell
   coder login coder.example.com
   coder config-ssh
   ```

1. Connect via SSH with the Host set to `coder.workspace-name`
   ![Fleet Connect to Coder](../../../images/fleet/ssh-connect-to-coder.png)
