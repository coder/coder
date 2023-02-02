1. Download and install one of the following system packages from [GitHub releases](https://github.com/coder/coder/releases/latest):

   - .deb (Debian, Ubuntu)
   - .rpm (Fedora, CentOS, RHEL, SUSE)
   - .apk (Alpine)

1. Run Coder as a system service.

   ```console
   # Optional) Set up an access URL
   sudo vim /etc/coder.d/coder.env

   # To systemd to start Coder now and on reboot
   sudo systemctl enable --now coder

   # View the logs to see Coder's URL and ensure a successful start
   journalctl -u coder.service -b
   ```

   > Set `CODER_ACCESS_URL` to the external URL that users and workspaces will use to
   > connect to Coder. This is not required if you are using the tunnel. Learn more
   > about Coder's [configuration options](../admin/configure.md).

1. Visit the Coder URL in the logs to set up your first account, or use the CLI:

   ```console
   coder login <access-url>
   ```

## Restarting Coder

After updating Coder or applying configuration changes, restart the server:

```console
sudo systemctl restart coder
```

## Next steps

- [Quickstart](../quickstart.md)
- [Configuring Coder](../admin/configure.md)
- [Templates](../templates.md)
