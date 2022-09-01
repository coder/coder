Coder publishes the following system packages [in GitHub releases](https://github.com/coder/coder/releases):

- .deb (Debian, Ubuntu)
- .rpm (Fedora, CentOS, RHEL, SUSE)
- .apk (Alpine)

Once installed, you can run Coder as a system service.

```sh
# Set up an access URL or enable CODER_TUNNEL
sudo vim /etc/coder.d/coder.env

# To systemd to start Coder now and on reboot
sudo systemctl enable --now coder

# View the logs to see Coder's URL and ensure a successful start
journalctl -u coder.service -b
```

Visit the Coder URL in the logs to set up your first account, or use the CLI:

```sh
coder login <access-url>
```

## Next steps

- [Quickstart](../quickstart.md)
- [Configuring Coder](../admin/configure.md)
- [Templates](../templates.md)
