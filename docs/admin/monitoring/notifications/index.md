# Notifications

Notifications are sent by Coder in response to specific internal events, such as
a workspace being deleted or a user being created.

## Enable experiment

In order to activate the notifications feature on Coder v2.15.X, you'll need to
enable the `notifications` experiment. Notifications are enabled by default
starting in v2.16.0.

```bash
# Using the CLI flag
$ coder server --experiments=notifications

# Alternatively, using the `CODER_EXPERIMENTS` environment variable
$ CODER_EXPERIMENTS=notifications coder server
```

More information on experiments can be found
[here](https://coder.com/docs/contributing/feature-stages#experimental-features).

## Event Types

Notifications are sent in response to internal events, to alert the affected
user(s) of this event. Currently we support the following list of events:

### Workspace Events

_These notifications are sent to the workspace owner._

- Workspace Deleted
- Workspace Manual Build Failure
- Workspace Automatic Build Failure
- Workspace Automatically Updated
- Workspace Dormant
- Workspace Marked For Deletion

### User Events

_These notifications are sent to users with **owner** and **user admin** roles._

- User Account Created
- User Account Deleted
- User Account Suspended
- User Account Activated
- _(coming soon) User Password Reset_
- _(coming soon) User Email Verification_

_These notifications are sent to the user themselves._

- User Account Suspended
- User Account Activated

### Template Events

_These notifications are sent to users with **template admin** roles._

- Template Deleted

## Configuration

You can modify the notification delivery behavior using the following server
flags.

| Required | CLI                                 | Env                                     | Type       | Description                                                                                                           | Default |
| :------: | ----------------------------------- | --------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------- | ------- |
|    ✔️    | `--notifications-dispatch-timeout`  | `CODER_NOTIFICATIONS_DISPATCH_TIMEOUT`  | `duration` | How long to wait while a notification is being sent before giving up.                                                 | 1m      |
|    ✔️    | `--notifications-method`            | `CODER_NOTIFICATIONS_METHOD`            | `string`   | Which delivery method to use (available options: 'smtp', 'webhook'). See [Delivery Methods](#delivery-methods) below. | smtp    |
|    -️    | `--notifications-max-send-attempts` | `CODER_NOTIFICATIONS_MAX_SEND_ATTEMPTS` | `int`      | The upper limit of attempts to send a notification.                                                                   | 5       |

## Delivery Methods

Notifications can currently be delivered by either SMTP or webhook. Each message
can only be delivered to one method, and this method is configured globally with
[`CODER_NOTIFICATIONS_METHOD`](../../../reference/cli/server.md#--notifications-method)
(default: `smtp`).

Enterprise customers can configure which method to use for each of the supported
[Events](#events); see the [Preferences](#preferences) section below for more
details.

## SMTP (Email)

Use the `smtp` method to deliver notifications by email to your users. Coder
does not ship with an SMTP server, so you will need to configure Coder to use an
existing one.

**Server Settings:**

| Required | CLI                               | Env                                   | Type        | Description                               | Default       |
| :------: | --------------------------------- | ------------------------------------- | ----------- | ----------------------------------------- | ------------- |
|    ✔️    | `--notifications-email-from`      | `CODER_NOTIFICATIONS_EMAIL_FROM`      | `string`    | The sender's address to use.              |               |
|    ✔️    | `--notifications-email-smarthost` | `CODER_NOTIFICATIONS_EMAIL_SMARTHOST` | `host:port` | The SMTP relay to send messages through.  | localhost:587 |
|    ✔️    | `--notifications-email-hello`     | `CODER_NOTIFICATIONS_EMAIL_HELLO`     | `string`    | The hostname identifying the SMTP server. | localhost     |

**Authentication Settings:**

| Required | CLI                                        | Env                                            | Type     | Description                                                               |
| :------: | ------------------------------------------ | ---------------------------------------------- | -------- | ------------------------------------------------------------------------- |
|    -     | `--notifications-email-auth-username`      | `CODER_NOTIFICATIONS_EMAIL_AUTH_USERNAME`      | `string` | Username to use with PLAIN/LOGIN authentication.                          |
|    -     | `--notifications-email-auth-password`      | `CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD`      | `string` | Password to use with PLAIN/LOGIN authentication.                          |
|    -     | `--notifications-email-auth-password-file` | `CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD_FILE` | `string` | File from which to load password for use with PLAIN/LOGIN authentication. |
|    -     | `--notifications-email-auth-identity`      | `CODER_NOTIFICATIONS_EMAIL_AUTH_IDENTITY`      | `string` | Identity to use with PLAIN authentication.                                |

**TLS Settings:**

| Required | CLI                                       | Env                                         | Type     | Description                                                                                                                                                      | Default |
| :------: | ----------------------------------------- | ------------------------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
|    -     | `--notifications-email-force-tls`         | `CODER_NOTIFICATIONS_EMAIL_FORCE_TLS`       | `bool`   | Force a TLS connection to the configured SMTP smarthost. If port 465 is used, TLS will be forced. See https://datatracker.ietf.org/doc/html/rfc8314#section-3.3. | false   |
|    -     | `--notifications-email-tls-starttls`      | `CODER_NOTIFICATIONS_EMAIL_TLS_STARTTLS`    | `bool`   | Enable STARTTLS to upgrade insecure SMTP connections using TLS. Ignored if `CODER_NOTIFICATIONS_EMAIL_FORCE_TLS` is set.                                         | false   |
|    -     | `--notifications-email-tls-skip-verify`   | `CODER_NOTIFICATIONS_EMAIL_TLS_SKIPVERIFY`  | `bool`   | Skip verification of the target server's certificate (**insecure**).                                                                                             | false   |
|    -     | `--notifications-email-tls-server-name`   | `CODER_NOTIFICATIONS_EMAIL_TLS_SERVERNAME`  | `string` | Server name to verify against the target certificate.                                                                                                            |         |
|    -     | `--notifications-email-tls-cert-file`     | `CODER_NOTIFICATIONS_EMAIL_TLS_CERTFILE`    | `string` | Certificate file to use.                                                                                                                                         |         |
|    -     | `--notifications-email-tls-cert-key-file` | `CODER_NOTIFICATIONS_EMAIL_TLS_CERTKEYFILE` | `string` | Certificate key file to use.                                                                                                                                     |         |

**NOTE:** you _MUST_ use `CODER_NOTIFICATIONS_EMAIL_FORCE_TLS` if your smarthost
supports TLS on a port other than `465`.

### Send emails using G-Suite

After setting the required fields above:

1. Create an [App Password](https://myaccount.google.com/apppasswords) using the
   account you wish to send from
2. Set the following configuration options:
   ```
   CODER_NOTIFICATIONS_EMAIL_SMARTHOST=smtp.gmail.com:465
   CODER_NOTIFICATIONS_EMAIL_AUTH_USERNAME=<user>@<domain>
   CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD="<app password created above>"
   ```

See
[this help article from Google](https://support.google.com/a/answer/176600?hl=en)
for more options.

### Send emails using Outlook.com

After setting the required fields above:

1. Setup an account on Microsoft 365 or outlook.com
2. Set the following configuration options:
   ```
   CODER_NOTIFICATIONS_EMAIL_SMARTHOST=smtp-mail.outlook.com:587
   CODER_NOTIFICATIONS_EMAIL_TLS_STARTTLS=true
   CODER_NOTIFICATIONS_EMAIL_AUTH_USERNAME=<user>@<domain>
   CODER_NOTIFICATIONS_EMAIL_AUTH_PASSWORD="<account password>"
   ```

See
[this help article from Microsoft](https://support.microsoft.com/en-us/office/pop-imap-and-smtp-settings-for-outlook-com-d088b986-291d-42b8-9564-9c414e2aa040)
for more options.

## Webhook

The webhook delivery method sends an HTTP POST request to the defined endpoint.
The purpose of webhook notifications is to enable integrations with other
systems.

**Settings**:

| Required | CLI                                | Env                                    | Type  | Description                             |
| :------: | ---------------------------------- | -------------------------------------- | ----- | --------------------------------------- |
|    ✔️    | `--notifications-webhook-endpoint` | `CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT` | `url` | The endpoint to which to send webhooks. |

Here is an example payload for Coder's webhook notification:

```json
{
	"_version": "1.0",
	"msg_id": "88750cad-77d4-4663-8bc0-f46855f5019b",
	"payload": {
		"_version": "1.0",
		"notification_name": "Workspace Deleted",
		"user_id": "4ac34fcb-8155-44d5-8301-e3cd46e88b35",
		"user_email": "danny@coder.com",
		"user_name": "danny",
		"user_username": "danny",
		"actions": [
			{
				"label": "View workspaces",
				"url": "https://et23ntkhpueak.pit-1.try.coder.app/workspaces"
			},
			{
				"label": "View templates",
				"url": "https://et23ntkhpueak.pit-1.try.coder.app/templates"
			}
		],
		"labels": {
			"initiator": "danny",
			"name": "my-workspace",
			"reason": "initiated by user"
		}
	},
	"title": "Workspace \"my-workspace\" deleted",
	"body": "Hi danny\n\nYour workspace my-workspace was deleted.\nThe specified reason was \"initiated by user (danny)\"."
}
```

The top-level object has these keys:

- `_version`: describes the version of this schema; follows semantic versioning
- `msg_id`: the UUID of the notification (matches the ID in the
  `notification_messages` table)
- `payload`: contains the specific details of the notification; described below
- `title`: the title of the notification message (equivalent to a subject in
  SMTP delivery)
- `body`: the body of the notification message (equivalent to the message body
  in SMTP delivery)

The `payload` object has these keys:

- `_version`: describes the version of this inner schema; follows semantic
  versioning
- `notification_name`: name of the event which triggered the notification
- `user_id`: Coder internal user identifier of the target user (UUID)
- `user_email`: email address of the target user
- `user_name`: name of the target user
- `user_username`: username of the target user
- `actions`: a list of CTAs (Call-To-Action); these are mainly relevant for SMTP
  delivery in which they're shown as buttons
- `labels`: dynamic map of zero or more string key-value pairs; these vary from
  event to event

## User Preferences

All users have the option to opt-out of any notifications. Go to **Account** ->
**Notifications** to turn notifications on or off. The delivery method for each
notification is indicated on the right hand side of this table.

![User Notification Preferences](../../../images/admin/monitoring/notifications/user-notification-preferences.png)

## Delivery Preferences (enterprise) (premium)

Administrators can configure which delivery methods are used for each different
[event type](#event-types).

![preferences](../../../images/admin/monitoring/notifications/notification-admin-prefs.png)

You can find this page under
`https://$CODER_ACCESS_URL/deployment/notifications?tab=events`.

## Stop sending notifications

Administrators may wish to stop _all_ notifications across the deployment. We
support a killswitch in the CLI for these cases.

To pause sending notifications, execute
[`coder notifications pause`](../../../reference/cli/notifications_pause.md).

To resume sending notifications, execute
[`coder notifications resume`](../../../reference/cli/notifications_resume.md).

## Troubleshooting

If notifications are not being delivered, use the following methods to
troubleshoot:

1. Ensure notifications are being added to the `notification_messages` table
2. Review any error messages in the `status_reason` column, should an error have
   occurred
3. Review the logs (search for the term `notifications`) for diagnostic
   information<br> _If you do not see any relevant logs, set
   `CODER_VERBOSE=true` or `--verbose` to output debug logs_

## Internals

The notification system is built to operate concurrently in a single- or
multi-replica Coder deployment, and has a built-in retry mechanism. It uses the
configured Postgres database to store notifications in a queue and facilitate
concurrency.

All messages are stored in the `notification_messages` table.

Messages older than 7 days are deleted.

### Message States

![states](../../../images/admin/monitoring/notifications/notification-states.png)

_A notifier here refers to a Coder replica which is responsible for dispatching
the notification. All running replicas act as notifiers to process pending
messages._

- a message begins in `pending` state
- transitions to `leased` when a Coder replica acquires new messages from the
  database
  - new messages are checked for every `CODER_NOTIFICATIONS_FETCH_INTERVAL`
    (default: 15s)
- if a message is delivered successfully, it transitions to `sent` state
- if a message encounters a non-retryable error (e.g. misconfiguration), it
  transitions to `permanent_failure`
- if a message encounters a retryable error (e.g. temporary server outage), it
  transitions to `temporary_failure`
  - this message will be retried up to `CODER_NOTIFICATIONS_MAX_SEND_ATTEMPTS`
    (default: 5)
  - this message will transition back to `pending` state after
    `CODER_NOTIFICATIONS_RETRY_INTERVAL` (default: 5m) and be retried
  - after `CODER_NOTIFICATIONS_MAX_SEND_ATTEMPTS` is exceeded, it transitions to
    `permanent_failure`

See [Troubleshooting](#troubleshooting) above for more details.
