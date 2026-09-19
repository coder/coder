# docker-lnx-server template

Docker-container workspace template for the captain's standalone Coder
deployment on `lnx-server` (tailnet host, Coder container `coder-fork:36b74ca3`
serving `http://lnx-server:7080/`).

It is the upstream `docker` starter template plus one host-specific fix:

- `coder_access_host` variable (default `lnx-server.hippo-tilapia.ts.net`)
  mapped to the Docker host gateway via an extra `host` block on
  `docker_container.workspace`. Plain bridge containers cannot resolve
  tailnet DNS names, so without this the workspace agent cannot download
  itself from the deployment access URL and the workspace sits at
  `connecting` forever. Coder publishes 7080 on `0.0.0.0`, so
  `host-gateway:7080` reaches coderd.

## Proof workspace

`proof1` (template `docker-test`, owner `admin`) was created end to end
through the deployed Coder on 2026-09-19 and reached agent `connected`
with `coder ssh proof1` working. It is intentionally LEFT RUNNING so the
captain can see it. Reach it at `http://lnx-server:7080/` -> Workspaces ->
proof1, or delete it with:

```sh
export CODER_URL=http://localhost:7080
CODER_SESSION_TOKEN=$(cat ~/.coder-admin-token) /home/trillium/coder-cli delete proof1
```

## Push a new version (from lnx-server)

The deployment CLI is an exact-version copy of the server binary:

```sh
export CODER_URL=http://localhost:7080
export CODER_SESSION_TOKEN=$(cat ~/.coder-admin-token)
cd /home/trillium/coder-templates/docker-test
/home/trillium/coder-cli templates push \
  --directory /home/trillium/coder-templates/docker-test --yes \
  --message "reason for the change"
```

The canonical source of the template is this directory in the fork.
The host copy at `/home/trillium/coder-templates/docker-test` is the
working copy that gets pushed; keep the two in sync.

## Host changes made 2026-09-19 (all on lnx-server, tailnet only)

1. First admin user created via `POST /api/v2/users/first` (the deployment
   had no users at all). Credentials live only in the host files named
   below; rotate them, do not copy them anywhere.
2. UFW: `sudo ufw allow from 172.16.0.0/12 to any port 7080 proto tcp`.
   The host firewall (`INPUT DROP`) was silently dropping workspace
   container traffic to the published Coder port, so agents could resolve
   the access URL but never connect. Verified with a clean busybox
   container before/after. UFW rules persist across reboot.
   Public exposure unchanged: 7080 was and is reachable only via tailnet
   (plus localhost and Docker subnets now).
3. Agent binary seeded into the deployment cache:
   `/home/coder/.cache/coder/site/orig/bin/coder-linux-amd64` inside the
   `coder` container (copied from the container's own `/opt/coder`, which
   is the exact deployed build). The fork image ships no embedded `/bin`
   agent binaries, so `/bin/coder-linux-amd64` 404'd and every workspace
   agent looped on download forever. Re-seed after any `docker rm`:

   ```sh
   docker cp coder:/opt/coder /tmp/coder-linux-amd64
   docker exec coder mkdir -p /home/coder/.cache/coder/site/orig/bin
   docker cp /tmp/coder-linux-amd64 coder:/home/coder/.cache/coder/site/orig/bin/coder-linux-amd64
   docker exec --user root coder chown coder:coder /home/coder/.cache/coder/site/orig/bin/coder-linux-amd64
   ```

   No restart is needed after seeding; coderd serves the file on demand.

## First admin credentials (host only, never in git)

- Initial admin user: `admin` / `admin@lnx-server.local`
- Password: `/home/trillium/.coder-first-admin-pass` (mode 600)
- Session token: `/home/trillium/.coder-admin-token` (mode 600)

The captain should log in at `http://lnx-server:7080/`, create his own
owner account, and rotate or remove the bootstrap `admin` password.

## Durability notes

- The template, workspaces, and users live in Coder's built-in PostgreSQL,
  whose data directory (`/home/coder/.config/coderv2/postgres`) is inside
  the `coder` container filesystem, as is the seeded agent binary above.
  Both survive container restarts and host reboots (restart policy
  `unless-stopped`) but NOT `docker rm` of the container. Deliberately NOT
  verified by restarting: the deployment is live, and a restart test was
  not worth the interruption. What persists across reboot was verified:
  the ufw rule (stored in `/etc/ufw`), the restart policy, this directory
  in git, and the host working copy below.
  Recreating the container to add persistent volumes for Postgres and the
  binary cache is a decision for the captain; until then, the recreate
  path is: re-push the template (next section), re-seed the binary
  (previous section), recreate workspaces.
- The built-in provisioner daemons (`scope=organization`, key `built-in`)
  start automatically with coderd; no separate provisioner needed on this
  single host.
- Host prerequisites already satisfied: `/var/run/docker.sock` mounted into
  the coder container, 74G free disk at setup time, 4 CPUs / 16G RAM.
