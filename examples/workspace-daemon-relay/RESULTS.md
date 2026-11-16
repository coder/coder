# What the proof of concept showed

## Checks

[validate.py](validate.py) ran against a development server with the relay
enforced and a workspace in a Docker container. 12 of 12 checks passed.

| Check | What it shows                                                                                                    |
|-------|------------------------------------------------------------------------------------------------------------------|
| A1    | Processes are given the daemon's local address as `CODER_URL`, whatever the template set                         |
| A2    | The command line works unchanged, through the daemon                                                             |
| A3    | The session token works through the daemon and not against the control plane directly, even from inside          |
| A4    | Other API calls and their responses pass through                                                                 |
| B1    | Outside the workspace the token is refused, and the answer says why                                              |
| B2    | A made-up lease does not help                                                                                    |
| B3    | A person's own session is as it was                                                                              |
| B4    | The control plane counts what it refused                                                                         |
| C1    | The daemon's token alone obtains no lease while the daemon runs                                                  |
| D1    | A daemon that was stopped gives the relay up, and the next one does not wait (working again after 1 s)           |
| D2    | A daemon that was killed is replaced once its lease has lapsed, and the takeover is recorded (after 60 s)        |
| D3    | While the new daemon waits, processes are already given its address, and are refused until it has a lease        |

Checked by hand in `log` mode: a session token without a lease is accepted,
the control plane logs that it would have been refused, and counts it.

The Go tests cover what the script does not: a lease of another workspace, a
lease that has run out, a request for a lease that is forged or old, a token
a user made that is named like a workspace's, AI Gateway's routes, streamed
responses and WebSockets through the relay.

## What the work turned up

- **The check has to sit where every API key is validated.** Routes validate
  keys with configurations of their own, for example workspace apps. A check
  on some routes would leave the token working on the others. The control
  plane now asks one function about every key, on every route.
- **AI Gateway is a second door.** It authenticates Coder tokens by itself
  and never reaches that function, so its routes got a check of their own.
- **A daemon must serve before it holds a lease.** A process keeps the
  address it was started with. A daemon that came back after a crash at first
  gave its startup scripts the control plane's own address, which they kept,
  and which refused them. The daemon now serves as soon as the control plane
  says the relay is on, and requests are vouched for once there is a lease.
- **A crash costs a minute.** After a kill, the workspace's session token is
  refused for up to a minute, until the old lease has lapsed. A clean stop
  costs nothing.
- **The extra hop was not measurable on one host.** A request through the
  daemon and a direct one both took 0.4 ms at the median, over 300 of each
  from inside the workspace. Most of them were answered by the rate limiter,
  so this measures the hop and not the API. It was not measured across a
  network.

## What it protects against, and what it does not

- **It protects against a copy of the token used elsewhere**: from a log, a
  transcript, a prompt sent to a model, Terraform state, or after the
  workspace has stopped.
- **It does not protect against a process inside the workspace.** Whatever
  runs there can use the relay, and can pass requests on for somebody
  outside. The relay ties the token to the workspace, not to a process.
- **The relay's port can be reached like any port of the workspace**: it is
  listed with the workspace's other listening ports, and port forwarding or
  a shared port reaches it. Sharing that port publicly would undo the relay
  for that workspace. Refusing to forward to it is not built.
- **A leaked daemon token is still a daemon token.** It obtains no lease
  while the daemon runs, and after the daemon stops it does, as a takeover
  that is recorded. Everything else a daemon's token does is unchanged.
- **A process running as the daemon's user can read the daemon's memory**,
  and with it the key and the lease.

## What breaks when it is enforced

- **Anything that takes the control plane's address from the template** and
  uses it with the workspace's session token. For example, a module that
  points a coding agent at AI Gateway with `data.coder_workspace.me.access_url`.
  Such tools have to take the address from `CODER_URL` when they start.
- **A template that uses the owner's session token while the workspace is
  built.** That request never passes through a daemon.
- **A process the daemon did not start**, which is not given `CODER_URL`.
- **Requests through a workspace proxy.** The relay forwards to the primary
  address only.

`log` mode exists to find these before enforcing.

## What is not done

- **The mode is an environment variable**, not a deployment option, and
  applies to the whole deployment. It cannot be turned on for some templates
  or organizations.
- **A lease is signed with the key for workspace app tokens.** It should have
  a key of its own.
- **A workspace's session token is recognised by its name.** It should be
  marked as such.
- **A lease is not checked against who holds the relay now.** A lease that
  was issued before a takeover works until it runs out, at most a minute.
- **The standalone AI Gateway and the AI Gateway proxy are not covered**,
  only AI Gateway inside the control plane.
- **Nothing proves where a daemon runs.** With a platform that vouches for
  the daemon, a takeover could be told from an attack.
- **Not tried:** a prebuilt workspace that is claimed, a dev container's
  sub-agent, Windows and macOS, and AI Gateway end to end with a model.
