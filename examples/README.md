# Project examples

| Project name                 | OS, Type                      | Features                                                | Status  |
| ---------------------------- | ----------------------------- | ------------------------------------------------------- | ------- |
| [gcp-windows](./gcp-windows) | VM, Windows Server 2022       | Regions, instance type                                  | Basic   |
| [gcp-linux](./gcp-linux)     | VM, Ubuntu 20.04              | Regions, instance type                                  | Basic   |
| [aws-linux](./aws-linux)     | VM, Ubuntu 20.04              | Regions, instance type                                  | Basic   |
| [aws-windows](./aws-windows) | VM, Windows Server 2019       | Regions, instance type                                  | Basic   |
| [aws-macos](./aws-macos)     | Mac Mini, OSX 12 Monterey     | Regions, instance type                                  | WIP     |
| kubernetes                   | Container/pod spec, any linux | Custom image, registry, provisioning ratio, PVC support | Planned |

## How to use

These are available during `coder projects init`. Optionally modify the terraform and use `coder projects create` or `coder projects update`, if you have already imported the project.

## Statuses

- Planned
- WIP
- Basic (proof of concept)
- Beta
- Stable
- Broken/unsupported

## Requests

Submit [an issue](https://github.com/coder/coder/issues/new) or pull request to request features or more examples.
