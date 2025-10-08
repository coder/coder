<!-- DO NOT EDIT | GENERATED CONTENT -->
# login

Authenticate with Coder deployment

## Usage

```console
coder login [flags] [<url>]
```

## Description

```console
Stores the session token in the OS keyring, with a fallback to plain text if the keyring is not available. The security command is used on macOS. Windows Credential Manager API is used on Windows. Linux depends on GNOME keyring and Secret Service API (via D-Bus).
```

## Options

### --first-user-email

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_FIRST_USER_EMAIL</code> |

Specifies an email address to use if creating the first user for the deployment.

### --first-user-username

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_FIRST_USER_USERNAME</code> |

Specifies a username to use if creating the first user for the deployment.

### --first-user-full-name

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_FIRST_USER_FULL_NAME</code> |

Specifies a human-readable name for the first user of the deployment.

### --first-user-password

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_FIRST_USER_PASSWORD</code> |

Specifies a password to use if creating the first user for the deployment.

### --first-user-trial

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>bool</code>                    |
| Environment | <code>$CODER_FIRST_USER_TRIAL</code> |

Specifies whether a trial license should be provisioned for the Coder deployment or not.

### --use-token-as-session

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

By default, the CLI will generate a new session token when logging in. This flag will instead use the provided token as the session token.
