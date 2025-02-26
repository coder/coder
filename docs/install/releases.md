# Releases

Coder releases are cut directly from main in our
[GitHub](https://github.com/coder/coder) on the first Tuesday of each month.

We recommend enterprise customers test the compatibility of new releases with
their infrastructure on a staging environment before upgrading a production
deployment.

## Release channels

We support two release channels:
[mainline](https://github.com/coder/coder/releases/tag/v2.19.0) for the bleeding
edge version of Coder and
[stable](https://github.com/coder/coder/releases/latest) for those with lower
tolerance for fault. We field our mainline releases publicly for one month
before promoting them to stable. The version prior to stable receives patches
only for security issues or CVEs.

### Mainline releases

- Intended for customers with a staging environment
- Gives earliest access to new features
- May include minor bugs
- All bugfixes and security patches are supported

### Stable releases

- Safest upgrade/installation path
- May not include the latest features
- All bugfixes and security patches are supported

### Security Support

- In-product security vulnerabilities and CVEs are supported

> For more information on feature rollout, see our
> [feature stages documentation](../contributing/feature-stages.md).

## Installing stable

When installing Coder, we generally advise specifying the desired version from
our GitHub [releases page](https://github.com/coder/coder/releases).

You can also use our `install.sh` script with the `stable` flag to install the
latest stable release:

```shell
curl -fsSL https://coder.com/install.sh | sh -s -- --stable
```

Best practices for installing Coder can be found on our [install](./index.md)
pages.

## Release schedule

| Release name | Release Date       | Status           |
|--------------|--------------------|------------------|
| 2.12.x       | June 04, 2024      | Not Supported    |
| 2.13.x       | July 02, 2024      | Not Supported    |
| 2.14.x       | August 06, 2024    | Not Supported    |
| 2.15.x       | September 03, 2024 | Not Supported    |
| 2.16.x       | October 01, 2024   | Security Support |
| 2.17.x       | November 05, 2024  | Security Support |
| 2.18.x       | December 03, 2024  | Stable           |
| 2.19.x       | February 04, 2024  | Mainline         |

> **Tip**: We publish a
> [`preview`](https://github.com/coder/coder/pkgs/container/coder-preview) image
> `ghcr.io/coder/coder-preview` on each commit to the `main` branch. This can be
> used to test under-development features and bug fixes that have not yet been
> released to [`mainline`](#mainline-releases) or [`stable`](#stable-releases).
>
> The `preview` image is not intended for production use.

### A note about January releases

v2.18 was promoted to stable on January 7th, 2025.

As of January, 2025 we skip the January release each year because most of our engineering team is out for the December holiday period.
