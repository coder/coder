# coder scaletest cleanup

Cleanup any orphaned scaletest resources
## Usage
```console
coder scaletest cleanup [flags]
```

## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --cleanup-concurrency | 1 | <code>Number of concurrent cleanup jobs to run. 0 means unlimited.<br/>Consumes $CODER_LOADTEST_CLEANUP_CONCURRENCY</code>|
| --cleanup-job-timeout | 5m0s | <code>Timeout per job. Jobs may take longer to complete under higher concurrency limits.<br/>Consumes $CODER_LOADTEST_CLEANUP_JOB_TIMEOUT</code>|
| --cleanup-timeout | 30m0s | <code>Timeout for the entire cleanup run. 0 means unlimited.<br/>Consumes $CODER_LOADTEST_CLEANUP_TIMEOUT</code>|