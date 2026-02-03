<!-- DO NOT EDIT | GENERATED CONTENT -->
# login

Authenticate with Coder deployment

## Usage

```console
coder login [flags] [<url>]
```

## Description

```console
By default, the session token is stored in the operating system keyring on macOS and Windows and a plain text file on Linux. Use the --use-keyring flag or CODER_USE_KEYRING environment variable to change the storage mechanism.
```

## Subcommands

| Name                                   | Purpose                         |
|----------------------------------------|---------------------------------|
| [<code>token</code>](./login_token.md) | Print the current session token |

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
