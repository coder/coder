import type { QueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { workspaces } from "#/api/queries/workspaces";
import type { Chat, CreateChatRequest } from "#/api/typesGenerated";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ASSISTANT_KEY, type BoardCard } from "./boardLabels";

// Same key AgentChatPage.tsx writes when the user picks a model. Copied
// rather than exported so the experiment adds no surface to that page.
const lastModelConfigIDStorageKey = "agents.last-model-config-id";

// The assistant reads and acts on other chats, which needs a workspace with
// the Coder tooling. One shared workspace serves every card's assistant.
const WORKSPACE_NAME = "agents-kanban";

const SYSTEM_PROMPT = `You are the assistant for one card on the user's Coder Agents board. A card is a topic that groups one or more agent chats and carries the user's notes.

Purpose: help the user understand the state of the work across the card's chats, answer questions about it, and, when the user explicitly asks, act on those chats on their behalf (read transcripts, send follow-up messages, check results).

The first user message is a snapshot taken when this chat was created. It only sketches the state of things and is not sufficient to answer from. Before answering any question about status, progress, or results, read the live data below. Never report from the snapshot alone; state what you verified and when.

Reading live data, from your workspace (CODER_URL and CODER_SESSION_TOKEN are set there; H='Coder-Session-Token: '$CODER_SESSION_TOKEN):
- Chat: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>" gives title, status, summary, last_turn_summary, diff_status (PR url, additions, deletions).
- Transcript: curl -sH "$H" "$CODER_URL/api/v2/chats/<id>/messages?limit=50" gives messages oldest to newest; the last assistant messages say what happened.
- Diff: .../chats/<id>/diff. Cost: .../chats/<id>/cost. All chats: .../chats?q=archived:false.
- Follow-up to a chat, only when the user asks: curl -sH "$H" -H 'Content-Type: application/json' -X POST "$CODER_URL/api/v2/chats/<id>/messages" -d '{"content":[{"type":"text","text":"..."}]}'.
- GitHub (PR state, checks, reviews): the gh CLI, for example gh pr view <url> --json state,reviewDecision,statusCheckRollup.

A workspace is required for all of this. If none is attached, create it before your first verification, without asking: list_templates, then create_workspace named "${WORKSPACE_NAME}" from the recommended template with its default parameters.

After reading the snapshot, acknowledge in one sentence and wait for the user's question. Do not start work on your own.`;

const WORKSPACE_INSTRUCTION = `Workspace: none attached yet. Create "${WORKSPACE_NAME}" as described in your instructions before you verify anything; do not ask first.`;

/** Assistant chat id by card id, for every card that has one. */
export const assistantIds = (
	chats: readonly Chat[],
): ReadonlyMap<string, string> =>
	new Map(
		chats.flatMap((chat) => {
			const cardId = chat.labels[ASSISTANT_KEY];
			return cardId ? [[cardId, chat.id] as const] : [];
		}),
	);

// Leads with the card name: the server titles the chat from its first
// message and may overwrite the title set below, so the automatic title
// should land close to it. Backend follow-up: accept a title on create.
export const snapshot = (
	card: BoardCard,
	workspaceMissing: boolean,
): string => {
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
		...(workspaceMissing ? [WORKSPACE_INSTRUCTION] : []),
	].join("\n\n");
};

interface OpenCardAssistant {
	readonly card: BoardCard;
	/** Id only: passing the chat object would let the compiler treat the list as mutated. */
	readonly existingId: string | undefined;
	readonly create: (req: CreateChatRequest) => Promise<Chat>;
	readonly rename: (vars: {
		chatId: string;
		title: string;
	}) => Promise<unknown>;
	readonly queryClient: QueryClient;
}

/**
 * The card's assistant chat id: the existing one, or a new chat in the shared
 * workspace titled for the card. Undefined after a reported failure; a failed
 * rename is reported by the mutation and the chat still opens.
 */
export const openCardAssistant = async ({
	card,
	existingId,
	create,
	rename,
	queryClient,
}: OpenCardAssistant): Promise<string | undefined> => {
	if (existingId) return existingId;
	try {
		const { workspaces: found } = await queryClient.fetchQuery(
			workspaces({ q: `owner:me name:${WORKSPACE_NAME}` }),
		);
		const workspace = found.find((w) => w.name === WORKSPACE_NAME);
		const model = localStorage.getItem(lastModelConfigIDStorageKey);
		const chat = await create({
			organization_id: card.primary.organization_id,
			content: [
				{ type: "text", text: snapshot(card, workspace === undefined) },
			],
			system_prompt: SYSTEM_PROMPT,
			workspace_id: workspace?.id,
			labels: { [ASSISTANT_KEY]: card.id },
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
		});
		await rename({ chatId: chat.id, title: `Assistant: ${card.title}` }).catch(
			() => undefined,
		);
		return chat.id;
	} catch (error) {
		toast.error(getErrorMessage(error, "Failed to open the assistant."));
		return undefined;
	}
};
