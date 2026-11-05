To enable port forwarding via the dashboard, Coder must be configured with a [wildcard access URL](../../admin/setup/index.md#wildcard-access-url).
If an access URL is not specified, Coder will create [a publicly accessible URL](../../admin/setup/index.md#tunnel) to reverse proxy the deployment, and port forwarding will work.

There is a [DNS limitation](https://datatracker.ietf.org/doc/html/rfc1035#section-2.3.1) where each segment of hostnames must not exceed 63 characters.
If your app name, agent name, workspace name and username exceed 63 characters in the hostname, port forwarding via the dashboard will not work.
