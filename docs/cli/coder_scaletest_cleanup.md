<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder scaletest cleanup


Cleanup scaletest workspaces, then cleanup scaletest users. The strategy flags will apply to each stage of the cleanup process.

## Usage
```console
coder scaletest cleanup [flags]
```


## Local Flags
| Name |  Default | Usage | Environment | 
| ---- |  ------- | ----- | -------- |
| --cleanup-concurrency |1 |<code>Number of concurrent cleanup jobs to run. 0 means unlimited.</code> | <code>$CODER_LOADTEST_CLEANUP_CONCURRENCY</code>  |
| --cleanup-job-timeout |5m0s |<code>Timeout per job. Jobs may take longer to complete under higher concurrency limits.</code> | <code>$CODER_LOADTEST_CLEANUP_JOB_TIMEOUT</code>  |
| --cleanup-timeout |30m0s |<code>Timeout for the entire cleanup run. 0 means unlimited.</code> | <code>$CODER_LOADTEST_CLEANUP_TIMEOUT</code>  |