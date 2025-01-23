# Zed

[Zed](https://zed.dev/) is an [open-source](https://github.com/zed-industries/zed)
multiplayer code editor from the creators of Atom and Tree-sitter.

## Use Zed to connect to Coder via SSH

Use the Coder CLI to log in and configure SSH, then connect to your workspace with Zed:

1. [Install Zed](https://zed.dev/docs/)
1. Install Coder CLI:

   ```shell
   curl -L https://coder.com/install.sh | sh
   ```

1. Log in and configure Coder SSH:

   ```shell
   coder login coder.example.com
   coder config-ssh
   ```

1. Connect via SSH with the Host set to `coder.workspace-name`:

   <!-- screenshot placeholder
   ![Fleet Connect to Coder](../../images/fleet/ssh-connect-to-coder.png)
   -->

<blockquote class="admonition note">

If you have any suggestions or experience any issues, please
[create a GitHub issue](https://github.com/coder/coder/issues) or share in
[our Discord channel](https://discord.gg/coder).

</blockquote>
