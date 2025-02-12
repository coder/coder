# Slack Notifications

[Slack](https://slack.com/) is a popular messaging platform designed for teams
and businesses, enabling real-time collaboration through channels, direct
messages, and integrations with external tools. With Coder's integration, you
can enable automated notifications directly within a self-hosted
[Slack app](https://api.slack.com/apps), keeping your team updated on key events
in your Coder environment.

Administrators can configure Coder to send notifications via an incoming webhook
endpoint. These notifications will be delivered as Slack messages direct to the
user. Routing is based on the user's email address, and this should be
consistent between Slack and their Coder login.

## Requirements

Before setting up Slack notifications, ensure that you have the following:

- Administrator access to the Slack platform to create apps
- Coder platform >=v2.16.0

## Create Slack Application

To integrate Slack with Coder, follow these steps to create a Slack application:

1. Go to the [Slack Apps](https://api.slack.com/apps) dashboard and create a new
   Slack App.

2. Under "Basic Information," you'll find a "Signing Secret." The Slack
   application uses it to
   [verify requests](https://api.slack.com/authentication/verifying-requests-from-slack)
   coming from Slack.

3. Under "OAuth & Permissions", add the following OAuth scopes:

   - `chat:write`: To send messages as the app.
   - `users:read`: To find the user details.
   - `users:read.email`: To find user emails.

4. Install the app to your workspace and note down the **Bot User OAuth Token**
   from the "OAuth & Permissions" section.

## Build a Webserver to Receive Webhooks

The Slack bot for Coder runs as a _Bolt application_, which is a framework
designed for building Slack apps using the Slack API.
[Bolt for JavaScript](https://github.com/slackapi/bolt-js) provides an
easy-to-use API for responding to events, commands, and interactions from Slack.

To build the server to receive webhooks and interact with Slack:

1. Initialize your project by running:

   ```bash
   npm init -y
   ```

2. Install the Bolt library:

   ```bash
   npm install @slack/bolt
   ```

3. Create and edit the `app.js` file. Below is an example of the basic
   structure:

   ```js
   const { App, LogLevel, ExpressReceiver } = require("@slack/bolt");
   const bodyParser = require("body-parser");

   const port = process.env.PORT || 6000;

   // Create a Bolt Receiver
   const receiver = new ExpressReceiver({
       signingSecret: process.env.SLACK_SIGNING_SECRET,
   });
   receiver.router.use(bodyParser.json());

   // Create the Bolt App, using the receiver
   const app = new App({
       token: process.env.SLACK_BOT_TOKEN,
       logLevel: LogLevel.DEBUG,
       receiver,
   });

   receiver.router.post("/v1/webhook", async (req, res) => {
       try {
           if (!req.body) {
               return res.status(400).send("Error: request body is missing");
           }

           const { title, body } = req.body;
           if (!title || !body) {
               return res
                   .status(400)
                   .send('Error: missing fields: "title", or "body"');
           }

           const payload = req.body.payload;
           if (!payload) {
               return res.status(400).send('Error: missing "payload" field');
           }

           const { user_email, actions } = payload;
           if (!user_email || !actions) {
               return res
                   .status(400)
                   .send('Error: missing fields: "user_email", "actions"');
           }

           // Get the user ID using Slack API
           const userByEmail = await app.client.users.lookupByEmail({
               email: user_email,
           });

           const slackMessage = {
               channel: userByEmail.user.id,
               text: body,
               blocks: [
                   {
                       type: "header",
                       text: { type: "plain_text", text: title },
                   },
                   {
                       type: "section",
                       text: { type: "mrkdwn", text: body },
                   },
               ],
           };

           // Add action buttons if they exist
           if (actions && actions.length > 0) {
               slackMessage.blocks.push({
                   type: "actions",
                   elements: actions.map((action) => ({
                       type: "button",
                       text: { type: "plain_text", text: action.label },
                       url: action.url,
                   })),
               });
           }

           // Post message to the user on Slack
           await app.client.chat.postMessage(slackMessage);

           res.status(204).send();
       } catch (error) {
           console.error("Error sending message:", error);
           res.status(500).send();
       }
   });

   // Acknowledge clicks on link_button, otherwise Slack UI
   // complains about missing events.
   app.action("button_click", async ({ body, ack, say }) => {
       await ack(); // no specific action needed
   });

   // Start the Bolt app
   (async () => {
       await app.start(port);
       console.log("⚡️ Coder Slack bot is running!");
   })();
   ```

4. Set environment variables to identify the Slack app:

   ```bash
   export SLACK_BOT_TOKEN=xoxb-...
   export SLACK_SIGNING_SECRET=0da4b...
   ```

5. Start the web application by running:

   ```bash
   node app.js
   ```

## Enable Interactivity in Slack

Slack requires the bot to acknowledge when a user clicks on a URL action button.
This is handled by setting up interactivity.

1. Under "Interactivity & Shortcuts" in your Slack app settings, set the Request
   URL to match the public URL of your web server's endpoint.

> Notice: You can use any public endpoint that accepts and responds to POST
> requests with HTTP 200. For temporary testing, you can set it to
> `https://httpbin.org/status/200`.

Once this is set, Slack will send interaction payloads to your server, which
must respond appropriately.

## Enable Webhook Integration in Coder

To enable webhook integration in Coder, define the POST webhook endpoint
matching the deployed Slack bot:

```bash
export CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT=http://localhost:6000/v1/webhook`
```

Finally, go to the **Notification Settings** in Coder and switch the notifier to
**Webhook**.
