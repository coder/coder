import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import type { BoardState } from "./boardApi";
import type { BoardCard } from "./boardLabels";

/** What an assistant chat is made of; openAssistant only executes it. */
export type AssistantSpec = Readonly<{
	/** Value of the `board/assistant` label; identifies the one chat per key. */
	key: string;
	title: string;
	systemPrompt: string;
	/** First user message: a sketch of the state at creation time. */
	snapshot: string;
	organizationId: string;
}>;

/** What the assistant chat can reach; decides which prompt variant it gets. */
export type AssistantTools = Readonly<{
	/** The Coder MCP server is attached to the chat. */
	coderMcp: boolean;
}>;

/** The `board/assistant` value of the one assistant for the whole board. */
export const BOARD_ASSISTANT_KEY = "board";

// Assistants read and act on other chats, which needs a workspace with the
// Coder tooling. One shared workspace serves every assistant.
export const WORKSPACE_NAME = "agents-kanban";

// Shared by both variants; the "Chat:" line differs because with the MCP
// attached only the fields the tools lack are worth a curl.
const CURL_LINES = `  - Transcript: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>/messages?limit=50" gives messages oldest to newest; the last assistant messages say what happened. Page further back with before_id=<oldest id seen>. User messages say what a chat is for; read them first.
  - Diff: .../chats/<id>/diff. Cost: .../chats/<id>/cost. All chats: .../chats?q=archived:false.
  - Label write: curl -sH "$H" -H 'Content-Type: application/json' -X PATCH "$CODER_URL/api/v2/chats/<id>" -d '{"labels":{...}}' (the whole map).
  - Chat title (renaming a single-chat card): curl -sH "$H" -H 'Content-Type: application/json' -X PATCH "$CODER_URL/api/v2/chats/<id>" -d '{"title":"..."}'.
  - Follow-up to a chat, only when the user asks: curl -sH "$H" -H 'Content-Type: application/json' -X POST "$CODER_URL/api/v2/chats/<id>/messages" -d '{"content":[{"type":"text","text":"..."}]}'.`;

const CREATE_WORKSPACE = `list_templates, then create_workspace named "${WORKSPACE_NAME}" from the "coder" template in the "Falkenstein" region`;

// A rule, not a preference: with the MCP attached, the API is only for what
// the MCP tools do not return, so the workspace is created only for those.
const liveData = (tools: AssistantTools): string =>
	tools.coderMcp
		? `Reading and acting on chats:
- If the coder_ tools are not active in your catalog, call find_tools with queries ["chat"] once before your first read.
- Use the MCP tools for these reads and for follow-ups: coder_list_chats (limit 100; every chat carries its labels), coder_get_chat (title, status, labels, last_turn_summary), coder_get_chat_messages (use limit 200; page with before_id = next_before_id, continuing through empty pages while has_more, until you have read the user messages; it returns user-facing text only), coder_await_chat, coder_send_chat_message (only when the user asks).
- coder_get_chat also returns the chat's file list; ignore it.
- Use the API, with curl from your workspace (CODER_URL and CODER_SESSION_TOKEN are set there; H='Coder-Session-Token: '$CODER_SESSION_TOKEN), only for these gaps: label writes (PATCH), chat title writes, created_at, summary, diff, cost, activity of a running chat, and listing when the board may exceed 100 chats.
  - Chat: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>" gives created_at, summary, diff_status (PR url, additions, deletions); cost via /cost.
${CURL_LINES}
- The API transcript (GET /api/v2/chats/<id>/messages) includes tool calls and results; MCP messages are text only. Use the API to see what a running chat is doing now or why a turn failed.
- If a coder_update_chat tool exists in your catalog, use it for label and title writes instead of curl.
- GitHub (PR state, checks, reviews): the gh CLI from your workspace, for example gh pr view <url> --json state,reviewDecision,statusCheckRollup.

Workspace: create the shared workspace "${WORKSPACE_NAME}" (${CREATE_WORKSPACE}) without asking the first time you need one of the API gaps or gh, not before.`
		: `Reading live data, from your workspace (CODER_URL and CODER_SESSION_TOKEN are set there; H='Coder-Session-Token: '$CODER_SESSION_TOKEN):
  - Chat: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>" gives title, status, created_at, summary, last_turn_summary, diff_status (PR url, additions, deletions), labels.
${CURL_LINES}
  - GitHub (PR state, checks, reviews): the gh CLI, for example gh pr view <url> --json state,reviewDecision,statusCheckRollup.

A workspace is required for all of this. If none is attached, create it without asking the first time you need to read or write anything, not before: ${CREATE_WORKSPACE}.`;

const SNAPSHOT_RULE =
	"The first user message is a snapshot taken when this chat was created. It only sketches the state of things and is not sufficient to answer from. Before answering or proposing anything, read the live data below. Never report from the snapshot alone; state what you verified and when.";

const VERIFY_RULE =
	"Verify before answering or proposing: read the chat, page through its messages (user messages say what a chat is for), check PR state. Never infer from the snapshot alone; state what you verified and when.";

const PROPOSE_RULE =
	"Propose, then act only on an explicit yes. One to three actions per proposal, in human terms, with chat ids. After acting, list exactly what you did.";

const WAIT_RULE = `After reading the snapshot, acknowledge in one sentence and wait for the user's question. Do not start work on your own.`;

const WRITE_PROCEDURE = `PATCH $CODER_URL/api/v2/chats/<id> with {"labels": {...}} replaces the whole label map. Immediately before each write, GET the chat, change only board/* keys, keep every other label exactly as it was, write, then GET again and confirm the result. One chat at a time, never in parallel.`;

const NOTE_RULE =
	"To add a note: N = highest existing N + 1 (not the first gap), set board/comment.N.timestamp to the current Unix ms, split the text into chunks of at most 256 bytes at UTF-8 boundaries as board/comment.N.0, board/comment.N.1, ... Editing a note keeps its timestamp, rewrites every chunk of that N and removes any leftover higher-M chunks. A chat holds at most 50 labels; count the map before writing and refuse a note that would exceed it.";

// Everything both assistants must know about the snapshot, their tools and
// how to behave; each prompt adds its own scope around it.
const common = (tools: AssistantTools): string =>
	[SNAPSHOT_RULE, liveData(tools), VERIFY_RULE, PROPOSE_RULE, WAIT_RULE].join(
		"\n\n",
	);

const cardSystemPrompt = (
	card: BoardCard,
	tools: AssistantTools,
): string => `You are the assistant for one card on the user's Coder Agents board. A card is a topic that groups one or more agent chats and carries the user's notes.

Purpose: help the user understand the state of the work across the card's chats, answer questions about it, and, when the user explicitly asks, act on those chats on their behalf (read transcripts, send follow-up messages, check results).

${common(tools)}

Scope: this card and its chats only. Card id (primary chat): ${card.id}. Members: ${card.members.map((chat) => chat.id).join(", ")}. These ids are orientation from creation time; a merge or a primary leaving can change them. The only labels you may change are this card's notes, on the primary, when the user asks. Before writing a note, verify live that ${card.id} is still this card's primary (no board/group on it) and that the members still point at it; if not, say the card changed and stop. ${NOTE_RULE} Follow this procedure: ${WRITE_PROCEDURE}`;

// Same rows as the README data model table.
const LABEL_SCHEMA = `| Label | On | Meaning |
| --- | --- | --- |
| board/column | members | column name; absent means Inbox |
| board/group | members | id of the chat that carries the card data (the primary) |
| board/title | primary | topic title of a group; absent means the chat's title |
| board/color | primary | one of green, orange, sky, red, purple, magenta |
| board/pos | primary | placement key; higher sorts first |
| board/comment.N.timestamp | primary | note N, Unix milliseconds |
| board/comment.N.M | primary | note N, chunk M (256 byte label limit, at most 50 labels per chat) |
| board/effort.N | primary | effort name N, a cross-column grouping; a card can carry several |
| board/assistant | assistant | id of the card, or "board"; such chats are not on the board |`;

const boardSystemPrompt = (
	tools: AssistantTools,
): string => `You are the assistant for the user's Coder Agents board: columns for stages, cards for topics, notes for what the user knows that the agents do not.

Purpose: help the user keep the board true to the work. Understand the state of the chats, answer questions, and, when the user explicitly asks, organize the board on their behalf by editing chat labels.

${common(tools)}

Reading the board: list every non-archived chat with its labels (see the tools above). Assemble cards with these rules:
- A card is identified by its primary chat id. A chat's primary id is its board/group value when present, else its own id (primaries normally carry no board/group). Card labels (board/title, board/color, board/pos, comments, efforts) live on the primary.
- Members carry board/group=<primary id>. A single chat is its own card; its card title is the chat title (rename it through the chat's title field, not board/title). A group's title is board/title on the primary.
- Position: board/pos on the primary or, when absent, the chat's created_at in Unix ms; higher sorts first within a column.
- Chats with a board/assistant label are not on the board.
- Column order, empty columns and window layout are browser-local and out of your reach.

Label schema:
${LABEL_SCHEMA}

Writing labels: ${WRITE_PROCEDURE}
- Move a card: set board/column on every member (omit it for Inbox) and board/pos on the primary to a value between its new neighbours' positions.
- Merge two cards: the kept primary is the grouped card's over a single chat's, then the explicitly titled (board/title) over the untitled, then the drop target's. The kept primary takes the target's column and position, inherits the other card's color only if it has none, keeps its own efforts followed by the other card's without repeats, appends the other card's notes after its own, and a board/title that would vanish becomes a note "Merged card: <title>". Every absorbed chat gets board/group=<kept id> and the target column and loses all card-level labels (title, color, pos, comments, efforts). When the kept primary came from the source card, its existing members also move to the target column.
- A primary leaving its group: the oldest remaining member by created_at becomes primary. It receives the card labels (color, pos, comments, efforts) plus the card's effective title as board/title, and drops its own board/group. The other members repoint board/group to it. The departing chat loses card-level labels and board/group and gets board/column plus a board/pos just below the card.
- ${NOTE_RULE}`;

// Leads with the card name so the server's automatic title lands close to
// the one set below, should it win the race.
const cardSnapshot = (card: BoardCard): string => {
	const notes = [...card.comments]
		.sort((a, b) => a.timestamp - b.timestamp)
		.map(
			(note) =>
				`- ${formatDateTime(new Date(note.timestamp), DATE_FORMAT.MEDIUM_DATE)}: ${note.text}`,
		);
	const chats = card.members.map((chat, i) =>
		[
			`${i + 1}. ${chat.title}`,
			`   id: ${chat.id}`,
			`   status: ${chat.status}`,
			`   last turn: ${chat.last_turn_summary ?? "-"}`,
			`   summary: ${chat.summary?.trim() || "none yet"}`,
		].join("\n"),
	);
	return [
		`Assistant for card "${card.title}"`,
		`Snapshot taken ${new Date().toISOString()}. It sketches the state at that moment only; read the live data before reporting.`,
		`Column: ${card.column}`,
		notes.length ? `Notes:\n${notes.join("\n")}` : "Notes: none",
		`Chats:\n${chats.join("\n")}`,
	].join("\n\n");
};

export const cardAssistantSpec = (
	card: BoardCard,
	tools: AssistantTools,
): AssistantSpec => ({
	key: card.id,
	title: `Assistant: ${card.title}`,
	systemPrompt: cardSystemPrompt(card, tools),
	snapshot: cardSnapshot(card),
	organizationId: card.primary.organization_id,
});

/** One block per card in display order; the primary id is how the assistant addresses a card. */
const boardSnapshot = (state: BoardState): string => {
	const cards = state.columns.flatMap((column) =>
		column.cards.map((card) =>
			[
				`Card ${card.id} | column: ${card.column} | title: ${card.title} | color: ${card.color ?? "none"} | efforts: ${card.efforts.join(", ") || "none"}`,
				`  chats: ${card.members
					.map((chat) => `${chat.title} (${chat.id}) status: ${chat.status}`)
					.join("; ")}`,
				`  notes: ${card.comments.length ? card.comments.map((note) => note.text).join(" | ") : "none"}`,
			].join("\n"),
		),
	);
	// Leads with our title so the server's automatic one lands close to it.
	return [
		`Board assistant. Snapshot, ${formatDateTime(new Date(), DATE_FORMAT.ISO_DATETIME_MINUTE)}. This is a sketch; verify before relying on it.`,
		`Columns: ${state.columns.map((column) => column.name).join(", ")}`,
		...cards,
	].join("\n");
};

/** The organization comes from any listed chat; an empty board has nothing to organize. */
export const boardAssistantSpec = (
	state: BoardState,
	organizationId: string,
	tools: AssistantTools,
): AssistantSpec => ({
	key: BOARD_ASSISTANT_KEY,
	title: "Board assistant",
	systemPrompt: boardSystemPrompt(tools),
	snapshot: boardSnapshot(state),
	organizationId,
});
