# Devcontainer releases and known issues

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

- Image caching: error pushing image

  - `BLOB_UNKNOWN: Manifest references unknown blob(s)`
  - [Issue 385](https://github.com/coder/envbuilder/issues/385)

- Support for VS Code Extensions requires a workaround.

  - [Issue 68](https://github.com/coder/envbuilder/issues/68#issuecomment-1805974271)

- Envbuilder does not support Volume Mounts

- Support for lifecycle hooks is limited.
  ([Issue](https://github.com/coder/envbuilder/issues/395))
  - Supported:
    - `onCreateCommand`
    - `updateContentCommand`
    - `postCreateCommand`
    - `postStartCommand`
  - Not supported:
    - `initializeCommand`
    - `postAttachCommand`
    - `waitFor`

Visit the
[Envbuilder repository](https://github.com/coder/envbuilder/blob/main/docs/devcontainer-spec-support.md)
for a full list of supported features and known issues.
