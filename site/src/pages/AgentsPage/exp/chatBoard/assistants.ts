import type { QueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	mcpServerConfigs,
	prependToInfiniteChatsCache,
} from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type {
	Chat,
	CreateChatRequest,
	MCPServerConfig,
} from "#/api/typesGenerated";
import {
	type AssistantSpec,
	type AssistantTools,
	WORKSPACE_NAME,
} from "./assistantSpecs";
import { ASSISTANT_KEY } from "./boardLabels";

// Same key the chat pages write when the user picks a model
// (submitChatTurn.ts, AgentCreateForm.tsx). Copied rather than imported so
// the experiment adds no surface to those modules.
export const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const WORKSPACE_INSTRUCTION = `Workspace: none attached yet. Create "${WORKSPACE_NAME}" as described in your instructions when you first need it; do not ask first.`;

// The deployment's own MCP server, which gives the assistant chat tools
// without a workspace. Usable only once the user has connected it.
const isUsableCoderMcp = (config: MCPServerConfig) =>
	config.enabled &&
	config.auth_connected &&
	(config.slug === "coder" ||
		config.url.endsWith("/api/experimental/mcp/http"));

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

type OpenAssistant = {
	/** Built once the chat's tools are known, since the prompt differs by them. */
	readonly spec: (tools: AssistantTools) => AssistantSpec;
	/** Id only: passing the chat object would let the compiler treat the list as mutated. */
	readonly existingId: string | undefined;
	readonly create: (req: CreateChatRequest) => Promise<Chat>;
	readonly rename: (vars: {
		chatId: string;
		title: string;
	}) => Promise<unknown>;
	readonly queryClient: QueryClient;
};

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
 * The assistant chat id for `spec`: the existing one, or a new chat titled
 * from the spec. A connected Coder MCP is attached and gets the prompt that
 * leans on it; otherwise the chat gets the curl-only prompt and the shared
 * workspace. Undefined after a reported failure; a failed rename is reported
 * by the mutation and the chat still opens. Opens for the same key on one
 * client share one request until it settles, so a second click before the
 * list refetch does not create a second assistant.
 */
export const openAssistant = (
	args: OpenAssistant,
): Promise<string | undefined> => {
	const { spec, existingId, queryClient } = args;
	if (existingId) return Promise.resolve(existingId);
	// The key does not depend on the tools; the bare spec is enough to find it.
	const { key } = spec({ coderMcp: false });
	const byKey = openingFor(queryClient);
	const inFlight = byKey.get(key);
	if (inFlight) return inFlight;
	const pending = createAssistant(args).finally(() => byKey.delete(key));
	byKey.set(key, pending);
	return pending;
};

const createAssistant = async ({
	spec,
	create,
	rename,
	queryClient,
}: OpenAssistant): Promise<string | undefined> => {
	try {
		const bare = spec({ coderMcp: false });
		// Failing to load the server list only means no MCP, not a failed open.
		const mcp = await queryClient
			.fetchQuery(mcpServerConfigs(bare.organizationId))
			.then((configs) => configs.find(isUsableCoderMcp))
			.catch(() => undefined);
		// With the MCP attached the chat can work without a workspace, so a
		// failed lookup only means none is attached; on the curl-only path
		// every read needs it, so the failure is the user's.
		const lookup = queryClient
			.fetchQuery(workspaces({ q: `owner:me name:${WORKSPACE_NAME}` }))
			.then(({ workspaces: found }) =>
				found.find((w) => w.name === WORKSPACE_NAME),
			);
		const workspace = await (mcp ? lookup.catch(() => undefined) : lookup);
		const built = mcp ? spec({ coderMcp: true }) : bare;
		const model = localStorage.getItem(lastModelConfigIDStorageKey);
		const text = workspace
			? built.snapshot
			: `${built.snapshot}\n\n${WORKSPACE_INSTRUCTION}`;
		const chat = await create({
			organization_id: built.organizationId,
			content: [{ type: "text", text }],
			system_prompt: built.systemPrompt,
			workspace_id: workspace?.id,
			mcp_server_ids: mcp ? [mcp.id] : undefined,
			labels: { [ASSISTANT_KEY]: built.key },
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
		});
		// The board reads assistant ids from the list; seeding it lets the next
		// open find this chat before the refetch delivers it.
		prependToInfiniteChatsCache(queryClient, chat);
		await rename({ chatId: chat.id, title: built.title }).catch(
			() => undefined,
		);
		return chat.id;
	} catch (error) {
		toast.error(getErrorMessage(error, "Failed to open the assistant."));
		return undefined;
	}
};
