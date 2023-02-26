<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder scaletest cleanup

Cleanup scaletest workspaces, then cleanup scaletest users. The strategy flags will apply to each stage of the cleanup process.

## Usage

```console
coder scaletest cleanup [flags]
```

## Flags

### --cleanup-concurrency

Number of concurrent cleanup jobs to run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CLEANUP_CONCURRENCY</code> |
| Default | <code>1</code> |

### --cleanup-job-timeout

Timeout per job. Jobs may take longer to complete under higher concurrency limits.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CLEANUP_JOB_TIMEOUT</code> |
| Default | <code>5m0s</code> |

### --cleanup-timeout

Timeout for the entire cleanup run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_LOADTEST_CLEANUP_TIMEOUT</code> |
| Default | <code>30m0s</code> |
