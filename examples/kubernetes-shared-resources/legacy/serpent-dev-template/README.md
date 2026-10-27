---
display_name: "serpent dev"
description: "Develop Serpent with persistent source, Go tooling, and controlled network access"
icon: /emojis/1f40d.png
tags: [go, kubernetes]
---

# serpent dev

## Getting started

Create a workspace and follow the startup log. The first start clones
[Serpent](https://github.com/coder/serpent), installs the linter, and warms its
cache. Subsequent starts reuse your checkout and installed tools without
resetting local changes. Open the terminal in `/workspace/serpent` and run:

```sh
make lint
```

The checkout starts on a writable `demo` branch at revision
`40b60bf4c1495caddc0292f862de275da292e6aa`. Its nine existing lint findings are
reported unchanged: five unchecked writes, three loop-variable captures, and one
deprecated import. `make lint` currently exits 2 because of these findings.
Startup displays source findings but fails for tool/setup errors. No source,
Makefile, or lint checks are modified or suppressed.

## Tools and storage

The public Go 1.21.13 Bookworm image supplies Go, Bash, make, Git, curl, CA
certificates, GCC, and libc headers. Startup installs golangci-lint v1.53.3.
These versions match the pinned repository's Go 1.21 requirements and Makefile;
they are older compatibility tools, not a current production toolchain.

Each workspace has a 10 GiB persistent volume at `/workspace`:

- `/workspace/serpent`: repository and your changes.
- `/workspace/cache/go`: installed Go tools and module cache, linked from `/root/go`
  to support the Makefile's `~/go/bin/golangci-lint` path.
- `/workspace/cache/go-build` and `/workspace/cache/golangci-lint`: compiler and
  analyzer caches.

These files survive stops. Deleting the workspace deletes its volume claim;
back up important work first. Files outside `/workspace` reset on stop. The
container uses its existing root account with dropped capabilities, no privilege
escalation, and no service-account token; it is not a hardened production image.
The workspace is limited to 2 CPU and 1536 MiB RAM, with two Go compilation workers.

## Networking and shared infrastructure

The platform must provide the `workspaces` namespace, a default dynamically
provisioned storage class, and a provisioner authorized to manage workspace pods,
volume claims, and network policies. This demo uses the development server's
built-in provisioner with an existing local kubeconfig; no separate provisioner
namespace or service account is required. Shared infrastructure is not owned by
an individual workspace's state. The Kubernetes context defaults to `default`.

Workspace egress needs cluster DNS, the deployment's exact Coder API/access
endpoint, and HTTPS to `github.com`, `proxy.golang.org`, and `sum.golang.org`.
Use matching FQDN and TLS SNI rules. Fresh downloads of the pinned dependencies
did not redirect; add `storage.googleapis.com` only if a required download is
observed redirecting there. Public image pulls are performed by the node.
`GOTOOLCHAIN=local` and the Go proxy without direct fallback prevent implicit
compiler downloads and arbitrary source-host fallback.
