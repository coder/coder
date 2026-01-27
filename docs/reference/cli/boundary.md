<!-- DO NOT EDIT | GENERATED CONTENT -->
# boundary

Network isolation tool for monitoring and restricting HTTP/HTTPS requests

## Usage

```console
coder boundary [flags] [args...]
```

## Description

```console
boundary creates an isolated network environment for target processes, intercepting HTTP/HTTPS traffic through a transparent proxy that enforces user-defined allow rules.
```

## Options

### --config

|             |                               |
|-------------|-------------------------------|
| Type        | <code>yaml-config-path</code> |
| Environment | <code>$BOUNDARY_CONFIG</code> |

Path to YAML config file.

### --allow

|             |                              |
|-------------|------------------------------|
| Type        | <code>string</code>          |
| Environment | <code>$BOUNDARY_ALLOW</code> |

Allow rule (repeatable). These are merged with allowlist from config file. Format: "pattern" or "METHOD[,METHOD] pattern".

### --

|      |                           |
|------|---------------------------|
| Type | <code>string-array</code> |
| YAML | <code>allowlist</code>    |

Allowlist rules from config file (YAML only).

### --log-level

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$BOUNDARY_LOG_LEVEL</code> |
| YAML        | <code>log_level</code>           |
| Default     | <code>warn</code>                |

Set log level (error, warn, info, debug).

### --log-dir

|             |                                |
|-------------|--------------------------------|
| Type        | <code>string</code>            |
| Environment | <code>$BOUNDARY_LOG_DIR</code> |
| YAML        | <code>log_dir</code>           |

Set a directory to write logs to rather than stderr.

### --proxy-port

|             |                          |
|-------------|--------------------------|
| Type        | <code>int</code>         |
| Environment | <code>$PROXY_PORT</code> |
| YAML        | <code>proxy_port</code>  |
| Default     | <code>8080</code>        |

Set a port for HTTP proxy.

### --pprof

|             |                              |
|-------------|------------------------------|
| Type        | <code>bool</code>            |
| Environment | <code>$BOUNDARY_PPROF</code> |
| YAML        | <code>pprof_enabled</code>   |

Enable pprof profiling server.

### --pprof-port

|             |                                   |
|-------------|-----------------------------------|
| Type        | <code>int</code>                  |
| Environment | <code>$BOUNDARY_PPROF_PORT</code> |
| YAML        | <code>pprof_port</code>           |
| Default     | <code>6060</code>                 |

Set port for pprof profiling server.

### --configure-dns-for-local-stub-resolver

|             |                                                              |
|-------------|--------------------------------------------------------------|
| Type        | <code>bool</code>                                            |
| Environment | <code>$BOUNDARY_CONFIGURE_DNS_FOR_LOCAL_STUB_RESOLVER</code> |
| YAML        | <code>configure_dns_for_local_stub_resolver</code>           |

Configure DNS for local stub resolver (e.g., systemd-resolved). Only needed when /etc/resolv.conf contains nameserver 127.0.0.53.

### --jail-type

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$BOUNDARY_JAIL_TYPE</code> |
| YAML        | <code>jail_type</code>           |
| Default     | <code>nsjail</code>              |

Jail type to use for network isolation. Options: nsjail (default), landjail.

### --disable-audit-logs

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>bool</code>                |
| Environment | <code>$DISABLE_AUDIT_LOGS</code> |
| YAML        | <code>disable_audit_logs</code>  |

Disable sending of audit logs to the workspace agent when set to true.

### --log-proxy-socket-path

|             |                                                          |
|-------------|----------------------------------------------------------|
| Type        | <code>string</code>                                      |
| Environment | <code>$CODER_AGENT_BOUNDARY_LOG_PROXY_SOCKET_PATH</code> |
| Default     | <code>/tmp/boundary-audit.sock</code>                    |

Path to the socket where the boundary log proxy server listens for audit logs.

### --version

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Print version information and exit.
