# Microsoft Teams Notifications

[Microsoft Teams](https://www.microsoft.com/en-us/microsoft-teams) is a widely
used collaboration platform, and with Coder's integration, you can enable
automated notifications directly within Teams using workflows and
[Adaptive Cards](https://adaptivecards.io/)

Administrators can configure Coder to send notifications via an incoming webhook
endpoint. These notifications appear as messages in Teams chats, either with the
Flow Bot or a specified user/service account.

## Requirements

Before setting up Microsoft Teams notifications, ensure that you have the
following:

- Administrator access to the Teams platform
- Coder platform >=v2.16.0

## Build Teams Workflow

The process of setting up a Teams workflow consists of three key steps:

1. Configure the Webhook Trigger.

   Begin by configuring the trigger: **"When a Teams webhook request is
   received"**.

   Ensure the trigger access level is set to **"Anyone"**.

1. Setup the JSON Parsing Action.

   Add the **"Parse JSON"** action, linking the content to the **"Body"** of the
   received webhook request. Use the following schema to parse the notification
   payload:

   ```json
   {
       "type": "object",
       "properties": {
           "_version": {
               "type": "string"
           },
           "payload": {
               "type": "object",
               "properties": {
                   "_version": {
                       "type": "string"
                   },
                   "user_email": {
                       "type": "string"
                   },
                   "actions": {
                       "type": "array",
                       "items": {
                           "type": "object",
                           "properties": {
                               "label": {
                                   "type": "string"
                               },
                               "url": {
                                   "type": "string"
                               }
                           },
                           "required": ["label", "url"]
                       }
                   }
               }
           },
           "title": {
               "type": "string"
           },
           "body": {
               "type": "string"
           }
       }
   }
   ```

   This action parses the notification's title, body, and the recipient's email
   address.

1. Configure the Adaptive Card Action.

   Finally, set up the **"Post Adaptive Card in a chat or channel"** action with
   the following recommended settings:

   **Post as**: Flow Bot

   **Post in**: Chat with Flow Bot

   **Recipient**: `user_email`

   Use the following _Adaptive Card_ template:

   ```json
   {
       "$schema": "https://adaptivecards.io/schemas/adaptive-card.json",
       "type": "AdaptiveCard",
       "version": "1.0",
       "body": [
           {
               "type": "Image",
               "url": "https://coder.com/coder-logo-horizontal.png",
               "height": "40px",
               "altText": "Coder",
               "horizontalAlignment": "center"
           },
           {
               "type": "TextBlock",
               "text": "**@{replace(body('Parse_JSON')?['title'], '"', '\"')}**"
           },
           {
               "type": "TextBlock",
               "text": "@{replace(body('Parse_JSON')?['body'], '"', '\"')}",
               "wrap": true
           },
           {
               "type": "ActionSet",
               "actions": [@{replace(replace(join(body('Parse_JSON')?['payload']?['actions'], ','), '{', '{"type": "Action.OpenUrl",'), '"label"', '"title"')}]
           }
       ]
   }
   ```

   _Notice_: The Coder `actions` format differs from the `ActionSet` schema, so
   its properties need to be modified: include `Action.OpenUrl` type, rename
   `label` to `title`. Unfortunately, there is no straightforward solution for
   `for-each` pattern.

   Feel free to customize the payload to modify the logo, notification title, or
   body content to suit your needs.

## Enable Webhook Integration

To enable webhook integration in Coder, define the POST webhook endpoint created
by your Teams workflow:

```bash
export CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT=https://prod-16.eastus.logic.azure.com:443/workflows/f8fbe3e8211e4b638...`
```

Finally, go to the **Notification Settings** in Coder and switch the notifier to
**Webhook**.

## Limitations

1. **Public Webhook Trigger**: The Teams webhook trigger must be open to the
   public (**"Anyone"** can send the payload). It's recommended to keep the
   endpoint secret and apply additional authorization layers to protect against
   unauthorized access.

2. **Markdown Support in Adaptive Cards**: Note that Adaptive Cards support a
   [limited set of Markdown tags](https://learn.microsoft.com/en-us/microsoftteams/platform/task-modules-and-cards/cards/cards-format?tabs=adaptive-md%2Cdesktop%2Cconnector-html).
