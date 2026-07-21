# Chat Sharing

Chat sharing lets you give other users or groups read-only access to a Coder Agents conversation.

## Share a chat

1. Open the chat you want to share on the **Agents** page. Only top-level chats can be shared; sub-agent chats inherit sharing from their parent.
1. Click the share icon in the chat top bar.
1. Click the **Search for user or group** field.
1. Search for and select a user or group.
1. Click **Add member** to grant **Read** access.
1. If you shared with a group, copy the chat URL from your browser and send it to the group members.

Coder does not create a separate share link. Users you share with directly receive a **Chat Shared** notification with a link to open the chat. Members who gain access only through a group are not notified.

## Shared chat access

Viewers can open the chat from a direct link, view messages, stream live updates, and download chat attachments. Chats shared by other users can appear in the sidebar under **Shared with you** when they are in the chat list. Pinned shared chats appear under **Pinned**. Viewers reach sub-agent chats by following sub-agent links inside the parent chat or by opening a direct URL.

Viewers have read-only access: they cannot send or edit messages, regenerate the chat title, archive the chat, or change its sharing settings.

## Disable chat sharing

Administrators can disable chat sharing for a deployment with `--disable-chat-sharing`, `CODER_DISABLE_CHAT_SHARING`, or `disableChatSharing`. When disabled, only chat owners can access their chats.
