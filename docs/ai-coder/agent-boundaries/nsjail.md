# nsjail Jail Type

nsjail is Agent Boundaries' default jail type that uses Linux namespaces to
provide process isolation. It creates unprivileged network namespaces to control
and monitor network access for processes running under Boundary.

## Overview

nsjail leverages Linux namespace technology to isolate processes at the network
level. When Agent Boundaries runs with nsjail, it creates a separate network
namespace for the isolated process, allowing Agent Boundaries to intercept and
filter all network traffic according to the configured policy.

This jail type requires Linux capabilities to create and manage network
namespaces, which means it has specific runtime requirements when running in
containerized environments like Docker.

## Architecture

<img width="1228" height="604" alt="Boundary" src="https://github.com/user-attachments/assets/1b7c8c5b-7b8f-4adf-8795-325bd28715c6" />

## Runtime & Permission Requirements for Running Agent Boundaries in Docker

This section describes the Linux capabilities and runtime configurations
required to run Agent Boundaries with nsjail inside a Docker container.
Requirements vary depending on the OCI runtime and the seccomp profile in use.

### 1. Default `runc` runtime with `CAP_NET_ADMIN`

When using Docker's default `runc` runtime, Agent Boundaries requires the
container to have `CAP_NET_ADMIN`. This is the minimal capability needed for
configuring virtual networking inside the container.

Docker's default seccomp profile may also block certain syscalls (such as
`clone`) required for creating unprivileged network namespaces. If you encounter
these restrictions, you may need to update or override the seccomp profile to
allow these syscalls.

[see Docker Seccomp Profile Considerations](#docker-seccomp-profile-considerations)

### 2. Default `runc` runtime with `CAP_SYS_ADMIN` (testing only)

For development or testing environments, you may grant the container
`CAP_SYS_ADMIN`, which implicitly bypasses many of the restrictions in Docker's
default seccomp profile.

- Agent Boundaries does not require `CAP_SYS_ADMIN` itself.
- However, Docker's default seccomp policy commonly blocks namespace-related
  syscalls unless `CAP_SYS_ADMIN` is present.
- Granting `CAP_SYS_ADMIN` enables Agent Boundaries to run without modifying the
  seccomp profile.

⚠️ Warning: `CAP_SYS_ADMIN` is extremely powerful and should not be used in
production unless absolutely necessary.

### 3. `sysbox-runc` runtime with `CAP_NET_ADMIN`

When using the `sysbox-runc` runtime (from Nestybox), Agent Boundaries can run
with only:

- `CAP_NET_ADMIN`

The sysbox-runc runtime provides more complete support for unprivileged user
namespaces and nested containerization, which typically eliminates the need for
seccomp profile modifications.

## Docker Seccomp Profile Considerations

Docker's default seccomp profile frequently blocks the `clone` syscall, which is
required by Agent Boundaries when creating unprivileged network namespaces. If
the `clone` syscall is denied, Agent Boundaries will fail to start.

To address this, you may need to modify or override the seccomp profile used by
your container to explicitly allow the required `clone` variants.

You can find the default Docker seccomp profile for your Docker version here
(specify your docker version):

https://github.com/moby/moby/blob/v25.0.13/profiles/seccomp/default.json#L628-L635

If the profile blocks the necessary `clone` syscall arguments, you can provide a
custom seccomp profile that adds an allow rule like the following:

```json
{
	"names": ["clone"],
	"action": "SCMP_ACT_ALLOW"
}
```

This example unblocks the clone syscall entirely.

### Example: Overriding the Docker Seccomp Profile

To use a custom seccomp profile, start by downloading the default profile for
your Docker version:

https://github.com/moby/moby/blob/v25.0.13/profiles/seccomp/default.json#L628-L635

Save it locally as seccomp-v25.0.13.json, then insert the clone allow rule shown
above (or add "clone" to the list of allowed syscalls).

Once updated, you can run the container with the custom seccomp profile:

```bash
docker run -it \
  --cap-add=NET_ADMIN \
  --security-opt seccomp=seccomp-v25.0.13.json \
  test bash
```

This instructs Docker to load your modified seccomp profile while granting only
the minimal required capability (`CAP_NET_ADMIN`).
