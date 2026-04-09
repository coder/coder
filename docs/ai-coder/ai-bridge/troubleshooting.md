# Troubleshooting

This page covers common issues when running AI Bridge behind load balancers and
reverse proxies, particularly on cloud platforms.

## Idle connection timeouts

AI Bridge uses Server-Sent Events (SSE) and long-lived HTTP connections to
stream responses from upstream LLM providers. These connections can remain idle
for extended periods while the model generates a response, especially for
complex or long-running requests.

Cloud load balancers enforce idle connection timeouts that close connections when
no data is transmitted within a configured window. When this timeout is shorter
than the time the model needs to start responding, the load balancer terminates
the connection and returns an error to the client.

### Symptoms

- Requests to AI Bridge fail with `502 Bad Gateway` or `504 Gateway Timeout`
  errors.
- Failures occur at a consistent time (for example, exactly 60 seconds),
  regardless of the request complexity.
- Short, simple requests succeed while longer requests fail.
- Error timing correlates with your load balancer's idle timeout setting rather
  than with the upstream provider's response time.

### AWS

AWS Application Load Balancers (ALBs) have a *default idle timeout of 60
seconds*. If no data is sent or received on a connection for this duration, the
ALB closes it.

This is a common issue when Coder is deployed on Amazon EKS and the
`CODER_ACCESS_URL` resolves to an ALB. AI coding clients (such as Codex or
Claude Code) connect to AI Bridge through the ALB, and inference requests that
take longer than 60 seconds to begin streaming will be terminated.

#### Solution

Increase the ALB idle timeout to accommodate long-running inference requests.
A value of 300 seconds (5 minutes) is a reasonable starting point, though you
may need to adjust based on your models and workloads.

**Using the AWS Console:**

1. Open the [Amazon EC2 console](https://console.aws.amazon.com/ec2/).
1. Navigate to **Load Balancers** under the **Load Balancing** section.
1. Select your ALB.
1. On the **Attributes** tab, choose **Edit**.
1. Update the **Idle timeout** value (maximum: 4000 seconds).
1. Choose **Save changes**.

**Using the AWS CLI:**

```sh
aws elbv2 modify-load-balancer-attributes \
  --load-balancer-arn <your-alb-arn> \
  --attributes Key=idle_timeout.timeout_seconds,Value=300
```

> [!NOTE]
> If increasing the ALB timeout is not feasible, an alternative workaround is to
> configure AI coding clients to connect to the Coder backplane using an
> internal Kubernetes service address (for example,
> `http://coder.coder.svc.cluster.local:8080`) instead of the external
> `CODER_ACCESS_URL`. This bypasses the ALB entirely for traffic originating
> from within the cluster.

For more information, see the
[AWS documentation on connection idle timeout](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html#connection-idle-timeout).

### GCP

Google Cloud HTTP(S) Load Balancers have a *default backend service timeout of
30 seconds*. This timeout controls how long the load balancer waits for the
backend to respond. For streaming responses like SSE, the load balancer may
close the connection if no data is received within this window.

GCP also enforces a non-configurable *stream idle timeout of 5 minutes* on
HTTP streams. Even if the backend service timeout is increased, an HTTP stream
that is idle for more than 5 minutes will be closed by the load balancer.

#### Solution

Increase the backend service timeout to accommodate inference workloads.

**Using the gcloud CLI:**

```sh
gcloud compute backend-services update <backend-service-name> \
  --global \
  --timeout=300
```

**Using the GCP Console:**

1. Open the [Google Cloud Console](https://console.cloud.google.com/).
1. Navigate to **Network services** > **Load balancing**.
1. Select your load balancer and click **Edit**.
1. In the **Backend configuration**, update the **Timeout** value.
1. Click **Update**.

> [!NOTE]
> For GKE Ingress-managed load balancers, you can set the timeout via a
> `BackendConfig` resource:
>
> ```yaml
> apiVersion: cloud.google.com/v1
> kind: BackendConfig
> metadata:
>   name: coder-backendconfig
> spec:
>   timeoutSec: 300
> ```
>
> Then annotate your Service:
>
> ```yaml
> metadata:
>   annotations:
>     cloud.google.com/backend-config: '{"default": "coder-backendconfig"}'
> ```

For more information, see the
[GCP documentation on backend service timeout](https://cloud.google.com/load-balancing/docs/backend-service#timeout-setting).
