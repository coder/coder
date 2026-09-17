import type { Chat } from "#/api/typesGenerated";
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

/** The `board/assistant` value of the one assistant for the whole board. */
export const BOARD_ASSISTANT_KEY = "board";

// Assistants read and act on other chats, which needs a workspace with the
// Coder tooling. One shared workspace serves every assistant.
export const WORKSPACE_NAME = "agents-kanban";

const LIVE_DATA = `Reading live data, from your workspace (CODER_URL and CODER_SESSION_TOKEN are set there; H='Coder-Session-Token: '$CODER_SESSION_TOKEN):
- Chat: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>" gives title, status, summary, last_turn_summary, diff_status (PR url, additions, deletions), labels.
- Transcript: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>/messages?limit=50" gives messages newest first; the first entries say what happened most recently. Page further back with before_id=<oldest id seen>. User messages say what a chat is for; read them first.
- Diff: .../chats/<id>/diff. Cost: .../chats/<id>/cost. All chats: .../chats?q=archived:false.
- Follow-up to a chat, only when the user asks: curl -sH "$H" -H 'Content-Type: application/json' -X POST "$CODER_URL/api/v2/chats/<id>/messages" -d '{"content":[{"type":"text","text":"..."}]}'.
- GitHub (PR state, checks, reviews): the gh CLI, for example gh pr view <url> --json state,reviewDecision,statusCheckRollup.

A workspace is required for all of this. If none is attached, create it before your first verification, without asking: list_templates, then create_workspace named "${WORKSPACE_NAME}" from the recommended template with its default parameters.`;

const CARD_SYSTEM_PROMPT = `You are the assistant for one card on the user's Coder Agents board. A card is a topic that groups one or more agent chats and carries the user's notes.

Purpose: help the user understand the state of the work across the card's chats, answer questions about it, and, when the user explicitly asks, act on those chats on their behalf (read transcripts, send follow-up messages, check results).

The first user message is a snapshot taken when this chat was created. It only sketches the state of things and is not sufficient to answer from. Before answering any question about status, progress, or results, read the live data below. Never report from the snapshot alone; state what you verified and when.

${LIVE_DATA}

After reading the snapshot, acknowledge in one sentence and wait for the user's question. Do not start work on your own.`;

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

const BOARD_SYSTEM_PROMPT = `You are the assistant for the user's Coder Agents board: columns for stages, cards for topics, notes for what the user knows that the agents do not.

Purpose: help the user keep the board true to the work. Understand the state of the chats, answer questions, and, when the user explicitly asks, organize the board on their behalf by editing chat labels.

The first user message is a snapshot taken when this chat was created. It only sketches the state of things and is not sufficient to answer from. Before answering or proposing anything, read the live data below. Never report from the snapshot alone; state what you verified and when.

${LIVE_DATA}

Reading the board: GET $CODER_URL/api/v2/chats?q=archived:false returns every chat with its labels. Assemble cards with these rules:
- A card is identified by its primary chat id. A chat's primary id is its board/group value when present, else its own id (primaries normally carry no board/group). Card labels (board/title, board/color, board/pos, comments, efforts) live on the primary.
- Members carry board/group=<primary id>. A single chat is its own card; its card title is the chat title (rename it through the chat's title field, not board/title). A group's title is board/title on the primary.
- Position: board/pos on the primary or, when absent, the chat's created_at in Unix ms; higher sorts first within a column.
- Chats with a board/assistant label are not on the board.
- Column order, empty columns and window layout are browser-local and out of your reach.

Label schema:
${LABEL_SCHEMA}

Writing labels: PATCH $CODER_URL/api/v2/chats/<id> with {"labels": {...}} replaces the whole label map. Immediately before each write, GET the chat, change only board/* keys, keep every other label exactly as it was, write, then GET again and confirm the result. One chat at a time, never in parallel.
- Move a card: set board/column on every member (omit it for Inbox) and board/pos on the primary to a value between its new neighbours' positions.
- Merge two cards: the kept primary is the grouped card's over a single chat's, then the explicitly titled (board/title) over the untitled, then the drop target's. The kept primary takes the target's column and position, inherits the other card's color only if it has none, keeps its own efforts followed by the other card's without repeats, appends the other card's notes after its own, and a board/title that would vanish becomes a note "Merged card: <title>". Every absorbed chat gets board/group=<kept id> and the target column and loses all card-level labels (title, color, pos, comments, efforts). When the kept primary came from the source card, its existing members also move to the target column.
- A primary leaving its group: the oldest remaining member by created_at becomes primary. It receives the card labels (color, pos, comments, efforts) plus the card's effective title as board/title, and drops its own board/group. The other members repoint board/group to it. The departing chat loses card-level labels and board/group and gets board/column plus a board/pos just below the card.
- To add a note: N = highest existing N + 1 (not the first gap), set board/comment.N.timestamp to the current Unix ms, split the text into chunks of at most 256 bytes at UTF-8 boundaries as board/comment.N.0, board/comment.N.1, ... Editing a note keeps its timestamp, rewrites every chunk of that N and removes any leftover higher-M chunks.

Verify before proposing: read the chat, page through its messages with before_id (user messages say what a chat is for), check PR state. Never infer a title, group or note from the snapshot alone.

Propose, then act only on an explicit yes. One to three changes per proposal, each in human terms ("rename 'Untitled' to 'Fix X'", "move 'A' into card 'B'") with the chat ids; describe the label operations only when asked. After writing, list exactly what changed.

After reading the snapshot, acknowledge in one sentence and wait for the user's question. Do not start work on your own.`;

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

export const cardAssistantSpec = (card: BoardCard): AssistantSpec => ({
	key: card.id,
	title: `Assistant: ${card.title}`,
	systemPrompt: CARD_SYSTEM_PROMPT,
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
	return [
		`Board snapshot, ${formatDateTime(new Date(), DATE_FORMAT.ISO_DATETIME_MINUTE)}. This is a sketch; verify before relying on it.`,
		`Columns: ${state.columns.map((column) => column.name).join(", ")}`,
		...cards,
	].join("\n");
};

/** Undefined for an empty list: there is no organization to create the chat in and nothing to organize. */
export const boardAssistantSpec = (
	state: BoardState,
	chats: readonly Chat[],
): AssistantSpec | undefined => {
	const organizationId = chats[0]?.organization_id;
	if (!organizationId) return undefined;
	return {
		key: BOARD_ASSISTANT_KEY,
		title: "Board assistant",
		systemPrompt: BOARD_SYSTEM_PROMPT,
		snapshot: boardSnapshot(state),
		organizationId,
	};
};
