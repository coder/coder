<!-- DO NOT EDIT | GENERATED CONTENT -->
# notifications

Manage Coder notifications

Aliases:

* notification

## Usage

```console
coder notifications
```

## Description

```console
Administrators can use these commands to change notification settings.
  - Pause Coder notifications. Administrators can temporarily stop notifiers from
dispatching messages in case of the target outage (for example: unavailable SMTP
server or Webhook not responding).:

     $ coder notifications pause

  - Resume Coder notifications:

     $ coder notifications resume

  - Send a test notification. Administrators can use this to verify the notification
target settings.:

     $ coder notifications test
```

## Subcommands

| Name                                             | Purpose                  |
|--------------------------------------------------|--------------------------|
| [<code>pause</code>](./notifications_pause.md)   | Pause notifications      |
| [<code>resume</code>](./notifications_resume.md) | Resume notifications     |
| [<code>test</code>](./notifications_test.md)     | Send a test notification |
