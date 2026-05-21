# Chat Sharing

Chat sharing lets you give other users or groups read-only access to a Coder Agents conversation.

## Share a chat

1. Open the root chat you want to share on the **Agents** page.
1. Click the share icon in the chat top bar.
1. Click the **Search for user or group** field.
1. Search for and select a user or group.
1. Click **Add member** to grant **Read** access.
1. Copy the chat URL from your browser and send it to the users or groups you shared with.

Coder does not create a separate share link or notify recipients. Shared users and groups must open the chat from the URL you send them.

Sub-agent child chats inherit sharing from the root chat and cannot be shared separately.

## Shared chat access

Shared users can open the chat from a direct link, view messages, stream live updates, and download chat attachments.

Shared chats do not appear in the recipient's normal chat list. Read-only users cannot continue the chat, edit messages, archive it, regenerate its title, or change sharing.

## Disable chat sharing

Administrators can disable chat sharing for a deployment with `--disable-chat-sharing`, `CODER_DISABLE_CHAT_SHARING`, or `disableChatSharing`. When disabled, only chat owners can access their chats.
