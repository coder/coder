# Envbuilder releases and known issues

## Release channels

Envbuilder provides two release channels:

- **Stable**
  - Available at
    [`ghcr.io/coder/envbuilder`](https://github.com/coder/envbuilder/pkgs/container/envbuilder).
    Tags `>=1.0.0` are considered stable.
- **Preview**
  - Available at
    [`ghcr.io/coder/envbuilder-preview`](https://github.com/coder/envbuilder/pkgs/container/envbuilder-preview).
    Built from the tip of `main`, and should be considered experimental and
    prone to breaking changes.

Refer to the
[Envbuilder GitHub repository](https://github.com/coder/envbuilder/) for more
information and to submit feature requests or bug reports.

## Known issues

Key limitations of Envbuilder include:

- **Custom `ENTRYPOINT`**: Envbuilder replaces the image entrypoint with its
  own binary. Custom `ENTRYPOINT` instructions in Dockerfiles are not executed.
- **`postAttachCommand`**: Not supported. Use `postStartCommand` or the Coder
  agent startup script instead.
- **`initializeCommand`**: Not supported.
- **SSH/Git credentials in lifecycle scripts**: Lifecycle scripts run before the
  Coder agent starts, so Coder-managed SSH keys and Git credentials are not
  available. Use the agent startup script for operations that require
  authentication. See
  [SSH and Git credentials in lifecycle scripts](./add-envbuilder.md#ssh-and-git-credentials-in-lifecycle-scripts).

Visit the
[Envbuilder repository](https://github.com/coder/envbuilder/blob/main/docs/devcontainer-spec-support.md)
for a full list of supported features and known issues.
