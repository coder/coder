# dogfood template

Ammar is this template's admin.

## Personalization

The startup script runs your `~/personalize` file if it exists.

## Hosting

Coder dogfoods on a beefy, single Teraswitch machine.

- We decided to use a bare metal provider for best-in-class cost-to-performance.
- We decided to use a single machine (vertical scaling) for fast parallelized builds and tests.

## Provisioner Configuration

Our dogfood coderd box runs an SSH tunnel to our dogfood Docker host's docker socket.

The socket is mounted onto `/var/run/dogfood-docker.sock` in the coderd box.

The SSH tunnel command can be found hanging out in the screen session named `forward`.

The tunnel and corresponding SSH key is owned by root.
