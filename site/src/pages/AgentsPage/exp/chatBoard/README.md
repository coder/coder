# Chat board (experiment)

A board view for Coder Agents chats: columns for stages, cards for topics,
notes for what the human knows that the agents do not. Off by default;
enable it under Agents settings, General, Experiments. Everything lives in
this folder; production touches it only at a few flag-gated hook points
(router, settings page, layout route check, chat list slot, `AgentChatPage`
`chatId` prop).

## Intent

Someone running many agent chats at once loses track of what each chat is
for, where it stands, and how chats relate. The sidebar is a flat list of
titles and statuses. Before this experiment the workaround was a hand kept
Markdown file: links to chats, one line of the human's own reading of the
status, notes on purpose and on chats that changed direction.

The board is that file as a UI. Its one job is to let a human administer
their own context across chats quickly: sort chats into stages, group the
ones that belong to one piece of work, write down understanding as it forms,
and read or steer a chat without leaving the overview. It is a human's
context administration board, so every action on it must be cheap.

Design rules that came out of building it:

- Durable state lives in chat labels only (prefix `board/`), so nothing needs
  backend support and other tools can tell these labels apart. Browser-local
  state (column order, window geometry, the flag) may be lost.
- Grouping must never hide a chat, in the board or in the sidebar.
- Cards stay where the user put them; chat activity never reorders them.
- One editing language everywhere: click the text to edit it in place, no
  pencils, nothing important behind a menu.
- Controls that appear on hover must not move anything else.
- No wall of live mini chat windows. Chats open on demand, beside the board.
- Prototype scope: no mobile layout, no pagination edge cases, no undo
  history beyond the last regrouping or note move.

## User stories

Stages

- I can create, rename, reorder and delete columns. A default "Inbox" column
  always exists and holds chats nobody has sorted yet.
- I can drag a card between columns and to an exact position within a
  column, and it stays there.

Cards and groups

- A card is one chat, or several chats grouped under a topic title. Every
  grouped chat stays visible on the card with its own title, status, age,
  unread mark and pull request information.
- I can drop a chat onto a card to group them, and take any chat out again,
  including the one that carries the card data. Merging never loses a title
  or notes; whatever would vanish is kept as a note.
- I can rename the card and any chat on it by clicking its text. Titles wrap
  to two lines and then truncate.
- I can give a card a color, shown as a stripe and a light wash behind the
  title, to tell cards apart at a glance.
- After an accidental drop I can undo the regrouping from the confirmation.

Notes

- I can write Markdown notes on a card, in order, edit them in place and
  delete them. The composer is always there at the bottom of the card.
- Notes carry the human's reading of the work. When cards merge or a chat
  leaves a group, the notes follow the card.

Status at a glance

- Each chat shows its status icon, last turn, age, unread mark and linked
  pull request with line counts.
- Resting on the info icon shows the chat's summary and cost without
  reflowing the card; clicking pins it.

Working without leaving the board

- Resting on a chat's icon previews the full chat in a floating window
  beside the card; clicking it, or dragging it, keeps the window. Windows
  move, resize, stack and survive a reload.
- I can filter the board with the same search the sidebar uses; whole cards
  stay or go, groups are never split by a filter.

Assistant

- Each card can have an assistant chat. It is created once, hidden from the
  board, given the card's title, notes and chats as a starting snapshot, and
  told to verify against the live chats before answering. It uses a shared
  workspace so it can read transcripts, send follow-ups and check pull
  requests on my behalf when I ask.
- The board has one assistant of its own, opened from the header. It gets
  a snapshot of every card with its primary chat id and is told how to read
  and edit board labels; it proposes changes and acts only on a yes. When it
  finishes a turn the board refetches the chat list.

Sidebar

- Grouped chats appear together in the regular chat list inside one frame,
  with the card title and column shown once. Other chats show their column
  as a small tag. Nothing is ever collapsed away by grouping.

## Data model

| Label                       | On        | Meaning                                    |
| --------------------------- | --------- | ------------------------------------------ |
| `board/column`              | members   | column name; absent means Inbox            |
| `board/group`               | members   | id of the chat that carries the card data  |
| `board/title`               | primary   | topic title; absent means the chat's title |
| `board/color`               | primary   | one of the theme accent names              |
| `board/pos`                 | primary   | placement key; higher sorts first          |
| `board/comment.N.timestamp` | primary   | note N, Unix milliseconds                  |
| `board/comment.N.M`         | primary   | note N, chunk M (256 byte label limit)     |
| `board/assistant`           | assistant | id of the card, or `board`; not a card     |

Writes replace the whole label map of a chat. Regrouping and note moves
snapshot the previous maps of every touched chat so they can be undone.

## Code layout

- `boardLabels.ts` is the codec: label keys, parsing and editing label maps,
  and `buildCards` / `buildColumns` from the chat list.
- `boardApi.ts` is the commands: pure functions from the full board model
  and ids to a `Plan` of label writes, title writes, storage patch and undo
  text; `null` means no-op.
- `runPlan.ts` is the executor: applies one `Plan` through injected write,
  rename and storage functions (error toast, undo). `boardChats.ts` holds
  the board's all-chats query and the label mutation that patches the
  caches before the request.
- `boardDrag.ts` maps a dnd-kit collision to a drop target and a drop to
  a command; `windows.ts` is pure window geometry and list edits.
- `ChatBoardPage.tsx` owns state and mutations; `BoardHeader.tsx`,
  `BoardColumns.tsx` and `BoardWindows.tsx` are its JSX. Columns, cards and
  `NotesSection.tsx` translate gestures into one command call each; the
  page filters the rendered columns but always hands the full model to the
  commands. `BoardCard.tsx` composes `CardColorPicker.tsx`,
  `EditableTitle.tsx`, `ChatStatusLine.tsx` and `ChatInfo.tsx`;
  `DragGhost.tsx` is the overlay drawn for whatever is being dragged.
- `assistantSpecs.ts` writes the prompts and snapshots for the card and
  board assistants; `assistants.ts` finds or creates the chat for a spec.
  `DraftChat.tsx` is the regular create form inside a board window.

## Not done

- Multi pull request chats show the first pull request only.
- The assistant is not told when its snapshot is stale on return.
- Concurrent edits from two browsers are last write wins.
