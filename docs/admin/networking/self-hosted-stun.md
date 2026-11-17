---
title: Self-host a STUN server
---

Coder clients and workspace agents use Session Traversal Utilities for Network Address Translation (STUN) servers to discover the addresses they use for direct connections.
By default, Coder uses Google's public STUN servers.
This guide is for administrators who want to run their own STUN server with coturn and point Coder at it.

Run your own STUN server when:

- Your organization doesn't allow clients or workspaces to contact public STUN servers.
- Your clients and workspaces share a corporate network where the network address translation (NAT) device by which they reach public STUN servers doesn't support hairpinning.
  Refer to the [NAT hairpinning example](./stun.md#3-direct-connections-with-vpn-and-nat-hairpinning).

If relayed connections are enough for your deployment, you can [disable STUN](../../reference/cli/server/index.md#--derp-server-stun-addresses) instead.

## Prerequisites

- A Linux virtual machine (VM) with a static IP address that every Coder client and workspace can reach over User Datagram Protocol (UDP).
  Use a public IP address if users connect from the internet.
  Use an address on your corporate network if all clients and workspaces connect through it.
- A firewall rule that allows inbound UDP traffic on port 3478 from every network your clients and workspaces connect from.
- A network path that preserves each client's source IP address and port.
  Reverse proxies, ingress controllers, and load balancers that replace the source address prevent STUN from working.
  Kubernetes clusters usually expose services this way, so run the STUN server on a VM instead.

## Run coturn

[coturn](https://github.com/coturn/coturn) is a STUN and Traversal Using Relays around NAT (TURN) server.
This guide runs it in STUN-only mode.

<div class="tabs">

### Linux package

1. Install coturn from your distribution's packages.
   On Debian or Ubuntu, run:

   ```sh
   sudo apt-get install -y coturn
   ```

1. Replace the contents of coturn's configuration file with this STUN-only configuration.
   On Debian and Ubuntu, the file is `/etc/turnserver.conf`:

   ```txt
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

## Configure Coder

Set [`CODER_DERP_SERVER_STUN_ADDRESSES`](../../reference/cli/server/index.md#--derp-server-stun-addresses) to a comma-separated list of your STUN servers, in `host:port` form.
Coder ignores this setting when you disable the built-in DERP relay with `CODER_DERP_SERVER_ENABLE=false`.
In that case, [use a DERP map](#use-a-custom-derp-map) instead.

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
- Run at least two STUN servers on separate hosts.
  Coder lists each server as its own region, so a single unreachable server doesn't remove the others.

### Use a custom DERP map

Coder's built-in DERP relay is optional for self-hosted STUN.
You need the built-in DERP relay only to configure STUN servers with `CODER_DERP_SERVER_STUN_ADDRESSES`, because Coder ignores that setting when you set `CODER_DERP_SERVER_ENABLE=false`.

Without the built-in DERP relay, add your STUN servers to the [DERP map](./index.md#custom-relays) that you provide with `CODER_DERP_CONFIG_URL` or `CODER_DERP_CONFIG_PATH`:

- Give each STUN server its own region with a unique `RegionID`.
- Give each region one node with a unique `Name`, the STUN server's `HostName`, `"STUNOnly": true`, and `"STUNPort": 3478`.
- Keep at least one relay node in the map.
  Coder doesn't start if the DERP map contains only STUN servers.

## Verify direct connections

Run these commands from a client on a network your users connect from.
The first command uses `jq` to shorten the `coder netcheck` output, so install `jq` on the client if it isn't already installed.

1. Check that the client can reach each STUN server:

   ```sh
   coder netcheck | jq '{udp: .derp.netcheck.UDP, stun: [.derp.regions[].node_reports[] | select(.node.STUNOnly) | {host: .node.HostName, can_stun: .stun.CanSTUN}]}'
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

   ```txt
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
