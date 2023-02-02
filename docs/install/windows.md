# Windows

Use the Windows installer to download the CLI and add Coder to `PATH`. Alternatively, you can install Coder on Windows via a [standalone binary](./binary.md).

1. Download the Windows installer from [GitHub releases](https://github.com/coder/coder/releases/latest) or from `winget`

   ```powershell
    winget install Coder.Coder
   ```

2. Run the application

   ![Windows installer](../images/install/windows-installer.png)

3. Start a Coder server

   ```console
   # Automatically sets up an external access URL on *.try.coder.app
   coder server

   # Requires a PostgreSQL instance (version 13 or higher) and external access URL
   coder server --postgres-url <url> --access-url <url>
   ```

   > Set `CODER_ACCESS_URL` to the external URL that users and workspaces will use to
   > connect to Coder. This is not required if you are using the tunnel. Learn more
   > about Coder's [configuration options](../admin/configure.md).

4. Visit the Coder URL in the logs to set up your first account, or use the CLI.

## Next steps

- [Quickstart](../quickstart.md)
- [Configuring Coder](../admin/configure.md)
- [Templates](../templates.md)
