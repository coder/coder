<!-- DO NOT EDIT | GENERATED CONTENT -->
# agent-firewall

Network isolation tool for monitoring and restricting HTTP/HTTPS requests

## Usage

```console
coder agent-firewall [flags] [args...]
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

### --jail-type

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$BOUNDARY_JAIL_TYPE</code> |
| YAML        | <code>jail_type</code>           |
| Default     | <code>nsjail</code>              |

Jail type to use for network isolation. Options: nsjail (default), landjail.

### --use-real-dns

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>bool</code>                   |
| Environment | <code>$BOUNDARY_USE_REAL_DNS</code> |
| YAML        | <code>use_real_dns</code>           |

Use real DNS in the jail instead of the dummy DNS (allows DNS exfiltration). Default: false.

### --no-user-namespace

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>bool</code>                        |
| Environment | <code>$BOUNDARY_NO_USER_NAMESPACE</code> |
| YAML        | <code>no_user_namespace</code>           |

Do not create a user namespace. Use in restricted environments that disallow user NS (e.g. Bottlerocket in EKS auto-mode).

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

### --enable-session-correlation

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>bool</code>                                  |
| Environment | <code>$BOUNDARY_SESSION_CORRELATION_ENABLED</code> |
| YAML        | <code>session_correlation_enabled</code>           |

Enable session correlation header injection. When no inject targets are configured, the target is auto-derived from CODER_AGENT_URL (set automatically inside Coder workspaces). Disable for deployments without Coder AI Gateway in front.

### --session-id-inject-target

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>string</code>                             |
| Environment | <code>$BOUNDARY_SESSION_ID_INJECT_TARGET</code> |

Inject target for session correlation headers. Repeat the flag once per target; each value describes exactly one target. Format: "domain=<host> [path=<glob>]". Example: --session-id-inject-target "domain=prod.coder.com path=/api/v2/aibridge/*".
