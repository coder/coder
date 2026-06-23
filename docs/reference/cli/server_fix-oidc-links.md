<!-- DO NOT EDIT | GENERATED CONTENT -->
# server fix-oidc-links

Reset OIDC linked IDs that do not match the expected issuer, allowing users to re-authenticate.

## Usage

```console
coder server fix-oidc-links [flags]
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass confirmation prompts.

### --postgres-url

|             |                                       |
|-------------|---------------------------------------|
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_PG_CONNECTION_URL</code> |

URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).

### --postgres-connection-auth

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>password\|awsiamrds</code>       |
| Environment | <code>$CODER_PG_CONNECTION_AUTH</code> |
| Default     | <code>password</code>                  |

Type of auth to use when connecting to postgres.

### --issuer-url

|             |                                     |
|-------------|-------------------------------------|
| Type        | <code>string</code>                 |
| Environment | <code>$CODER_OIDC_ISSUER_URL</code> |

The OIDC issuer URL. The canonical issuer is resolved via OIDC discovery.

### -n, --dry-run

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>bool</code>                          |
| Environment | <code>$CODER_FIX_OIDC_LINKS_DRY_RUN</code> |

Print analysis only, do not modify the database.

### --force-reset-all

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>bool</code>                                  |
| Environment | <code>$CODER_FIX_OIDC_LINKS_FORCE_RESET_ALL</code> |

Reset all OIDC linked IDs, not just those with a mismatched issuer. Mutually exclusive with --issuer-url.
