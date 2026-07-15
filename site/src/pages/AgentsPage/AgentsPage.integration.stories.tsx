import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { userChatDebugLogging } from "#/api/queries/chatDebugLogging";
import type { ChatExecutionSnapshotEvent } from "#/api/queries/chats";
import {
	chatModelConfigs,
	chatModelsKey,
	chatPromptsQuery,
	chatQueryKeys,
	mcpServerConfigsKey,
	userChatPersonalModelOverrides,
	userChatProviderConfigs,
	userCompactionThresholds,
} from "#/api/queries/chats";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { preferenceSettings } from "#/api/queries/users";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MOCK_TIMESTAMP,
	MockChat,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import { MockDeploymentSSH, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import AgentChatPage from "./AgentChatPage";
import AgentsPage from "./AgentsPage";

const CHAT_ID = MockChat.id;

const chat: TypesGen.Chat = {
	...MockChat,
	title: "Data flow contract",
};

const modelCatalog: TypesGen.ChatModelsResponse = {
	providers: [
		{
			provider: "openai",
			available: true,
			models: [
				{
					id: "openai:gpt-4o",
					provider: "openai",
					model: "gpt-4o",
					display_name: "GPT-4o",
				},
			],
		},
	],
	unsupported_providers: [],
};

const modelConfigs: TypesGen.ChatModelConfig[] = [
	{
		...MockChatModelConfig,
		id: chat.last_model_config_id ?? "model-config-1",
		model: "gpt-4o",
		display_name: "GPT-4o",
		is_default: true,
	},
];

type SocketListener = (event: Event | MessageEvent<string>) => void;

type SocketConnection = {
	readonly url: string;
	readonly socket: ContractWebSocket;
	publishOpen: () => void;
	publishMessage: (data: unknown) => void;
};

let socketConnections: SocketConnection[] = [];
let listedChat = chat;
let resolveMessages:
	| ((response: TypesGen.ChatMessagesResponse) => void)
	| undefined;

class ContractWebSocket {
	static readonly CONNECTING = 0;
	static readonly OPEN = 1;
	static readonly CLOSING = 2;
	static readonly CLOSED = 3;

	readonly CONNECTING = ContractWebSocket.CONNECTING;
	readonly OPEN = ContractWebSocket.OPEN;
	readonly CLOSING = ContractWebSocket.CLOSING;
	readonly CLOSED = ContractWebSocket.CLOSED;
	readonly url: string;
	readonly protocol = "";
	readonly extensions = "";
	readonly bufferedAmount = 0;
	binaryType: BinaryType = "blob";
	readyState = ContractWebSocket.OPEN;
	onclose: ((this: WebSocket, ev: CloseEvent) => unknown) | null = null;
	onerror: ((this: WebSocket, ev: Event) => unknown) | null = null;
	onmessage: ((this: WebSocket, ev: MessageEvent) => unknown) | null = null;
	onopen: ((this: WebSocket, ev: Event) => unknown) | null = null;

	readonly #listeners = new Map<string, Set<SocketListener>>();

	constructor(url: string | URL) {
		this.url = String(url);
		socketConnections.push({
			url: this.url,
			socket: this,
			publishOpen: () => this.#publish("open", new Event("open")),
			publishMessage: (data) =>
				this.#publish(
					"message",
					new MessageEvent("message", { data: JSON.stringify(data) }),
				),
		});
	}

	addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
		const listeners = this.#listeners.get(type) ?? new Set<SocketListener>();
		const callback: SocketListener =
			typeof listener === "function"
				? listener
				: (event) => listener.handleEvent(event);
		listeners.add(callback);
		this.#listeners.set(type, listeners);
	}

	removeEventListener(
		type: string,
		listener: EventListenerOrEventListenerObject,
	) {
		if (typeof listener !== "function") {
			return;
		}
		this.#listeners.get(type)?.delete(listener);
	}

	close() {
		this.readyState = ContractWebSocket.CLOSED;
	}

	dispatchEvent() {
		return true;
	}

	send() {}

	#publish(type: string, event: Event | MessageEvent<string>) {
		if (this.readyState === ContractWebSocket.CLOSED) {
			return;
		}
		for (const listener of this.#listeners.get(type) ?? []) {
			listener(event);
		}
	}
}

const activeConnection = (urlPart: string) =>
	[...socketConnections]
		.reverse()
		.find(
			(connection) =>
				connection.socket.readyState === ContractWebSocket.OPEN &&
				connection.url.includes(urlPart),
		);

const personalModelOverride = (
	context: TypesGen.ChatPersonalModelOverrideContext,
): TypesGen.ChatPersonalModelOverride => ({
	context,
	mode: "chat_default",
	model_config_id: "",
	is_set: false,
	is_malformed: false,
});

const meta: Meta<typeof AgentsPage> = {
	title: "pages/AgentsPage/DataFlowContract",
	component: AgentsPage,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		queries: [
			{ key: chatModelsKey, data: modelCatalog },
			{ key: chatModelConfigs().queryKey, data: modelConfigs },
			{ key: userChatProviderConfigs().queryKey, data: [] },
			{
				key: userChatPersonalModelOverrides().queryKey,
				data: {
					enabled: false,
					root: personalModelOverride("root"),
					general: personalModelOverride("general"),
					explore: personalModelOverride("explore"),
					deployment_defaults: {
						general: {
							context: "general",
							model_config_id: "",
							reasoning_effort: "",
							is_malformed: false,
						},
						explore: {
							context: "explore",
							model_config_id: "",
							reasoning_effort: "",
							is_malformed: false,
						},
					},
				} satisfies TypesGen.UserChatPersonalModelOverridesResponse,
			},
			{
				key: userCompactionThresholds().queryKey,
				data: {
					thresholds: [],
				} satisfies TypesGen.UserChatCompactionThresholds,
			},
			{ key: mcpServerConfigsKey, data: [] },
			{
				key: preferenceSettings().queryKey,
				data: {
					task_notification_alert_dismissed: false,
					thinking_display_mode: "always_expanded",
					shell_tool_display_mode: "always_expanded",
					code_diff_display_mode: "always_expanded",
					agent_chat_send_shortcut: "enter",
				} satisfies TypesGen.UserPreferenceSettings,
			},
			{
				key: userChatDebugLogging().queryKey,
				data: {
					debug_logging_enabled: false,
					user_toggle_allowed: false,
					forced_by_deployment: false,
				} satisfies TypesGen.UserChatDebugLoggingSettings,
			},
			{
				key: workspaces({ q: "owner:me", limit: 0 }).queryKey,
				data: { workspaces: [], count: 0 },
			},
			{ key: deploymentSSHConfig().queryKey, data: MockDeploymentSSH },
			{ key: chatPromptsQuery(CHAT_ID).queryKey, data: { prompts: [] } },
			{
				key: chatQueryKeys.diffContents(CHAT_ID),
				data: { chat_id: CHAT_ID },
			},
			{
				key: getAuthorizationKey({
					checks: {
						canShareChat: {
							object: {
								resource_type: "chat",
								owner_id: chat.owner_id,
								organization_id: chat.organization_id,
							},
							action: "share",
						},
					},
				}),
				data: { canShareChat: true },
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				path: `/agents/${CHAT_ID}`,
				pathParams: { agentId: CHAT_ID },
			},
			routing: [
				{
					path: "/agents",
					useStoryElement: true,
					children: [{ path: ":agentId", element: <AgentChatPage /> }],
				},
			],
		}),
	},
	beforeEach: () => {
		socketConnections = [];
		listedChat = chat;
		const originalWebSocket = globalThis.WebSocket;
		globalThis.WebSocket = ContractWebSocket as unknown as typeof WebSocket;

		const messagesPromise = new Promise<TypesGen.ChatMessagesResponse>(
			(resolve) => {
				resolveMessages = resolve;
			},
		);
		spyOn(API.experimental, "getChats").mockImplementation(async () => [
			listedChat,
		]);
		spyOn(API.experimental, "getChat").mockResolvedValue(chat);
		spyOn(API.experimental, "getChatMessages").mockReturnValue(messagesPromise);
		spyOn(API, "getApiKey").mockRejectedValue(new Error("missing API key"));
		spyOn(API.experimental, "updateChat").mockResolvedValue();

		return () => {
			resolveMessages = undefined;
			globalThis.WebSocket = originalWebSocket;
		};
	},
};

export default meta;
type Story = StoryObj<typeof AgentsPage>;

export const RestBootstrapThenSnapshotStream: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await waitFor(() => {
			expect(activeConnection("/api/experimental/chats/watch")).toBeDefined();
		});
		expect(activeConnection(`/chats/${CHAT_ID}/stream`)).toBeUndefined();

		resolveMessages?.({
			messages: [MockChatMessage],
			queued_messages: [],
			has_more: false,
		});

		await expect(canvas.findByText("Hello")).resolves.toBeVisible();
		await waitFor(() => {
			const connection = activeConnection(`/chats/${CHAT_ID}/stream`);
			expect(connection).toBeDefined();
			expect(connection?.url).toContain("after_id=1");
		});

		const stream = activeConnection(`/chats/${CHAT_ID}/stream`);
		stream?.publishOpen();
		stream?.publishMessage([
			{
				type: "message",
				chat_id: CHAT_ID,
				message: {
					id: 2,
					chat_id: CHAT_ID,
					created_at: MOCK_TIMESTAMP,
					role: "assistant",
					content: [{ type: "text", text: "Snapshot reply" }],
				},
			} satisfies ChatExecutionSnapshotEvent,
		]);

		await expect(canvas.findByText("Snapshot reply")).resolves.toBeVisible();

		listedChat = { ...chat, title: "Recovered without a global hint" };
		activeConnection("/api/experimental/chats/watch")?.publishOpen();
		await expect(
			canvas.findByText("Recovered without a global hint"),
		).resolves.toBeVisible();
	},
};
