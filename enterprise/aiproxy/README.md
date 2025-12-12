# AI Proxy Certificate Setup

This document describes how to set up MITM certificates for the AI proxy, including cross-signing for proxy chaining scenarios.

## Overview

The AI proxy uses MITM (Man-in-the-Middle) to intercept HTTPS traffic to AI providers. When chaining through an upstream SSL-bumping proxy (like Squid), both proxies need coordinated certificate trust.

## Certificate Hierarchy

```
┌─────────────────────────────────────┐
│  Upstream Proxy Root CA             │
│  (e.g., Squid Root CA)              │
│  Self-signed                        │
└──────────────┬──────────────────────┘
               │ signs
               ▼
┌─────────────────────────────────────┐
│  Downstream Proxy CA (intermediate) │
│  (AI Proxy's MITM CA)               │
│  Cross-signed by upstream           │
└──────────────┬──────────────────────┘
               │ signs
               ▼
┌─────────────────────────────────────┐
│  Leaf certificates                  │
│  (Generated per-site for MITM)      │
└─────────────────────────────────────┘
```

## Creating a New CA Key Pair

### 1. Generate a new private key

```sh
openssl genrsa -out mitm.key 2048
chmod 400 mitm.key
```

### 2. Create a self-signed CA certificate

```sh
openssl req -new -x509 -days 365 \
  -key mitm.key \
  -out mitm.crt \
  -subj "/CN=AI Proxy CA"
```

## Cross-Signing Against an Existing CA

Cross-signing allows your CA to be trusted by clients that already trust another root CA. This is essential for proxy chaining where an upstream proxy does SSL bumping.

### 1. Create a Certificate Signing Request (CSR) from your key

```sh
openssl req -new \
  -key mitm.key \
  -out mitm.csr \
  -subj "/CN=AI Proxy CA"
```

### 2. Create an extensions file for CA certificates

```sh
cat > ca_extensions.cnf << 'EOF'
basicConstraints=CA:TRUE
keyUsage=keyCertSign,cRLSign
EOF
```

### 3. Sign the CSR with the upstream CA

```sh
openssl x509 -req \
  -in mitm.csr \
  -CA upstream-ca.crt \
  -CAkey upstream-ca.key \
  -CAcreateserial \
  -out mitm-cross-signed.crt \
  -days 365 \
  -extfile ca_extensions.cnf
```

### 4. Create a certificate chain file

The chain file should contain your certificate followed by the upstream CA:

```sh
cat mitm-cross-signed.crt upstream-ca.crt > mitm-chain.crt
```

### 5. Verify the chain

```sh
# Check the signing relationship
openssl x509 -in mitm-cross-signed.crt -noout -subject -issuer

# Verify the chain is valid
openssl verify -CAfile upstream-ca.crt mitm-cross-signed.crt
```

Expected output:
```
subject=CN = AI Proxy CA
issuer=CN = Upstream Root CA
mitm-cross-signed.crt: OK
```

## Proxy Chaining Architecture

### Request Flow

```mermaid
sequenceDiagram
    participant Client
    participant AIProxy as AI Proxy<br/>(Downstream)
    participant Squid as Squid<br/>(Upstream)
    participant Coder as Coder Server<br/>(aibridge)
    participant Anthropic as Anthropic API

    Note over Client,Anthropic: Phase 1: Tunnel Establishment

    Client->>AIProxy: CONNECT api.anthropic.com:443<br/>Proxy-Authorization: Basic <coder-token>
    AIProxy->>Squid: CONNECT api.anthropic.com:443
    Squid->>Anthropic: TCP Connection
    Anthropic-->>Squid: Connected
    Squid-->>AIProxy: 200 Connection Established
    AIProxy-->>Client: 200 Connection Established

    Note over Client,Anthropic: Phase 2: TLS Handshakes (MITM)

    Client->>AIProxy: TLS ClientHello
    AIProxy->>AIProxy: Generate cert for api.anthropic.com<br/>signed by AI Proxy CA
    AIProxy-->>Client: TLS ServerHello<br/>(AI Proxy's cert)
    Client->>AIProxy: TLS Finished

    AIProxy->>Squid: TLS ClientHello
    Squid->>Squid: Generate cert for api.anthropic.com<br/>signed by Squid CA
    Squid-->>AIProxy: TLS ServerHello<br/>(Squid's cert)
    Note over AIProxy: Validates Squid's cert<br/>using UpstreamProxyCACert
    AIProxy->>Squid: TLS Finished

    Squid->>Anthropic: TLS ClientHello
    Anthropic-->>Squid: TLS ServerHello<br/>(Real cert)
    Squid->>Anthropic: TLS Finished

    Note over Client,Anthropic: Phase 3: Request Interception & Routing

    Client->>AIProxy: POST /v1/messages<br/>(to api.anthropic.com)
    AIProxy->>AIProxy: Decrypt request<br/>Extract coder-token<br/>Rewrite URL to aibridge

    AIProxy->>Squid: POST /api/v2/aibridge/anthropic/v1/messages<br/>(to Coder server)<br/>Authorization: Bearer <coder-token>
    Squid->>Squid: Decrypt, log, re-encrypt
    Squid->>Coder: POST /api/v2/aibridge/anthropic/v1/messages

    Note over Coder: Validate token<br/>Record usage<br/>Forward to provider

    Coder->>Anthropic: POST /v1/messages<br/>(with Anthropic API key)
    Anthropic-->>Coder: Response (streaming or JSON)
    Coder-->>Squid: Response
    Squid-->>AIProxy: Response
    AIProxy-->>Client: Response
```

### Trust Relationships

```mermaid
flowchart TB
    subgraph Client["Client Machine"]
        ClientTrust["Trusts: Squid Root CA<br/>(squid-ca.crt)"]
    end

    subgraph AIProxy["AI Proxy (Downstream)"]
        AIProxyCA["AI Proxy CA<br/>- Signs certs for clients<br/>- Cross-signed by Squid CA"]
        AIProxyTrust["Trusts: Squid Root CA<br/>(via UpstreamProxyCACert)"]
    end

    subgraph Squid["Squid Proxy (Upstream)"]
        SquidCA["Squid Root CA<br/>- Signs certs for AI Proxy<br/>- Self-signed root"]
    end

    subgraph Internet["Internet"]
        RealCA["Public CAs<br/>(DigiCert, Let's Encrypt, etc.)"]
        Target["Target Servers"]
    end

    ClientTrust -->|validates chain via| SquidCA
    AIProxyCA -->|signed by| SquidCA
    AIProxyTrust -->|validates| SquidCA
    Squid -->|trusts| RealCA
    RealCA -->|signs| Target
```

### Certificate Files Reference

| ID | File                                       | Purpose                                           | Used by                                        |
|----|--------------------------------------------|---------------------------------------------------|------------------------------------------------|
| A  | Upstream Root CA key (`squid-ca.key`)      | Signs intermediate CA; Signs fake certs for Squid | Upstream Proxy (Squid)                         |
| B  | Upstream Root CA cert (`squid-ca.crt`)     | Trust anchor for entire chain                     | AI Proxy (upstream trust); Client (root trust) |
| C  | AI Proxy CA key (`mitm.key`)               | Signs fake certificates for client connections    | AI Proxy                                       |
| D  | AI Proxy CA cert (`mitm-cross-signed.crt`) | Intermediate CA, signed by upstream root          | AI Proxy (part of chain served to clients)     |
| E  | Certificate chain (`mitm-chain.crt`)       | D + B combined, full chain                        | AI Proxy (loads as CA cert file)               |

> **Note**: Clients only need to trust `squid-ca.crt` (B). If this is already in the system trust store (e.g., corporate proxy CA), no additional client configuration is needed.

### Certificate Signing (Setup Time)

How the cross-signed certificate chain is created:

```mermaid
flowchart TD
    A["A: squid-ca.key"] -->|signs| D["D: mitm-cross-signed.crt"]
    C["C: mitm.key"] -->|generates CSR| D
    D -->|concatenate| E["E: mitm-chain.crt"]
    B["B: squid-ca.crt"] -->|concatenate| E
```

### Certificate Trust (Runtime)

```mermaid
flowchart LR
    Client -->|trusts| B["B: squid-ca.crt"]
    AIProxy["AI Proxy"] -->|trusts| B
    Squid -->|trusts| PublicCAs["Public CAs"]
```

> Clients validate the chain: leaf cert → D (intermediate) → B (root). Only B needs to be trusted.

### Certificate Usage (Runtime)

Which keys sign fake certificates:

```mermaid
flowchart LR
    C["C: mitm.key"] -->|signs fake certs| AIProxy["AI Proxy"]
    A["A: squid-ca.key"] -->|signs fake certs| Squid
```

## Troubleshooting

### Verifying Cross-Signing

Check that your certificate shows a different issuer than subject:

```sh
openssl x509 -in mitm-cross-signed.crt -noout -subject -issuer
```

If both are the same, the certificate is self-signed, not cross-signed.

### Testing the Chain

```sh
# Test with curl through the proxy
curl -x http://localhost:8888 \
  --cacert /path/to/ai-proxy-ca.crt \
  https://api.anthropic.com/v1/messages
```
