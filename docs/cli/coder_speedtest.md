# coder speedtest

Run upload and download tests from your machine to a workspace
## Usage
```console
coder speedtest <workspace> [flags]
```

## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --direct, -d | false | <code>Specifies whether to wait for a direct connection before testing speed.</code>|
| --reverse, -r | false | <code>Specifies whether to run in reverse mode where the client receives and the server sends.</code>|
| --time, -t | 5s | <code>Specifies the duration to monitor traffic.</code>|