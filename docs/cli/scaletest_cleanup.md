
# cleanup

 
Cleanup any orphaned scaletest resources


## Usage
```console
cleanup
```

## Description
```console
Cleanup scaletest workspaces, then cleanup scaletest users. The strategy flags will apply to each stage of the cleanup process.
```


## Options
### --cleanup-concurrency
Number of concurrent cleanup jobs to run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Number of concurrent cleanup jobs to run. 0 means unlimited.&lt;/code&gt; |
| Default |     &lt;code&gt;1&lt;/code&gt; |



### --cleanup-timeout
Timeout for the entire cleanup run. 0 means unlimited.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout for the entire cleanup run. 0 means unlimited.&lt;/code&gt; |
| Default |     &lt;code&gt;30m&lt;/code&gt; |



### --cleanup-job-timeout
Timeout per job. Jobs may take longer to complete under higher concurrency limits.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Timeout per job. Jobs may take longer to complete under higher concurrency limits.&lt;/code&gt; |
| Default |     &lt;code&gt;5m&lt;/code&gt; |


