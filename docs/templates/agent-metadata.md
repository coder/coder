# Agent Metadata

![agent-metadata](../images/agent-metadata.png)

With Agent Metadata, template admins can expose operational metrics from their
workspaces to their users. It is the dynamic complement of
[Resource Metadata](./resource-metadata.md).

See the
[Terraform reference](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent#metadata).

## Examples

All of these examples use
[heredoc strings](https://developer.hashicorp.com/terraform/language/expressions/strings#heredoc-strings)
for the script declaration. With heredoc strings, you can script without messy
escape codes, just as if you were working in your terminal.

Some of the below examples use the [`coder stat`](../cli/stat.md) command. This
is useful for determining CPU/memory usage inside a container, which can be
tricky otherwise.

Here's a standard set of metadata snippets for Linux agents:

```hcl
resource "coder_agent" "main" {
  os             = "linux"
  ...
  metadata {
    display_name = "CPU Usage"
    key  = "cpu"
    # Uses the coder stat command to get container CPU usage.
    script = "coder stat cpu"
    interval = 1
    timeout = 1
  }

  metadata {
    display_name = "Memory Usage"
    key  = "mem"
    # Uses the coder stat command to get container memory usage in GiB.
    script = "coder stat mem --prefix Gi"
    interval = 1
    timeout = 1
  }

  metadata {
    display_name = "CPU Usage (Host)"
    key  = "cpu_host"
    # calculates CPU usage by summing the "us", "sy" and "id" columns of
    # top.
    script = <<EOT
    top -bn1 | awk 'FNR==3 {printf "%2.0f%%", $2+$3+$4}'
    EOT
    interval = 1
    timeout = 1
  }

    metadata {
    display_name = "Memory Usage (Host)"
    key  = "mem_host"
    script = <<EOT
    free | awk '/^Mem/ { printf("%.0f%%", $4/$2 * 100.0) }'
    EOT
    interval = 1
    timeout = 1
  }

  metadata {
    display_name = "Disk Usage"
    key  = "disk"
    script = "df -h | awk '$6 ~ /^\\/$/ { print $5 }'"
    interval = 1
    timeout = 1
  }

  metadata {
    display_name = "Load Average"
    key  = "load"
    script = <<EOT
        awk '{print $1,$2,$3}' /proc/loadavg
    EOT
    interval = 1
    timeout = 1
  }
}
```

## Utilities

[top](https://linux.die.net/man/1/top) is available in most Linux distributions
and provides virtual memory, CPU and IO statistics. Running `top` produces
output that looks like:

```text
%Cpu(s): 65.8 us,  4.4 sy,  0.0 ni, 29.3 id,  0.3 wa,  0.0 hi,  0.2 si,  0.0 st
MiB Mem :  16009.0 total,    493.7 free,   4624.8 used,  10890.5 buff/cache
MiB Swap:      0.0 total,      0.0 free,      0.0 used.  11021.3 avail Mem
```

[vmstat](https://linux.die.net/man/8/vmstat) is available in most Linux
distributions and provides virtual memory, CPU and IO statistics. Running
`vmstat` produces output that looks like:

```text
procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu-----
r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st
0  0  19580 4781680 12133692 217646944    0    2     4    32    1    0  1  1 98  0  0
```

[dstat](https://linux.die.net/man/1/dstat) is considerably more parseable than
`vmstat` but often not included in base images. It is easily installed by most
package managers under the name `dstat`. The output of running `dstat 1 1` looks
like:

```text
--total-cpu-usage-- -dsk/total- -net/total- ---paging-- ---system--
usr sys idl wai stl| read  writ| recv  send|  in   out | int   csw
1   1  98   0   0|3422k   25M|   0     0 | 153k  904k| 123k  174k
```

## DB Write Load

Agent metadata can generate a significant write load and overwhelm your database
if you're not careful. The approximate writes per second can be calculated using
the following formula (applied once for each unique metadata interval):

```text
num_running_agents * write_multiplier / metadata_interval
```

For example, let's say you have:

- 10 running agents
- each with 4 metadata snippets
- where two have an interval of 4 seconds, and the other two 6 seconds

You can expect at most `(10 * 2 / 4) + (10 * 2 / 6)` or ~8 writes per second.
The actual writes per second may be a bit lower due to batching of metadata.
Adding more metadata with the same interval will not increase writes per second,
but it may still increase database load slightly.

We use a `write_multiplier` of `2` because each metadata write generates two
writes. One of the writes is to the `UNLOGGED` `workspace_agent_metadata` table
and the other to the `NOTIFY` query that enables live stats streaming in the UI.
