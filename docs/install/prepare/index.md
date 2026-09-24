# Prepare prerequisites

Preparation gathers the external pieces a deployment depends on.
Most of them are owned by another team, so their lead time, not the installation itself, usually sets your timeline.
Start the slowest items first.

## What to gather

| Prerequisite                        | Usual owner                     | Notes                                                                                                                                           |
|-------------------------------------|---------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| Access hostname and DNS records     | DNS and TLS owner               | You need the Coder hostname, and a wildcard record if you want to serve workspace apps on subdomains                                            |
| TLS certificates                    | DNS and TLS owner               | A wildcard certificate covers the hostname and the workspace app subdomains                                                                     |
| PostgreSQL database                 | Database administrator          | Include a backup and restore plan, not only a connection string                                                                                 |
| Identity provider application       | Identity provider administrator | Register the OIDC or SAML application and agree on the group claims you intend to map                                                           |
| Coder license                       | Licensing contact               | Required for Premium features; refer to [Licensing](./licensing.md)                                                                             |
| Container images and Coder binaries | Coder platform owner            | Only for restricted networks; refer to [Air-gapped deployments](./airgap.md) and [Mirror Coder Registry with Artifactory](./registry-mirror.md) |

## Lead times

Certificates, database provisioning, and identity provider registration are the items most likely to block an install, because each one needs a request to another team and a review cycle.
Requesting them while you're still planning is usually the difference between a deployment that takes days and one that takes weeks.

## Before you move on

You can install the control plane without every item on this list, and a proof of concept often does.
Treat the list as strong recommendations: each item you skip is work that reappears later, usually when real users already depend on the deployment.

Next: [Install the control plane](../server/index.md).

<children></children>
