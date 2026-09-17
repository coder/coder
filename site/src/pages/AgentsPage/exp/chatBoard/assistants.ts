import type { QueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { prependToInfiniteChatsCache } from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type { Chat, CreateChatRequest } from "#/api/typesGenerated";
import { type AssistantSpec, WORKSPACE_NAME } from "./assistantSpecs";
import { ASSISTANT_KEY } from "./boardLabels";

// Same key the chat pages write when the user picks a model
// (submitChatTurn.ts, AgentCreateForm.tsx). Copied rather than imported so
// the experiment adds no surface to those modules.
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

// Creations in flight, by `board/assistant` key, per client.
const opening = new WeakMap<
	QueryClient,
	Map<string, Promise<string | undefined>>
>();

const openingFor = (queryClient: QueryClient) => {
	let byKey = opening.get(queryClient);
	if (!byKey) {
		byKey = new Map();
		opening.set(queryClient, byKey);
	}
	return byKey;
};

/**
 * The assistant chat id for `spec`: the existing one, or a new chat in the
 * shared workspace titled from the spec. Undefined after a reported failure;
 * a failed rename is reported by the mutation and the chat still opens.
 * Opens for the same key on one client share one request until it settles,
 * so a second click before the list refetch does not create a second
 * assistant.
 */
export const openAssistant = (
	args: OpenAssistant,
): Promise<string | undefined> => {
	const { spec, existingId, queryClient } = args;
	if (existingId) return Promise.resolve(existingId);
	const byKey = openingFor(queryClient);
	const inFlight = byKey.get(spec.key);
	if (inFlight) return inFlight;
	const pending = createAssistant(args).finally(() => byKey.delete(spec.key));
	byKey.set(spec.key, pending);
	return pending;
};

const createAssistant = async ({
	spec,
	create,
	rename,
	queryClient,
}: OpenAssistant): Promise<string | undefined> => {
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
		// The board reads assistant ids from the list; seeding it lets the next
		// open find this chat before the refetch delivers it.
		prependToInfiniteChatsCache(queryClient, chat);
		await rename({ chatId: chat.id, title: spec.title }).catch(() => undefined);
		return chat.id;
	} catch (error) {
		toast.error(getErrorMessage(error, "Failed to open the assistant."));
		return undefined;
	}
};
