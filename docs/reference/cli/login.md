<!-- DO NOT EDIT | GENERATED CONTENT -->

# login

Authenticate with Coder deployment

## Usage

```console
coder login [flags] [<url>]
```

## Options

### --first-user-email

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_FIRST_USER_EMAIL</code> |

Specifies an email address to use if creating the first user for the deployment.

### --first-user-username

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_FIRST_USER_USERNAME</code> |

Specifies a username to use if creating the first user for the deployment.

### --first-user-full-name

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_FIRST_USER_FULL_NAME</code> |

Specifies a human-readable name for the first user of the deployment.

### --first-user-password

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_FIRST_USER_PASSWORD</code> |

Specifies a password to use if creating the first user for the deployment.

### --first-user-trial

|             |                                      |
| ----------- | ------------------------------------ |
| Type        | <code>bool</code>                    |
| Environment | <code>$CODER_FIRST_USER_TRIAL</code> |

Specifies whether a trial license should be provisioned for the Coder deployment or not.

### --use-token-as-session

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

By default, the CLI will generate a new session token when logging in. This flag will instead use the provided token as the session token.

### --first-user-first-name

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_FIRST_USER_FIRST_NAME</code> |

Specifies the first name of the user.

### --first-user-last-name

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_FIRST_USER_LAST_NAME</code> |

Specifies the last name of the user.

### --first-user-phone-number

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_FIRST_USER_PHONE_NUMBER</code> |

Specifies the phone number of the user.

### --first-user-job-title

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>string</code>                      |
| Environment | <code>$CODER_FIRST_USER_JOB_TITLE</code> |

Specifies the job title of the user.

### --first-user-company-name

|             |                                             |
| ----------- | ------------------------------------------- |
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_FIRST_USER_COMPANY_NAME</code> |

Specifies the company name of the user.

### --first-user-country

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_FIRST_USER_COUNTRY</code> |

Specifies the country of the user.

### --first-user-developers

|             |                                           |
| ----------- | ----------------------------------------- |
| Type        | <code>string</code>                       |
| Environment | <code>$CODER_FIRST_USER_DEVELOPERS</code> |

Specifies the number of developers.
