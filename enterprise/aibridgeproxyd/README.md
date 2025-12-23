# AI Bridge Proxy

A MITM (Man-in-the-Middle) proxy server for intercepting and decrypting HTTPS requests to AI providers.

## Overview

The AI Bridge Proxy intercepts HTTPS traffic, decrypts it using a configured CA certificate, and forwards requests to AI Bridge for processing.

## Configuration

### Certificate Setup

Generate a CA key pair for MITM:

#### 1. Generate a new private key

```sh
openssl genrsa -out mitm.key 2048
chmod 400 mitm.key
```

#### 2. Create a self-signed CA certificate

```sh
openssl req -new -x509 -days 365 \
  -key mitm.key \
  -out mitm.crt \
  -subj "/CN=Coder AI Bridge Proxy CA"
```

### Configuration options

| Environment Variable               | Description                     | Default |
|------------------------------------|---------------------------------|---------|
| `CODER_AIBRIDGE_PROXY_ENABLED`     | Enable the AI Bridge Proxy      | `false` |
| `CODER_AIBRIDGE_PROXY_LISTEN_ADDR` | Address the proxy listens on    | `:8888` |
| `CODER_AIBRIDGE_PROXY_CERT_FILE`   | Path to the CA certificate file | -       |
| `CODER_AIBRIDGE_PROXY_KEY_FILE`    | Path to the CA private key file | -       |

### Client Configuration

Clients must trust the proxy's CA certificate and authenticate with their Coder session token.

#### CA Certificate

Clients need to trust the MITM CA certificate:

```sh
# Node.js
export NODE_EXTRA_CA_CERTS="/path/to/mitm.crt"

# Python (requests, httpx)
export REQUESTS_CA_BUNDLE="/path/to/mitm.crt"
export SSL_CERT_FILE="/path/to/mitm.crt"

# Go
export SSL_CERT_FILE="/path/to/mitm.crt"
```

#### Proxy Authentication

Clients authenticate with the proxy using their Coder session token in the `Proxy-Authorization` header via HTTP Basic Auth.
The token is passed as the password (username is ignored):

```sh
export HTTP_PROXY="http://ignored:<coder-session-token>@<proxy-host>:<proxy-port>"
export HTTPS_PROXY="http://ignored:<coder-session-token>@<proxy-host>:<proxy-port>"
```

For example:

```sh
export HTTP_PROXY="http://coder:${CODER_SESSION_TOKEN}@localhost:8888"
export HTTPS_PROXY="http://coder:${CODER_SESSION_TOKEN}@localhost:8888"
```

Most HTTP clients and AI SDKs will automatically use these environment variables.
