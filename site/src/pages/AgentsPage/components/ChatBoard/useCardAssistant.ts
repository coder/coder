import { useEffect, useRef } from "react";
import { useMutation, useQueryClient } from "react-query";
import { API } from "#/api/api";
import { createChat, updateChatTitle } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { lastModelConfigIDStorageKey } from "../../AgentChatPage";
import { ASSISTANT_KEY, type BoardCard } from "./boardLabels";

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

A workspace is required for all of this. If none is attached, create it before your first verification, without asking: list_templates, then create_workspace named "${WORKSPACE_NAME}" from the "coder" template in the "Falkenstein" region.

After reading the snapshot, acknowledge in one sentence and wait for the user's question. Do not start work on your own.`;

const WORKSPACE_INSTRUCTION = `Workspace: none attached yet. Create "${WORKSPACE_NAME}" as described in your instructions before you verify anything; do not ask first.`;

const assistantTitle = (card: BoardCard) => `Assistant: ${card.title}`;

/** The card's assistant chat, if one was created before. */
export const findAssistant = (
	card: BoardCard,
	chats: readonly Chat[],
): Chat | undefined =>
	chats.find((chat) => chat.labels[ASSISTANT_KEY] === card.id);

// Leads with the card name so the server's automatic title lands close to
// the one set below, should it win the race.
const snapshot = (card: BoardCard, workspaceMissing: boolean): string => {
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

/** Creates a card's assistant chat on first use; later opens reuse it. */
export const useCardAssistant = (
	chats: readonly Chat[],
	cards: readonly BoardCard[],
) => {
	const queryClient = useQueryClient();
	const create = useMutation(createChat(queryClient));
	const rename = useMutation(updateChatTitle(queryClient));

	// The server generates a title from the first message shortly after
	// creation, overwriting ours. Put it back when that happens to a young
	// chat; older renames are the user's and stay.
	const renamed = useRef(new Set<string>());
	const renameAsync = rename.mutateAsync;
	useEffect(() => {
		const cardById = new Map(cards.map((card) => [card.id, card]));
		for (const chat of chats) {
			const card = cardById.get(chat.labels[ASSISTANT_KEY] ?? "");
			if (!card || renamed.current.has(chat.id)) continue;
			const young =
				Date.now() - new Date(chat.created_at).getTime() < 5 * 60_000;
			if (!young || chat.title === assistantTitle(card)) continue;
			renamed.current.add(chat.id);
			void renameAsync({ chatId: chat.id, title: assistantTitle(card) });
		}
	}, [chats, cards, renameAsync]);

	const open = async (
		card: BoardCard,
		existing: Chat | undefined,
	): Promise<string> => {
		const title = assistantTitle(card);
		if (existing) return existing.id;
		const { workspaces } = await API.getWorkspaces({
			q: `owner:me name:${WORKSPACE_NAME}`,
		});
		const workspace = workspaces.find((w) => w.name === WORKSPACE_NAME);
		const model = localStorage.getItem(lastModelConfigIDStorageKey);
		const chat = await create.mutateAsync({
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
		await rename.mutateAsync({ chatId: chat.id, title });
		return chat.id;
	};

	return { open, isCreating: create.isPending };
};
