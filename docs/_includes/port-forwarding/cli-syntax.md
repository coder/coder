This command can be used to forward TCP or UDP ports from the remote workspace so they can be accessed locally.
Both the TCP and UDP command line flags (`--tcp` and `--udp`) can be given once or multiple times.

The supported syntax variations for the `--tcp` and `--udp` flag are:

- Single port with optional remote port: `local_port[:remote_port]`
- Comma separation `local_port1,local_port2`
- Port ranges `start_port-end_port`
- Any combination of the above
