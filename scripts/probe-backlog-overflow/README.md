# probe-backlog-overflow

Diagnostic for [PLAT-307](https://linear.app/codercom/issue/PLAT-307).
Measures how the local OS responds to TCP accept-queue overflow.

The probe opens a listening socket with a configurable backlog, never
accepts, then fires N concurrent dials. Each dial is classified into one
of: `success`, `refused` (`ECONNREFUSED` / `WSAECONNREFUSED`), `reset`
(`ECONNRESET`), `timeout` (context deadline exceeded), or `other`.

The hypothesis under test:

- On **Linux** with `net.ipv4.tcp_abort_on_overflow=0` (default), the kernel
  silently drops SYN packets that overflow the accept queue. Clients
  retransmit per `tcp_syn_retries` until either the connection accepts or
  the context expires. We expect **success + timeout**, no `refused`.

- On **Windows**, accept-queue overflow causes the kernel to send RST to
  the SYN. Clients see immediate `WSAECONNREFUSED`. We expect **success +
  refused**, no `timeout`.

If both predictions hold across the two probe branches, the
PLAT-307 backlog-overflow hypothesis is confirmed.

## Run locally

```sh
go run ./scripts/probe-backlog-overflow -backlog 1 -dials 200 -timeout 6s
```

## CI

Each probe branch (`probe/tcp-backlog-overflow-linux`,
`probe/tcp-backlog-overflow-windows`) contains a workflow that runs this
probe on the matching runner. Trigger via `workflow_dispatch` or by pushing
to the branch.
