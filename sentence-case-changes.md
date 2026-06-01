# Sentence Case Changes - Coder Agents UI

PR: https://github.com/coder/coder/pull/25941

## Settings headings

| Before | After |
|---|---|
| Personal Instructions | Personal instructions |
| Chat Layout | Chat layout |
| Keyboard Shortcuts | Keyboard shortcuts |
| Thinking Display | Thinking display |
| Shell Output Display | Shell output display |
| Code Diff Display | Code diff display |
| Autostop Fallback | Autostop fallback |

## Select/option labels

| Before | After |
|---|---|
| Always Expanded | Always expanded |
| Always Collapsed | Always collapsed |

## Sidebar and nav labels

| Before | After |
|---|---|
| New Agent | New chat |
| Personal Skills | Personal skills |
| Manage Agents | Manage agents |

## Admin/limits labels

| Before | After |
|---|---|
| Group Limits | Group limits |
| Per-User Overrides | Per-user overrides |
| Default Spend Limit | Default spend limit |

## Other

| Before | After |
|---|---|
| Weekly/Workspace Usage | Weekly/Workspace usage |
| View Usage | View usage |

## Files changed (20)

### Components

- `components/PersonalInstructionsSettings.tsx`
- `components/ChatFullWidthSettings.tsx`
- `components/ChatSendShortcutSettings.tsx`
- `components/DisplayModeSettings.tsx`
- `components/UsageIndicator.tsx`
- `components/WorkspaceAutostopSettings.tsx`
- `components/AgentCreateForm.tsx`
- `components/ChatConversation/LiveStreamTail.tsx`
- `components/ChatsSidebar/chats/ChatsPanel.tsx`
- `components/ChatsSidebar/settings/SettingsPanel.tsx`
- `components/LimitsTab/DefaultLimitSection.tsx`
- `components/LimitsTab/GroupLimitsSection.tsx`
- `components/LimitsTab/UserOverridesSection.tsx`
- `AgentSettingsPage.tsx`
- `AgentSettingsPersonalSkillsPageView.tsx`

### Stories/tests

- `AgentSettingsGeneralPageView.stories.tsx`
- `AgentSettingsLifecyclePageView.stories.tsx`
- `AgentsPageView.stories.tsx`
- `components/ChatsSidebar/ChatsSidebar.stories.tsx`
- `components/ChatConversation/LiveStreamTail.stories.tsx`

All paths relative to `site/src/pages/AgentsPage/`.
