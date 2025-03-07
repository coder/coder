
 Troubleshooting

`coder ping <workspace>` will ping the workspace agent and print diagnostics on
the state of the connection. These diagnostics are created by inspecting both
the client and agent network configurations, and provide insights into why a
direct connection may be impeded, or why the quality of one might be degraded.

The `-v/--verbose` flag can be appended to the command to print client debug
logs.

```console
$ coder ping dev
pong from workspace proxied via  DERP(Council Bluffs, Iowa)  in 42ms
pong from workspace proxied via  DERP(Council Bluffs, Iowa)  in 41ms
pong from workspace proxied via  DERP(Council Bluffs, Iowa)  in 39ms
✔ preferred DERP region: 999 (Council Bluffs, Iowa)
✔ sent local data to Coder networking coordinator
✔ received remote agent data from Coder networking coordinator
    preferred DERP region: 999 (Council Bluffs, Iowa)
    endpoints: x.x.x.x:46433, x.x.x.x:46433, x.x.x.x:46433
✔ Wireguard handshake 11s ago

❗ You are connected via a DERP relay, not directly (p2p)
Possible client-side issues with direct connection:
 - Network interface utun0 has MTU 1280, (less than 1378), which may degrade the quality of direct connections

Possible agent-side issues with direct connection:
 - Agent is potentially behind a hard NAT, as multiple endpoints were retrieved from different STUN servers
 - Agent IP address is within an AWS range (AWS uses hard NAT)
```

## Common Problems with Direct Connections

### Disabled Deployment-wide

Direct connections can be disabled at the deployment level by setting the
`CODER_BLOCK_DIRECT` environment variable or the `--block-direct-connections`
flag on the server. When set, this will be reflected in the output of
`coder ping`.

### UDP Blocked

Some corporate firewalls block UDP traffic. Direct connections require UDP
traffic to be allowed between the client and agent, as well as between the
client/agent and STUN servers in most cases. `coder ping` will indicate if
either the Coder agent or client had issues sending or receiving UDP packets to
STUN servers.

If this is the case, you may need to add exceptions to the firewall to allow UDP
for Coder workspaces, clients, and STUN servers.

### Endpoint-Dependent NAT (Hard NAT)

Hard NATs prevent public endpoints gathered from STUN servers from being used by
the peer to establish a direct connection. Typically, if only one side of the
connection is behind a hard NAT, direct connections can still be established
easily. However, if both sides are behind hard NATs, direct connections may take
longer to establish or may not be possible at all.

`coder ping` will indicate if it's possible the client or agent is behind a hard
NAT.

Learn more about [STUN and NAT](./stun.md).

### No STUN Servers

If there are no STUN servers available within a deployment's DERP MAP, direct
connections may not be possible. Notable exceptions are if the client and agent
are on the same network, or if either is able to use UPnP instead of STUN to
resolve the public IP of the other. `coder ping` will indicate if no STUN
servers were found.

### Endpoint Firewalls

Direct connections may also be impeded if one side is behind a hard NAT and the
other is running a firewall that blocks ingress traffic from unknown 5-tuples
(Protocol, Source IP, Source Port, Destination IP, Destination Port).

If this is suspected, you may need to add an exception for Coder to the
firewall, or reconfigure the hard NAT.

### VPNs

If a VPN is the default route for all IP traffic, it may interfere with the
ability for clients and agents to form direct connections. This happens if the
NAT does not permit traffic to be
['hairpinned'](./stun.md#3-direct-connections-with-vpn-and-nat-hairpinning) from
the public IP address of the NAT (determined via STUN) to the internal IP
address of the agent.

If this is the case, you may need to add exceptions to the VPN for Coder, modify
the NAT configuration, or deploy an internal STUN server.

### Low MTU

If a network interface on the side of either the client or agent has an MTU
smaller than 1378, any direct connections form may have degraded quality or
performance, as IP packets are fragmented. `coder ping` will indicate if this is
the case by inspecting network interfaces on both the client and the workspace
agent.

If another interface cannot be used, and the MTU cannot be changed, you may need
to disable direct connections, and relay all traffic via DERP instead, which
will not be affected by the low MTU.

## Throughput

The `coder speedtest <workspace>` command measures the throughput between the
client and the workspace agent.

```console
$ coder speedtest workspace
29ms via coder
Starting a 5s download test...
INTERVAL       TRANSFER         BANDWIDTH
0.00-1.00 sec  630.7840 MBits   630.7404 Mbits/sec
1.00-2.00 sec  913.9200 MBits   913.8106 Mbits/sec
2.00-3.00 sec  943.1040 MBits   943.0399 Mbits/sec
3.00-4.00 sec  933.3760 MBits   933.2143 Mbits/sec
4.00-5.00 sec  848.8960 MBits   848.7019 Mbits/sec
5.00-5.02 sec  13.5680 MBits    828.8189 Mbits/sec
----------------------------------------------------
0.00-5.02 sec  4283.6480 MBits  853.8217 Mbits/sec
```
