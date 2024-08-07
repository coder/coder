<!-- DO NOT EDIT | GENERATED CONTENT -->

# port-forward

Forward ports from a workspace to the local machine. For reverse port forwarding, use "coder ssh -R".

Aliases:

- tunnel

## Usage

```console
coder port-forward [flags] <workspace>
```

## Description

```console
  - Port forward a single TCP port from 1234 in the workspace to port 5678 on your
local machine:

     $ coder port-forward <workspace> --tcp 5678:1234

  - Port forward a single UDP port from port 9000 to port 9000 on your local
machine:

     $ coder port-forward <workspace> --udp 9000

  - Port forward multiple TCP ports and a UDP port:

     $ coder port-forward <workspace> --tcp 8080:8080 --tcp 9000:3000 --udp 5353:53

  - Port forward multiple ports (TCP or UDP) in condensed syntax:

     $ coder port-forward <workspace> --tcp 8080,9000:3000,9090-9092,10000-10002:10010-10012

  - Port forward specifying the local address to bind to:

     $ coder port-forward <workspace> --tcp 1.2.3.4:8080:8080
```

## Options

### -p, --tcp

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>string-array</code>            |
| Environment | <code>$CODER_PORT_FORWARD_TCP</code> |

Forward TCP port(s) from the workspace to the local machine.

### --udp

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>string-array</code>            |
| Environment | <code>$CODER_PORT_FORWARD_UDP</code> |

Forward UDP port(s) from the workspace to the local machine. The UDP connection has TCP-like semantics to support stateful UDP protocols.

### --disable-autostart

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>bool</code>                         |
| Environment | <code>$CODER_SSH_DISABLE_AUTOSTART</code> |
| Default     | <code>false</code>                        |

Disable starting the workspace automatically when connecting via SSH.
