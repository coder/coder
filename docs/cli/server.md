
# server

 
Start a Coder server


## Usage
```console
server
```

## Subcommands
| Name |   Purpose |
| ---- |   ----- |
| [&lt;code&gt;create-admin-user&lt;/code&gt;](./server_create-admin-user) | Create a new admin user with the given username, email and password and adds it to every organization. |
| [&lt;code&gt;postgres-builtin-url&lt;/code&gt;](./server_postgres-builtin-url) | Output the connection URL for the built-in PostgreSQL deployment. |
| [&lt;code&gt;postgres-builtin-serve&lt;/code&gt;](./server_postgres-builtin-serve) | Run the built-in PostgreSQL deployment. |

## Options
### --access-url
The URL that users will use to access the Coder deployment.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The URL that users will use to access the Coder deployment.&lt;/code&gt; |

### --wildcard-access-url
Specifies the wildcard hostname to use for workspace applications in the form &#34;*.example.com&#34;.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies the wildcard hostname to use for workspace applications in the form &#34;*.example.com&#34;.&lt;/code&gt; |

### --redirect-to-access-url
Specifies whether to redirect requests that do not match the access URL host.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies whether to redirect requests that do not match the access URL host.&lt;/code&gt; |

### --autobuild-poll-interval
Interval to poll for scheduled workspace builds.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Interval to poll for scheduled workspace builds.&lt;/code&gt; |
| Default |     &lt;code&gt;1m0s&lt;/code&gt; |



### --http-address
HTTP bind address of the server. Unset to disable the HTTP endpoint.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;HTTP bind address of the server. Unset to disable the HTTP endpoint.&lt;/code&gt; |
| Default |     &lt;code&gt;127.0.0.1:3000&lt;/code&gt; |



### --tls-address
HTTPS bind address of the server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;HTTPS bind address of the server.&lt;/code&gt; |
| Default |     &lt;code&gt;127.0.0.1:3443&lt;/code&gt; |



### --address, -a
Bind address of the server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Bind address of the server.&lt;/code&gt; |

### --tls-enable
Whether TLS will be enabled.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether TLS will be enabled.&lt;/code&gt; |

### --tls-redirect-http-to-https
Whether HTTP requests will be redirected to the access URL (if it&#39;s a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether HTTP requests will be redirected to the access URL (if it&#39;s a https URL and TLS is enabled). Requests to local IP addresses are never redirected regardless of this setting.&lt;/code&gt; |
| Default |     &lt;code&gt;true&lt;/code&gt; |



### --tls-cert-file
Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Path to each certificate for TLS. It requires a PEM-encoded file. To configure the listener to use a CA certificate, concatenate the primary certificate and the CA certificate together. The primary certificate should appear first in the combined file.&lt;/code&gt; |

### --tls-client-ca-file
PEM-encoded Certificate Authority file used for checking the authenticity of client
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;PEM-encoded Certificate Authority file used for checking the authenticity of client&lt;/code&gt; |

### --tls-client-auth
Policy the server will follow for TLS Client Authentication. Accepted values are &#34;none&#34;, &#34;request&#34;, &#34;require-any&#34;, &#34;verify-if-given&#34;, or &#34;require-and-verify&#34;.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Policy the server will follow for TLS Client Authentication. Accepted values are &#34;none&#34;, &#34;request&#34;, &#34;require-any&#34;, &#34;verify-if-given&#34;, or &#34;require-and-verify&#34;.&lt;/code&gt; |
| Default |     &lt;code&gt;none&lt;/code&gt; |



### --tls-key-file
Paths to the private keys for each of the certificates. It requires a PEM-encoded file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Paths to the private keys for each of the certificates. It requires a PEM-encoded file.&lt;/code&gt; |

### --tls-min-version
Minimum supported version of TLS. Accepted values are &#34;tls10&#34;, &#34;tls11&#34;, &#34;tls12&#34; or &#34;tls13&#34;
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Minimum supported version of TLS. Accepted values are &#34;tls10&#34;, &#34;tls11&#34;, &#34;tls12&#34; or &#34;tls13&#34;&lt;/code&gt; |
| Default |     &lt;code&gt;tls12&lt;/code&gt; |



### --tls-client-cert-file
Path to certificate for client TLS authentication. It requires a PEM-encoded file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Path to certificate for client TLS authentication. It requires a PEM-encoded file.&lt;/code&gt; |

### --tls-client-key-file
Path to key for client TLS authentication. It requires a PEM-encoded file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Path to key for client TLS authentication. It requires a PEM-encoded file.&lt;/code&gt; |

### --derp-server-enable
Whether to enable or disable the embedded DERP relay server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether to enable or disable the embedded DERP relay server.&lt;/code&gt; |
| Default |     &lt;code&gt;true&lt;/code&gt; |



### --derp-server-region-id
Region ID to use for the embedded DERP server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Region ID to use for the embedded DERP server.&lt;/code&gt; |
| Default |     &lt;code&gt;999&lt;/code&gt; |



### --derp-server-region-code
Region code to use for the embedded DERP server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Region code to use for the embedded DERP server.&lt;/code&gt; |
| Default |     &lt;code&gt;coder&lt;/code&gt; |



### --derp-server-region-name
Region name that for the embedded DERP server.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Region name that for the embedded DERP server.&lt;/code&gt; |
| Default |     &lt;code&gt;Coder Embedded Relay&lt;/code&gt; |



### --derp-server-stun-addresses
Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.&lt;/code&gt; |
| Default |     &lt;code&gt;stun.l.google.com:19302&lt;/code&gt; |



### --derp-server-relay-url
An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;An HTTP URL that is accessible by other replicas to relay DERP traffic. Required for high availability.&lt;/code&gt; |

### --derp-config-url
URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/&lt;/code&gt; |

### --derp-config-path
Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Path to read a DERP mapping from. See: https://tailscale.com/kb/1118/custom-derp-servers/&lt;/code&gt; |

### --prometheus-enable
Serve prometheus metrics on the address defined by prometheus address.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Serve prometheus metrics on the address defined by prometheus address.&lt;/code&gt; |

### --prometheus-address
The bind address to serve prometheus metrics.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The bind address to serve prometheus metrics.&lt;/code&gt; |
| Default |     &lt;code&gt;127.0.0.1:2112&lt;/code&gt; |



### --pprof-enable
Serve pprof metrics on the address defined by pprof address.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Serve pprof metrics on the address defined by pprof address.&lt;/code&gt; |

### --pprof-address
The bind address to serve pprof.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The bind address to serve pprof.&lt;/code&gt; |
| Default |     &lt;code&gt;127.0.0.1:6060&lt;/code&gt; |



### --oauth2-github-client-id
Client ID for Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Client ID for Login with GitHub.&lt;/code&gt; |

### --oauth2-github-client-secret
Client secret for Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Client secret for Login with GitHub.&lt;/code&gt; |

### --oauth2-github-allowed-orgs
Organizations the user must be a member of to Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Organizations the user must be a member of to Login with GitHub.&lt;/code&gt; |

### --oauth2-github-allowed-teams
Teams inside organizations the user must be a member of to Login with GitHub. Structured as: &lt;organization-name&gt;/&lt;team-slug&gt;.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Teams inside organizations the user must be a member of to Login with GitHub. Structured as: &lt;organization-name&gt;/&lt;team-slug&gt;.&lt;/code&gt; |

### --oauth2-github-allow-signups
Whether new users can sign up with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether new users can sign up with GitHub.&lt;/code&gt; |

### --oauth2-github-allow-everyone
Allow all logins, setting this option means allowed orgs and teams must be empty.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Allow all logins, setting this option means allowed orgs and teams must be empty.&lt;/code&gt; |

### --oauth2-github-enterprise-base-url
Base URL of a GitHub Enterprise deployment to use for Login with GitHub.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Base URL of a GitHub Enterprise deployment to use for Login with GitHub.&lt;/code&gt; |

### --oidc-allow-signups
Whether new users can sign up with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether new users can sign up with OIDC.&lt;/code&gt; |
| Default |     &lt;code&gt;true&lt;/code&gt; |



### --oidc-client-id
Client ID to use for Login with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Client ID to use for Login with OIDC.&lt;/code&gt; |

### --oidc-client-secret
Client secret to use for Login with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Client secret to use for Login with OIDC.&lt;/code&gt; |

### --oidc-email-domain
Email domains that clients logging in with OIDC must match.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Email domains that clients logging in with OIDC must match.&lt;/code&gt; |

### --oidc-issuer-url
Issuer URL to use for Login with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Issuer URL to use for Login with OIDC.&lt;/code&gt; |

### --oidc-scopes
Scopes to grant when authenticating with OIDC.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Scopes to grant when authenticating with OIDC.&lt;/code&gt; |
| Default |     &lt;code&gt;openid,profile,email&lt;/code&gt; |



### --oidc-ignore-email-verified
Ignore the email_verified claim from the upstream provider.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Ignore the email_verified claim from the upstream provider.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --oidc-username-field
OIDC claim field to use as the username.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;OIDC claim field to use as the username.&lt;/code&gt; |
| Default |     &lt;code&gt;preferred_username&lt;/code&gt; |



### --oidc-group-field
Change the OIDC default &#39;groups&#39; claim field. By default, will be &#39;groups&#39; if present in the oidc scopes argument.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Change the OIDC default &#39;groups&#39; claim field. By default, will be &#39;groups&#39; if present in the oidc scopes argument.&lt;/code&gt; |

### --oidc-sign-in-text
The text to show on the OpenID Connect sign in button
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The text to show on the OpenID Connect sign in button&lt;/code&gt; |
| Default |     &lt;code&gt;OpenID Connect&lt;/code&gt; |



### --oidc-icon-url
URL pointing to the icon to use on the OepnID Connect login button
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL pointing to the icon to use on the OepnID Connect login button&lt;/code&gt; |

### --telemetry
Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether telemetry is enabled or not. Coder collects anonymized usage data to help improve our product.&lt;/code&gt; |
| Default |     &lt;code&gt;true&lt;/code&gt; |



### --telemetry-trace
Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether Opentelemetry traces are sent to Coder. Coder collects anonymized application tracing to help improve our product. Disabling telemetry also disables this option.&lt;/code&gt; |
| Default |     &lt;code&gt;true&lt;/code&gt; |



### --telemetry-url
URL to send telemetry.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL to send telemetry.&lt;/code&gt; |
| Default |     &lt;code&gt;https://telemetry.coder.com&lt;/code&gt; |



### --trace
Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether application tracing data is collected. It exports to a backend configured by environment variables. See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md&lt;/code&gt; |

### --trace-honeycomb-api-key
Enables trace exporting to Honeycomb.io using the provided API Key.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enables trace exporting to Honeycomb.io using the provided API Key.&lt;/code&gt; |

### --trace-logs
Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enables capturing of logs as events in traces. This is useful for debugging, but may result in a very large amount of events being sent to the tracing backend which may incur significant costs. If the verbose flag was supplied, debug-level logs will be included.&lt;/code&gt; |

### --provisioner-daemons
Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Number of provisioner daemons to create on start. If builds are stuck in queued state for a long time, consider increasing this.&lt;/code&gt; |
| Default |     &lt;code&gt;3&lt;/code&gt; |



### --provisioner-daemon-poll-interval
Time to wait before polling for a new job.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Time to wait before polling for a new job.&lt;/code&gt; |
| Default |     &lt;code&gt;1s&lt;/code&gt; |



### --provisioner-daemon-poll-jitter
Random jitter added to the poll interval.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Random jitter added to the poll interval.&lt;/code&gt; |
| Default |     &lt;code&gt;100ms&lt;/code&gt; |



### --provisioner-force-cancel-interval
Time to force cancel provisioning tasks that are stuck.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Time to force cancel provisioning tasks that are stuck.&lt;/code&gt; |
| Default |     &lt;code&gt;10m0s&lt;/code&gt; |



### --dangerous-disable-rate-limits
Disables all rate limits. This is not recommended in production.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Disables all rate limits. This is not recommended in production.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --api-rate-limit
Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Maximum number of requests per minute allowed to the API per user, or per IP address for unauthenticated users. Negative values mean no rate limit. Some API endpoints have separate strict rate limits regardless of this value to prevent denial-of-service or brute force attacks.&lt;/code&gt; |
| Default |     &lt;code&gt;512&lt;/code&gt; |



### --verbose, -v
Output debug-level logs.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Output debug-level logs.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --log-human
Output human-readable logs to a given file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Output human-readable logs to a given file.&lt;/code&gt; |
| Default |     &lt;code&gt;/dev/stderr&lt;/code&gt; |



### --log-json
Output JSON logs to a given file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Output JSON logs to a given file.&lt;/code&gt; |

### --log-stackdriver
Output Stackdriver compatible logs to a given file.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Output Stackdriver compatible logs to a given file.&lt;/code&gt; |

### --dangerous-allow-path-app-sharing
Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Allow workspace apps that are not served from subdomains to be shared. Path-based app sharing is DISABLED by default for security purposes. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --dangerous-allow-path-app-site-owner-access
Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Allow site-owners to access workspace apps from workspaces they do not own. Owners cannot access path-based apps they do not own by default. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. Path-based apps can be disabled entirely with --disable-path-apps for further security.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --experiments
Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter &#39;*&#39; to opt-in to all available experiments.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter &#39;*&#39; to opt-in to all available experiments.&lt;/code&gt; |

### --update-check
Periodically check for new releases of Coder and inform the owner. The check is performed once per day.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Periodically check for new releases of Coder and inform the owner. The check is performed once per day.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --max-token-lifetime
The maximum lifetime duration users can specify when creating an API token.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The maximum lifetime duration users can specify when creating an API token.&lt;/code&gt; |
| Default |     &lt;code&gt;2562047h47m16.854775807s&lt;/code&gt; |



### --swagger-enable
Expose the swagger endpoint via /swagger.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Expose the swagger endpoint via /swagger.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --proxy-trusted-headers
Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Headers to trust for forwarding IP addresses. e.g. Cf-Connecting-Ip, True-Client-Ip, X-Forwarded-For&lt;/code&gt; |

### --proxy-trusted-origins
Origin addresses to respect &#34;proxy-trusted-headers&#34;. e.g. 192.168.1.0/24
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Origin addresses to respect &#34;proxy-trusted-headers&#34;. e.g. 192.168.1.0/24&lt;/code&gt; |

### --cache-dir
The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The directory to cache temporary files. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.&lt;/code&gt; |
| Default |     &lt;code&gt;/home/coder/.cache/coder&lt;/code&gt; |



### --in-memory
Controls whether data will be stored in an in-memory database.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Controls whether data will be stored in an in-memory database.&lt;/code&gt; |

### --postgres-url
URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with &#34;coder server postgres-builtin-url&#34;.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL of a PostgreSQL database. If empty, PostgreSQL binaries will be downloaded from Maven (https://repo1.maven.org/maven2) and store all data in the config root. Access the built-in database with &#34;coder server postgres-builtin-url&#34;.&lt;/code&gt; |

### --secure-auth-cookie
Controls if the &#39;Secure&#39; property is set on browser session cookies.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Controls if the &#39;Secure&#39; property is set on browser session cookies.&lt;/code&gt; |

### --strict-transport-security
Controls if the &#39;Strict-Transport-Security&#39; header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Controls if the &#39;Strict-Transport-Security&#39; header is set on all static file responses. This header should only be set if the server is accessed via HTTPS. This value is the MaxAge in seconds of the header.&lt;/code&gt; |
| Default |     &lt;code&gt;0&lt;/code&gt; |



### --strict-transport-security-options
Two optional fields can be set in the Strict-Transport-Security header; &#39;includeSubDomains&#39; and &#39;preload&#39;. The &#39;strict-transport-security&#39; flag must be set to a non-zero value for these options to be used.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Two optional fields can be set in the Strict-Transport-Security header; &#39;includeSubDomains&#39; and &#39;preload&#39;. The &#39;strict-transport-security&#39; flag must be set to a non-zero value for these options to be used.&lt;/code&gt; |

### --ssh-keygen-algorithm
The algorithm to use for generating ssh keys. Accepted values are &#34;ed25519&#34;, &#34;ecdsa&#34;, or &#34;rsa4096&#34;.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The algorithm to use for generating ssh keys. Accepted values are &#34;ed25519&#34;, &#34;ecdsa&#34;, or &#34;rsa4096&#34;.&lt;/code&gt; |
| Default |     &lt;code&gt;ed25519&lt;/code&gt; |



### --metrics-cache-refresh-interval
How frequently metrics are refreshed
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;How frequently metrics are refreshed&lt;/code&gt; |
| Default |     &lt;code&gt;1h0m0s&lt;/code&gt; |



### --agent-stats-refresh-interval
How frequently agent stats are recorded
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;How frequently agent stats are recorded&lt;/code&gt; |
| Default |     &lt;code&gt;30s&lt;/code&gt; |



### --agent-fallback-troubleshooting-url
URL to use for agent troubleshooting when not set in the template
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL to use for agent troubleshooting when not set in the template&lt;/code&gt; |
| Default |     &lt;code&gt;https://coder.com/docs/coder-oss/latest/templates#troubleshooting-templates&lt;/code&gt; |



### --audit-logging
Specifies whether audit logging is enabled.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies whether audit logging is enabled.&lt;/code&gt; |
| Default |     &lt;code&gt;true&lt;/code&gt; |



### --browser-only
Whether Coder only allows connections to workspaces via the browser.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Whether Coder only allows connections to workspaces via the browser.&lt;/code&gt; |

### --scim-auth-header
Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication.&lt;/code&gt; |

### --disable-path-apps
Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Disable workspace apps that are not served from subdomains. Path-based apps can make requests to the Coder API and pose a security risk when the workspace serves malicious JavaScript. This is recommended for security purposes if a --wildcard-access-url is configured.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --session-duration
The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The token expiry duration for browser sessions. Sessions may last longer if they are actively making requests, but this functionality can be disabled via --disable-session-expiry-refresh.&lt;/code&gt; |
| Default |     &lt;code&gt;24h0m0s&lt;/code&gt; |



### --disable-session-expiry-refresh
Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Disable automatic session expiry bumping due to activity. This forces all sessions to become invalid after the session expiry duration has been reached.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --disable-password-auth
Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Disable password authentication. This is recommended for security purposes in production deployments that rely on an identity provider. Any user with the owner role will be able to sign in with their password regardless of this setting to avoid potential lock out. If you are locked out of your account, you can use the `coder server create-admin` command to create a new admin user directly in the database.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |



### --config, -c
Specify a YAML file to load configuration from.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify a YAML file to load configuration from.&lt;/code&gt; |

### --write-config
&lt;br/&gt;Write out the current server configuration to the path specified by --config.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;
Write out the current server configuration to the path specified by --config.&lt;/code&gt; |

### --
Support links to display in the top right drop down menu.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Support links to display in the top right drop down menu.&lt;/code&gt; |

### --
Git Authentication providers
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Git Authentication providers&lt;/code&gt; |
