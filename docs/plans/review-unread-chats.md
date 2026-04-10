# Plan: "Review Unread Chats" Feature for /agents

## Problem
When juggling 3+ concurrent agent chats with unread notifications, it's hard to keep track of which chats need attention. The existing unread dots in the sidebar help, but there's no efficient way to triage through all of them sequentially.

## Solution Overview
Add a **"Review Unread Chats"** button to the sidebar and a **review popup** that lets you step through unread chats one-by-one with Previous/Next navigation, marking each as read as you go.

---

## Architecture Summary (Existing)

| Layer | File | Responsibility |
|---|---|---|
| Container | `AgentsPage.tsx` | Data fetching, WebSocket events, `has_unread` management |
| View | `AgentsPageView.tsx` | Layout, passes data to sidebar + Outlet |
| Sidebar | `AgentsSidebar.tsx` | Renders chat list, unread dot indicator |
| Chat page | `AgentChatPage.tsx` | Individual chat streaming, messages, store |
| Chat UI | `ChatPageContent.tsx` | Chat conversation + input |
| Types | `typesGenerated.ts` | `Chat.has_unread: boolean` |

**Existing unread flow:**
- WebSocket `status_change` event → `has_unread: true` (optimistic, for non-active chats)
- Navigate to chat → `has_unread: false` (optimistic cache update)
- Sidebar renders a `2×2 bg-content-link` blue dot when `chat.has_unread && !isActiveChat`

**Existing design patterns to reuse:**
- Dialog: Radix-based `Dialog`/`DialogContent` from `#/components/Dialog/Dialog`
- Warning border (editing state): `shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]`
- Theme vars: `--border-warning` (orange), `--surface-orange`, `--content-warning`
- Button component: `#/components/Button/Button`
- Badge component: `#/components/Badge/Badge`
- Chat store: Zustand-like `useChatStore()` + `useChatSelector()`

---

## Implementation Plan

### Step 1: Create the `useUnreadChats` hook

**File:** `site/src/pages/AgentsPage/hooks/useUnreadChats.ts`

**Purpose:** Derive the list of unread chats from the existing `chatList` prop.

```ts
// Filters chatList to only chats where has_unread === true
// Returns: { unreadChats: Chat[], unreadCount: number, hasReviewThreshold: boolean }
// hasReviewThreshold is true when unreadCount >= 3 (the trigger)
```

**Why a hook:** This logic is used in two places (the sidebar button and the review dialog), so extracting it avoids duplication. It's a pure derivation from `chatList` with `useMemo`.

---

### Step 2: Create the `ReviewUnreadButton` component

**File:** `site/src/pages/AgentsPage/components/Sidebar/ReviewUnreadButton.tsx`

**Purpose:** The sidebar button that appears below "New Agent" when ≥3 chats have unread notifications.

**Behavior:**
- **Hidden** when fewer than 3 unread chats exist
- **Visible** with animated entry when ≥3 unread chats exist
- Uses the warning/editing border style: `shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]`
- Displays an unread count badge in the upper-right corner using the `Badge` component (or a custom styled `<span>`) with `bg-content-warning` styling
- Clicking opens the review dialog

**Visual spec:**
```
┌──────────────────────────────────────┐
│ ✏️  New Agent                        │ ← existing SettingsNavItem
├──────────────────────────────────────┤
│ ┌─ orange border ─────────────── ⁵ ┐ │ ← new ReviewUnreadButton
│ │  📋  Review unread chats          │ │    with unread count badge
│ └────────────────────────────────── ┘ │
├──────────────────────────────────────┤
│ (chat list below)                    │
└──────────────────────────────────────┘
```

**Styling (Tailwind):**
```tsx
<button className={cn(
  "relative flex w-full items-center gap-2.5 rounded-md px-2.5 py-2",
  "text-left text-sm cursor-pointer transition-all",
  "bg-surface-orange/30 text-content-warning",
  "shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
  "hover:bg-surface-orange/50",
)}>
  <ListChecksIcon className="h-4 w-4 shrink-0" />
  <span>Review unread chats</span>
  {/* Badge in upper-right */}
  <span className="absolute -top-1.5 -right-1.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-content-warning px-1 text-xs font-bold text-white">
    {unreadCount}
  </span>
</button>
```

---

### Step 3: Create the `ReviewUnreadDialog` component

**File:** `site/src/pages/AgentsPage/components/ReviewUnreadDialog.tsx`

**Purpose:** Full-screen (or large) dialog for stepping through unread chats.

**Props:**
```ts
interface ReviewUnreadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  unreadChats: Chat[];
  onChatReviewed: (chatId: string) => void; // callback to mark as read
}
```

**Layout:**
```
┌─────────────────────────────────────────────────────┐
│  TOP BANNER                                         │
│  Chat title: "Fix auth bug in login flow"           │
│  Original message: "Please fix the authentication..." │
│  ─────────────────────────────────────────────────── │
│  SUMMARY                                            │
│  "Agent completed 3 file edits, ran tests, and is   │
│   waiting for your review of the proposed changes."  │
├─────────────────────────────────────────────────────┤
│                                                     │
│  CHAT INTERFACE                                     │
│  (embedded ChatPageContent — the regular chat UI)   │
│  Messages, tools, streaming output, input box...    │
│                                                     │
│                                                     │
├─────────────────────────────────────────────────────┤
│  ◀ Previous          (2 of 5)          Next ▶       │
│  BOTTOM NAVIGATION BANNER                           │
└─────────────────────────────────────────────────────┘
```

**Key sub-components within this file:**

#### 3a. `ReviewBanner` (top section)
- Displays `chat.title` prominently
- Shows the first user message (the "original message" that kicked off the chat) — fetched from the chat messages query
- Renders a short summary of chat activity (see Step 4)

#### 3b. Embedded Chat Interface
- Renders the same `ChatPageContent` used by `AgentChatPageView`, but within the dialog context
- This gives the user the full chat experience: see messages, send follow-ups, see tool outputs
- The chat data is fetched using the same `useQuery(chat(chatId))` + `useInfiniteQuery(chatMessagesForInfiniteScroll(chatId))` pattern from `AgentChatPage.tsx`

#### 3c. `ReviewNavigationBar` (bottom section)
- **Previous button**: Disabled on the first chat. Navigates to the previous unread chat in the queue.
- **Position indicator**: "2 of 5" centered text
- **Next button**: Moves to the next unread chat. On the last chat, label changes to "Done" and closes the dialog.
- Each navigation action marks the current chat as read (calls `onChatReviewed`)

**State management:**
```ts
const [currentIndex, setCurrentIndex] = useState(0);
const currentChat = unreadChats[currentIndex];

// When navigating away from a chat, mark it as read
const handleNext = () => {
  onChatReviewed(currentChat.id);
  if (currentIndex >= unreadChats.length - 1) {
    onOpenChange(false); // close dialog — all reviewed
  } else {
    setCurrentIndex(prev => prev + 1);
  }
};

const handlePrevious = () => {
  if (currentIndex > 0) {
    setCurrentIndex(prev => prev - 1);
  }
};
```

**Dialog sizing:**
- Use the Radix Dialog with custom large sizing: `max-w-5xl h-[85vh]` — or `max-w-[90vw] h-[90vh]` for near-fullscreen
- Override the default `DialogContent` max-width via className

---

### Step 4: Create the `useChatSummary` hook

**File:** `site/src/pages/AgentsPage/hooks/useChatSummary.ts`

**Purpose:** Generate a short summary of what happened in a chat since the user last looked.

**Approach — client-side heuristic (no AI call):**
Derive the summary from the chat's message history and status. This avoids an extra API call and works instantly:

```ts
function deriveChatSummary(chat: Chat, messages: ChatMessage[]): string {
  const parts: string[] = [];
  
  // Count assistant actions
  const toolCalls = messages.filter(m => m.role === "tool").length;
  const assistantMessages = messages.filter(m => m.role === "assistant").length;
  
  // Status-based summary
  switch (chat.status) {
    case "completed":
      parts.push("Agent finished its work.");
      break;
    case "waiting":
      parts.push("Agent is waiting for your input.");
      break;
    case "error":
      parts.push(`Agent encountered an error: ${chat.last_error ?? "unknown"}`);
      break;
    case "running":
      parts.push("Agent is still working.");
      break;
    // ... etc
  }
  
  if (toolCalls > 0) parts.push(`Used ${toolCalls} tool(s).`);
  
  // Diff status
  if (chat.diff_status) {
    parts.push("Has pending code changes to review.");
  }
  
  return parts.join(" ");
}
```

This can be enhanced later to use an LLM summarization endpoint if desired, but the heuristic is fast and good enough for v1.

---

### Step 5: Wire into `AgentsPage.tsx` (container)

**Changes to `AgentsPage.tsx`:**

1. Add `reviewDialogOpen` state:
   ```ts
   const [reviewDialogOpen, setReviewDialogOpen] = useState(false);
   ```

2. Add a `markChatAsRead` callback that optimistically updates the cache:
   ```ts
   const markChatAsRead = useCallback((chatId: string) => {
     updateInfiniteChatsCache(queryClient, (chats) =>
       chats.map(c => c.id === chatId ? { ...c, has_unread: false } : c)
     );
   }, [queryClient]);
   ```
   This mirrors the existing optimistic-clear pattern at lines 455–473.

3. Pass new props down to `AgentsPageView`:
   ```ts
   reviewDialogOpen={reviewDialogOpen}
   onOpenReviewDialog={() => setReviewDialogOpen(true)}
   onCloseReviewDialog={() => setReviewDialogOpen(false)}
   onChatReviewed={markChatAsRead}
   ```

---

### Step 6: Wire into `AgentsPageView.tsx` (view)

**Changes to `AgentsPageView.tsx`:**

1. Accept and forward the new props
2. Pass `onOpenReviewDialog` to `AgentsSidebar` (for the button)
3. Render `ReviewUnreadDialog` at the view level (sibling to the layout)

```tsx
<div className="flex h-full">
  <AgentsSidebar
    {...sidebarProps}
    onOpenReviewDialog={onOpenReviewDialog}
  />
  <Outlet context={outletContext} />
</div>

<ReviewUnreadDialog
  open={reviewDialogOpen}
  onOpenChange={onCloseReviewDialog}
  unreadChats={unreadChats}
  onChatReviewed={onChatReviewed}
/>
```

---

### Step 7: Wire into `AgentsSidebar.tsx` (sidebar)

**Changes to `AgentsSidebar.tsx`:**

1. Accept `onOpenReviewDialog` prop
2. Insert `ReviewUnreadButton` right after the "New Agent" `SettingsNavItem` (around line 1034):

```tsx
{/* Existing "New Agent" button */}
<SettingsNavItem
  icon={SquarePenIcon}
  label="New Agent"
  active={!activeChatId && sidebarView.panel === "chats"}
  to="/agents"
  onClick={onBeforeNewAgent}
  disabled={isCreating}
/>

{/* NEW: Review Unread button */}
<ReviewUnreadButton
  chatList={chatList}
  onClick={onOpenReviewDialog}
/>
```

---

### Step 8: Add Storybook stories

**Files:**
- `site/src/pages/AgentsPage/components/Sidebar/ReviewUnreadButton.stories.tsx`
- `site/src/pages/AgentsPage/components/ReviewUnreadDialog.stories.tsx`
- Update `AgentsPageView.stories.tsx` and `AgentsSidebar.stories.tsx` with new stories

**Stories to add:**

| Story | What it tests |
|---|---|
| `ReviewUnreadButton/Hidden` | < 3 unread → button not visible |
| `ReviewUnreadButton/Visible` | ≥ 3 unread → button visible with orange border and count badge |
| `ReviewUnreadButton/HighCount` | 10+ unread → badge shows double-digit count |
| `ReviewUnreadDialog/Default` | Dialog open with 3 unread chats, first chat shown |
| `ReviewUnreadDialog/LastChat` | On last chat, "Next" becomes "Done" |
| `ReviewUnreadDialog/FirstChat` | Previous button disabled |
| `ReviewUnreadDialog/SingleChat` | Edge case: only 1 chat in review (if threshold changed) |
| `AgentsSidebar/WithReviewButton` | Integration: sidebar with ≥3 unread showing the review button |

---

### Step 9: Add tests

**Files:**
- `site/src/pages/AgentsPage/hooks/useUnreadChats.test.ts`
- `site/src/pages/AgentsPage/components/ReviewUnreadDialog.test.tsx`
- Update `AgentsSidebar.test.tsx`

**Test cases:**

| Test | Assertion |
|---|---|
| `useUnreadChats` returns correct count | Filter logic, threshold check |
| Review button hidden below threshold | Button not in DOM when < 3 unread |
| Review button visible at threshold | Button in DOM when ≥ 3 unread |
| Badge shows correct count | `textContent` matches unread count |
| Dialog opens on button click | Dialog visible after click |
| Next navigates to next chat | Title changes, index increments |
| Previous disabled on first | Button has `disabled` attribute |
| Last chat Next closes dialog | Dialog closes, all chats marked read |
| Chat marked read on navigation | `onChatReviewed` called with correct ID |

---

## File Change Summary

| File | Change Type | Description |
|---|---|---|
| `hooks/useUnreadChats.ts` | **New** | Hook: derive unread chats list + threshold |
| `hooks/useUnreadChats.test.ts` | **New** | Tests for the hook |
| `hooks/useChatSummary.ts` | **New** | Hook: generate client-side chat summary |
| `components/Sidebar/ReviewUnreadButton.tsx` | **New** | Sidebar button with orange border + badge |
| `components/Sidebar/ReviewUnreadButton.stories.tsx` | **New** | Storybook stories |
| `components/ReviewUnreadDialog.tsx` | **New** | Full review dialog with navigation |
| `components/ReviewUnreadDialog.stories.tsx` | **New** | Storybook stories |
| `components/ReviewUnreadDialog.test.tsx` | **New** | Tests for the dialog |
| `AgentsPage.tsx` | **Modified** | Add state, mark-read callback, pass new props |
| `AgentsPageView.tsx` | **Modified** | Accept new props, render dialog, forward to sidebar |
| `components/Sidebar/AgentsSidebar.tsx` | **Modified** | Accept `onOpenReviewDialog`, render `ReviewUnreadButton` |
| `AgentsPageView.stories.tsx` | **Modified** | Add stories with review button visible |
| `components/Sidebar/AgentsSidebar.stories.tsx` | **Modified** | Add `WithReviewButton` story |

---

## Risks & Open Questions

1. **Embedded chat in dialog**: The biggest complexity is rendering `ChatPageContent` inside a dialog rather than as a routed page. The chat store, WebSocket connections, and scroll containers are currently tied to the routed `AgentChatPage`. We may need to extract the data-fetching logic from `AgentChatPage.tsx` into a reusable hook (`useChatPageData`) so the dialog can instantiate its own chat session without URL routing.

2. **Summary quality**: The client-side heuristic summary (Step 4) is fast but shallow. A future iteration could call an LLM endpoint to generate a richer summary. For v1, the heuristic is sufficient.

3. **Queue stability**: If a new chat becomes unread while the dialog is open, should it be added to the queue? **Recommendation:** Snapshot the unread list when the dialog opens. New unreads don't enter the current review session — they'll appear on the next open.

4. **Threshold configurability**: The ≥3 threshold is hardcoded. Could be made a user preference later, but hardcoded is fine for v1.

5. **Mobile responsiveness**: The dialog should be responsive. On mobile, it may need to be full-screen instead of 85vh.

## Suggested Implementation Order

1. `useUnreadChats` hook (pure logic, easy to test)
2. `ReviewUnreadButton` (visual only, easy to verify in Storybook)
3. Wire button into sidebar (see it in context)
4. `ReviewUnreadDialog` shell (layout + navigation, no chat embed yet)
5. `useChatSummary` hook
6. Embed `ChatPageContent` into the dialog (hardest part)
7. Wire dialog state into `AgentsPage.tsx` / `AgentsPageView.tsx`
8. Stories + tests
9. Polish (animations, edge cases, mobile)
