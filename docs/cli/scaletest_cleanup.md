<!-- DO NOT EDIT | GENERATED CONTENT -->

# scaletest cleanup

Cleanup any orphaned scaletest resources

## Usage

```console
coder scaletest cleanup
```

## Description

```console
Cleanup scaletest workspaces, then cleanup scaletest users. The strategy flags will apply to each stage of the cleanup process.
```

## Options

### --cleanup-concurrency

|             |                                                   |
| ----------- | ------------------------------------------------- |
| Environment | <code>$CODER_SCALETEST_CLEANUP_CONCURRENCY</code> |
| Default     | <code>1</code>                                    |

Number of concurrent cleanup jobs to run. 0 means unlimited.

### --cleanup-timeout

|             |                                               |
| ----------- | --------------------------------------------- |
| Environment | <code>$CODER_SCALETEST_CLEANUP_TIMEOUT</code> |
| Default     | <code>30m</code>                              |

Timeout for the entire cleanup run. 0 means unlimited.

### --cleanup-job-timeout

|             |                                                   |
| ----------- | ------------------------------------------------- |
| Environment | <code>$CODER_SCALETEST_CLEANUP_JOB_TIMEOUT</code> |
| Default     | <code>5m</code>                                   |

Timeout per job. Jobs may take longer to complete under higher concurrency limits.
