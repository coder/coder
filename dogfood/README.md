# dogfood template

Ammar is this template's admin.

This template runs the `gcr.io/coder-dogfood/master/coder-dev-ubuntu` Docker
image in a `sysbox-runc` container.

## Personalization

The startup script runs your `~/personalize` file if it exists.

## How is this hosted?

Coder dogfoods on a beefy, single Teraswitch machine. We decided to use
a bare metal provider for best-in-class cost-to-performance. We decided to
use a single machine for crazy fast parallelized builds and tests.

## How is the provisioner configured?

Our dogfood VM runs an SSH tunnel to our dogfood Docker host's docker socket.
The socket is mounted on `/var/run/dogfood-docker.sock`.

The SSH command can be found hanging out in the screen session named
`forward`.

The tunnel and corresponding SSH key is under the root user.
