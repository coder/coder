import type { QueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { workspaces } from "#/api/queries/workspaces";
import type { Chat, CreateChatRequest } from "#/api/typesGenerated";
import { type AssistantSpec, WORKSPACE_NAME } from "./assistantSpecs";
import { ASSISTANT_KEY } from "./boardLabels";

// Same key AgentChatPage.tsx writes when the user picks a model. Copied
// rather than exported so the experiment adds no surface to that page.
export const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const WORKSPACE_INSTRUCTION = `Workspace: none attached yet. Create "${WORKSPACE_NAME}" as described in your instructions before you verify anything; do not ask first.`;

/** Assistant chat id by `board/assistant` value: a card id, or the board key. */
export const assistantIds = (
	chats: readonly Chat[],
): ReadonlyMap<string, string> =>
	new Map(
		chats.flatMap((chat) => {
			const key = chat.labels[ASSISTANT_KEY];
			return key ? [[key, chat.id] as const] : [];
		}),
	);

interface OpenAssistant {
	readonly spec: AssistantSpec;
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
 * The assistant chat id for `spec`: the existing one, or a new chat in the
 * shared workspace titled from the spec. Undefined after a reported failure;
 * a failed rename is reported by the mutation and the chat still opens.
 */
export const openAssistant = async ({
	spec,
	existingId,
	create,
	rename,
	queryClient,
}: OpenAssistant): Promise<string | undefined> => {
	if (existingId) return existingId;
	try {
		const { workspaces: found } = await queryClient.fetchQuery(
			workspaces({ q: `owner:me name:${WORKSPACE_NAME}` }),
		);
		const workspace = found.find((w) => w.name === WORKSPACE_NAME);
		const model = localStorage.getItem(lastModelConfigIDStorageKey);
		const text = workspace
			? spec.snapshot
			: `${spec.snapshot}\n\n${WORKSPACE_INSTRUCTION}`;
		const chat = await create({
			organization_id: spec.organizationId,
			content: [{ type: "text", text }],
			system_prompt: spec.systemPrompt,
			workspace_id: workspace?.id,
			labels: { [ASSISTANT_KEY]: spec.key },
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
		});
		await rename({ chatId: chat.id, title: spec.title }).catch(() => undefined);
		return chat.id;
	} catch (error) {
		toast.error(getErrorMessage(error, "Failed to open the assistant."));
		return undefined;
	}
};
