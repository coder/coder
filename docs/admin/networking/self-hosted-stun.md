---
title: Self-host a STUN server
---

Coder clients and workspace agents use Session Traversal Utilities for Network Address Translation (STUN) servers to discover the addresses they use for direct connections.
By default, Coder uses Google's public STUN servers.
This guide is for administrators who want to run their own STUN server with coturn or Tailscale's `stund`, and point Coder at it.

Run your own STUN server when:

- Your organization doesn't allow clients or workspaces to contact public STUN servers.
- Your clients and workspaces share a corporate network that doesn't support network address translation (NAT) hairpinning.
  Refer to the [NAT hairpinning example](./stun.md#3-direct-connections-with-vpn-and-nat-hairpinning).

If relayed connections are enough for your deployment, you can [disable STUN](../../reference/cli/server/index.md#--derp-server-stun-addresses) instead.

## Prerequisites

- A Linux virtual machine (VM) with a static IP address that every Coder client and workspace can reach over User Datagram Protocol (UDP).
  Use a public IP address if users connect from the internet.
  Use an address on your corporate network if all clients and workspaces connect through it.
- A firewall rule that allows inbound UDP traffic on port 3478 from every network your clients and workspaces connect from.
- A network path that preserves each client's source IP address and port.
  Proxies, ingress controllers, and load balancers that replace the source address prevent STUN from working.
  Kubernetes clusters usually expose services this way, so run the STUN server on a VM instead.
- Coder's built-in relay, which is enabled by default.
  Refer to [Configure Coder](#configure-coder) for details.

## Choose a STUN server

Both servers in this guide work with Coder direct connections.

| Server                                                                | Install options                          | Notes                                                                                                                   |
|-----------------------------------------------------------------------|------------------------------------------|-------------------------------------------------------------------------------------------------------------------------|
| [coturn](https://github.com/coturn/coturn)                            | Linux packages, official container image | Recommended. coturn is also a Traversal Using Relays around NAT (TURN) server, so this guide runs it in STUN-only mode. |
| [`stund`](https://github.com/tailscale/tailscale/tree/main/cmd/stund) | Build from source with Go                | A single binary from Tailscale. It ignores requests from generic STUN test tools, so use `coder netcheck` to test it.   |

## Run coturn

<div class="tabs">

### Linux package

1. Install coturn from your distribution's packages:

   ```sh
   sudo apt-get install -y coturn
   ```

1. Replace the contents of `/etc/turnserver.conf` with this STUN-only configuration:

   ```text
   listening-port=3478
   stun-only
   no-rfc5780
   no-tls
   no-cli
   no-software-attribute
   syslog
   ```

   `stun-only` turns off TURN relaying.
   `no-rfc5780` stops coturn from opening a second port for NAT behavior discovery, which Coder doesn't use.
   `no-tls` and `no-cli` turn off the Transport Layer Security (TLS) listener and the admin console.
   `no-software-attribute` hides the coturn version from STUN responses.
   Newer coturn releases enable some of these options by default and log a deprecation message for them at startup.

1. Enable and start the service:

   ```sh
   sudo systemctl enable --now coturn
   ```

1. Confirm that the service is running:

   ```sh
   systemctl is-active coturn
   ```

   The command prints `active`.

### Container

1. Run the official coturn image with STUN-only options:

   ```sh
   docker run -d --name coturn --restart unless-stopped \
     -p 3478:3478/udp \
     coturn/coturn:4.18.0 \
     -n --listening-port=3478 --stun-only --no-tls
   ```

   `-n` tells coturn to ignore its configuration file and use only the command-line options.

1. Confirm that the container is running:

   ```sh
   docker ps --filter name=coturn --format '{{.Status}}'
   ```

   The command prints a status that starts with `Up`.

On other container platforms, expose UDP port 3478 without a proxy that replaces the client's source address.

</div>

## Run stund

`stund` doesn't publish packages or container images, so you build it with Go.

1. Install [Go](https://go.dev/doc/install) on the VM.

1. Build `stund`:

   ```sh
   go install tailscale.com/cmd/stund@latest
   ```

1. Copy the binary to `/usr/local/bin`:

   ```sh
   sudo install -m 0755 "$(go env GOPATH)/bin/stund" /usr/local/bin/stund
   ```

1. Create `/etc/systemd/system/stund.service` with this content:

   ```ini
   [Unit]
   Description=Tailscale STUN server (stund)
   After=network-online.target
   Wants=network-online.target

   [Service]
   ExecStart=/usr/local/bin/stund -stun :3478 -http 127.0.0.1:3479
   DynamicUser=yes
   Restart=on-failure

   [Install]
   WantedBy=multi-user.target
   ```

   By default, `stund` also serves a debug and metrics endpoint on port 3479 on every interface.
   `-http 127.0.0.1:3479` keeps that endpoint off the network.

1. Reload systemd so it finds the new unit:

   ```sh
   sudo systemctl daemon-reload
   ```

1. Enable and start the service:

   ```sh
   sudo systemctl enable --now stund
   ```

1. Confirm that the service is running:

   ```sh
   systemctl is-active stund
   ```

   The command prints `active`.

## Configure Coder

Set [`CODER_DERP_SERVER_STUN_ADDRESSES`](../../reference/cli/server/index.md#--derp-server-stun-addresses) to a comma-separated list of your STUN servers, in `host:port` form:

```sh
CODER_DERP_SERVER_STUN_ADDRESSES=stun1.example.com:3478,stun2.example.com:3478
```

If you deploy Coder with Helm, add the variable to your `values.yaml`:

```yaml
coder:
  env:
    - name: "CODER_DERP_SERVER_STUN_ADDRESSES"
      value: "stun1.example.com:3478,stun2.example.com:3478"
```

Restart Coder to apply the change.
Running workspaces pick up the new STUN servers without a restart.

Keep these behaviors in mind:

- Your list replaces Google's STUN servers.
  Coder doesn't add them back.
- Each entry can be a hostname or an IP address, and must include the port.
  Hostnames must resolve from every network your clients and workspaces use.
- Run at least 2 STUN servers on separate hosts.
  Coder queries each server separately, so direct connections keep working if one server goes down.
- Coder applies this setting only when its built-in relay is enabled, which is the default.
  If you set `CODER_DERP_SERVER_ENABLE=false` and provide your own relay map with `CODER_DERP_CONFIG_URL` or `CODER_DERP_CONFIG_PATH`, Coder ignores `CODER_DERP_SERVER_STUN_ADDRESSES`.
  Add your STUN servers to that relay map instead.

## Verify direct connections

Run these commands from a client on a network your users connect from.

1. Check that the client can reach each STUN server.
   This example uses `jq` to shorten the `coder netcheck` output:

   ```sh
   coder netcheck | jq '{udp: .derp.netcheck.UDP, stun: [.derp.regions[] | select(.region.RegionCode | startswith("coder_stun")) | .node_reports[0] | {host: .node.HostName, can_stun: .stun.CanSTUN}]}'
   ```

   Each of your STUN servers appears with `"can_stun": true`:

   ```json
   {
     "udp": true,
     "stun": [
       {
         "host": "stun1.example.com",
         "can_stun": true
       },
       {
         "host": "stun2.example.com",
         "can_stun": true
       }
     ]
   }
   ```

1. Connect to a workspace:

   ```sh
   coder ping <workspace>
   ```

   The output reports a direct (`p2p`) connection:

   ```text
   pong from <workspace> p2p via  203.0.113.10:41641  in 25ms
   ...
   ✔ You are connected directly (p2p)
   ```

If `can_stun` is `false`, or `coder ping` reports `proxied via DERP`, check that UDP port 3478 is open between that network and your STUN server.
Refer to [UDP blocked](./troubleshooting.md#udp-blocked) for details.

## Learn more

- [STUN and NAT](./stun.md)
- [Networking troubleshooting](./troubleshooting.md)
- [coturn documentation](https://github.com/coturn/coturn/wiki)
