# dogfood template

## How is this hosted?

Coder dogfoods on a beefy, single Teraswitch machine. We decided to use
a bare metal provider for best-in-class cost-to-performance. We decided to
use a single machine for crazy fast parallelized builds.

# How is the provisioner configured?

Our dogfood VM runs an SSH tunnel to our dogfood Docker host's docker socket.
The socket is mounted on `/var/run/dogfood-docker.sock`.

The SSH command can be found hanging out in the screen session named
`docker-dogfood-tunnel`.

The tunnel and corresponding SSH key is under the root user.
