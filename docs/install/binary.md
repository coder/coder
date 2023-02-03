Coder publishes self-contained .zip and .tar.gz archives in [GitHub releases](https://github.com/coder//latest). The archives bundle `coder` binary.

1. Download the [release archive](https://github.com/coder/coder/releases/latest) appropriate for your operating system

1. Unzip the folder you just downloaded, and move the `coder` executable to a location that's on your `PATH`

   ```console
   # ex. macOS and Linux
   mv coder /usr/local/bin
   ```

   > Windows users: see [this guide](https://answers.microsoft.com/en-us/windows/forum/all/adding-path-variable/97300613-20cb-4d85-8d0e-cc9d3549ba23) for adding folders to `PATH`.

1. Start a Coder server

   ```console
   # Automatically sets up an external access URL on *.try.coder.app
   coder server

   # Requires a PostgreSQL instance (version 13 or higher) and external access URL
   coder server --postgres-url <url> --access-url <url>
   ```

   > Set `CODER_ACCESS_URL` to the external URL that users and workspaces will use to
   > connect to Coder. This is not required if you are using the tunnel. Learn more
   > about Coder's [configuration options](../admin/configure.md).

1. Visit the Coder URL in the logs to set up your first account, or use the CLI.

## Next steps

- [Quickstart](../quickstart.md)
- [Configuring Coder](../admin/configure.md)
- [Templates](../templates.md)
