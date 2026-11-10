---
title: Releases
---

Coder releases are cut directly from the `main` branch of the [coder/coder repo](https://github.com/coder/coder).
For a full list of releases, you can visit [coder/coder's Releases](https://github.com/coder/coder/releases).
Alternatively, if you just want the latest CLI, you can visit the [Coder Downloads page](https://coder.com/downloads).

This page explains details about our release versioning scheme, schedules, terminology, and support periods.

We recommend enterprise customers test the compatibility of new releases with their infrastructure on a staging environment before upgrading a production deployment.

## Release versioning

Coder releases follow a semantic versioning scheme.
Each release takes the format `vX.Y.Z`:

- `X` is the major version. The current major version of Coder is `2`.
- `Y` is the minor version.
  The docs will also refer to the minor version as the combined `vX.Y`.
- `Z` is the specific release, also referred to as the hotfix or patch release for a given minor version.

The Coder Team handles releasing minor versions differently from releasing patches.
This section includes details on each.

### Minor versions

Generally, the Coder Team publishes new minor versions on the first Tuesday of every month.
The release of a new minor version is always patch release `0` (for example, `vX.Y.0`).
The date of a `.0` release for a given minor version is the same as a minor version's *release date*.

#### January releases

Minor versions on the first Tuesday of January are **not** guaranteed to occur, due to changes in the Coder Team's availability during the December holiday season.
That being said, an ad-hoc release might still occur.
You should not rely on the existence a January release.
If needed, reach out to Coder Support or your account team directly to determine if one will be occurring closer to the release date.

### Patch releases

The Coder Team ships bug fixes and security patches between minor releases as patch releases, such as `v2.38.5`.
Patch releases don't follow a fixed schedule.
That being said, each minor version, on average, has a new patch release about once a week.

A patch release becomes unsupported when a newer patch release on the same minor version is published.
This means that you should apply the latest patch release to your Coder deployment to get the latest bug fixes and security patches.
The Coder Support Team may ask you during troubleshooting to apply the latest patch release as a troubleshooting step.

Every fix is merged to `main` first, then backported to the release branches that are eligible for it:

| Release channel          | Backported fixes                        |
|--------------------------|-----------------------------------------|
| Mainline                 | Bug fixes and security patches          |
| Stable                   | Bug fixes and security patches          |
| Security Support         | Security patches and CVE fixes          |
| Extended Support Release | Critical bug fixes and security patches |

The Coder Team decides which releases receive a bug fix based on its severity and how likely users on each release are to encounter the bug.
A fix might ship only in mainline and stable, in which case the supported path for older releases is to upgrade.

Fixes that require a database schema change aren't backported.
They ship in the next minor release.

To request a patch release for an issue that affects your deployment, [contact Coder support](../support/index.md) with your Coder version, the impact, and any deadline.
To test a fix before it's released, use the `preview` image described in [Release schedule](#release-schedule).

## Release channels

Coder ships in four primary release channels:

- **Mainline**
- **Stable**
- **Security Support**
- **Extended Support Release**

Coder will occasionally ship release candidates, which are ad-hoc builds meant to validate in-development features.

The Coder Team fields each mainline release publicly for one month before promoting it to stable.
The security support version receives patches only for security issues or Common Vulnerabilities and Exposures (CVEs).

### Mainline releases

The bleeding edge version of Coder.
This is also whichever version has the latest `.0` release.
Mainline releases:

- Are intended for customers with a staging environment.
- Gives earliest access to new features.
- Might include minor bugs.
- Will include all bug fixes and security patches.

### Stable releases

The minor version before the mainline version.
If the mainline version is `Y`, then the stable version is `Y - 1`.
Stable releases:

- Are the safest upgrade/installation path.
- Might not include the latest features.
- Will include all bug fixes and security patches.

### Security Support

The minor version before the stable version.
If the mainline version is `Y`, then the security support version is `Y - 2`.
Security releases:

- Might not include the latest features.
- Do not receive bug fixes except for in-product security vulnerabilities and CVEs.
- Are not recommended for new deployments.
- Will reach support end of life (EOL) after one month, unless it is an [Extended Support Release](#extended-support-release).

For more information on feature rollout, visit the [feature stages documentation](./feature-stages.md).

### Extended Support Release

Two releases each year are designated as Extended Support Releases (ESRs).
Each ESR is supported for 12 months from the date of its `.0` release.
ESRs:

- Are designed for organizations that prioritize long-term stability.
- Receive only critical bug fixes and security patches.
- Are ideal for regulated environments or large deployments with strict upgrade cycles.

ESR versions will be updated with critical bug fixes and security patches that are available to paying customers.
This extended support model provides predictable, long-term maintenance for organizations that require enhanced stability.
Because ESR forgoes new features in favor of maintenance and stability, it is best suited for teams with strict upgrade constraints.
The latest ESR version is [Coder 2.34](https://github.com/coder/coder/releases/tag/v2.34.18).

For more information, visit the [Coder ESR announcement](https://coder.com/blog/esr) or the [2.29 to 2.34 ESR Upgrade Guide](../install/operate/upgrade/esr-2.29-2.34-upgrade.md).

### Release Candidates

Release Candidates (RCs) are ad-hoc builds that Coder releases to validate in-development features with select customers.
Features introduced in a release candidate (RC) are not guaranteed to be included in a mainline or stable release.
RCs:

- Are not guaranteed to be stable or free of bugs.
- Are not intended for production use.

With RCs, the Coder Team can create builds for customers and other users to try out new, under-development functionality without cutting a new minor version.
Unlike mainline and stable releases, RCs do not follow a fixed schedule and carry no guarantees around stability or long-term support.
They exist purely as a feedback mechanism: the Coder Team can ship targeted builds, gather real-world input, and iterate before committing changes to the standard release channels.

## Install stable

Download the latest stable or mainline release from the [Coder downloads page](https://coder.com/downloads).
To install a specific version, use the [coder/coder releases page on GitHub](https://github.com/coder/coder/releases).

You can find best practices for installing Coder in the [Install](../install/index.md) section.

## Release schedule
<!-- Autogenerated release calendar from scripts/update-release-calendar.sh -->
<!-- RELEASE_CALENDAR_START -->
| Release name                                                                                    | Release Date       | Status                   | Latest Release                                                   |
|-------------------------------------------------------------------------------------------------|--------------------|--------------------------|------------------------------------------------------------------|
| [2.29](https://coder.com/changelog/coder-2-29)                                                  | December 02, 2025  | Extended Support Release | [v2.29.26](https://github.com/coder/coder/releases/tag/v2.29.26) |
| [2.34](https://coder.com/changelog/coder-2-34)                                                  | June 02, 2026      | Extended Support Release | [v2.34.18](https://github.com/coder/coder/releases/tag/v2.34.18) |
| [2.35](https://coder.com/changelog/coder-2-35-1)                                                | July 07, 2026      | Not Supported            | [v2.35.12](https://github.com/coder/coder/releases/tag/v2.35.12) |
| [2.36](https://coder.com/changelog/coder-2-36)                                                  | August 04, 2026    | Not Supported            | [v2.36.12](https://github.com/coder/coder/releases/tag/v2.36.12) |
| [2.37](https://coder.com/changelog/coder-2-37)                                                  | September 01, 2026 | Security Support         | [v2.37.10](https://github.com/coder/coder/releases/tag/v2.37.10) |
| [2.38](https://coder.com/changelog/broader-model-access-and-tighter-governance-controls-in-238) | October 06, 2026   | Stable                   | [v2.38.6](https://github.com/coder/coder/releases/tag/v2.38.6)   |
| [2.39](https://coder.com/changelog/project-level-memory-for-coder-agents-in-239)                | November 03, 2026  | Mainline                 | [v2.39.0](https://github.com/coder/coder/releases/tag/v2.39.0)   |
| 2.40                                                                                            |                    | Not Released             | N/A                                                              |
<!-- RELEASE_CALENDAR_END -->

> [!TIP]
> We publish a [`preview`](https://github.com/coder/coder/pkgs/container/coder-preview) image `ghcr.io/coder/coder-preview` on each commit to the `main` branch.
> You can use this to test under-development features and bug fixes that have not yet been released to [`mainline`](#mainline-releases) or [`stable`](#stable-releases).
>
> The `preview` image is not intended for production use.
