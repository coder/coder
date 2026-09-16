import type {
	CallToolResult,
	ReadResourceResult,
	Transport,
} from "@modelcontextprotocol/client";
import {
	CallToolResultSchema,
	ReadResourceResultSchema,
} from "@modelcontextprotocol/core";
import type { McpUiHostContext } from "@modelcontextprotocol/ext-apps/app-bridge";
import { AppBridge } from "@modelcontextprotocol/ext-apps/app-bridge";
import { useEffect, useEffectEvent, useReducer, useRef } from "react";
import { useMutation } from "react-query";
import {
	callChatMCPAppTool,
	readChatMCPAppResource,
} from "#/api/queries/chats";
import type { ChatStore } from "../ChatConversation/chatStore";
import {
	type BoundCall,
	initialMcpAppLifecycleState,
	type McpAppLifecycleState,
	mcpAppLifecycleReducer,
} from "./mcpAppLifecycle";
import { flattenModelContext } from "./mcpAppParts";
import {
	MCP_APP_VIEW_SANDBOX,
	type McpAppCsp,
	type McpAppPermissions,
} from "./sandboxUrl";

const TEARDOWN_TIMEOUT_MS = 2_000;
const BOOT_TIMEOUT_MS = 15_000;

type McpAppView = {
	html: string;
	csp?: McpAppCsp;
	permissions?: McpAppPermissions;
};

export type McpAppContainerSize = { width: number; height: number };

type BridgeSession = {
	bridge: AppBridge;
	initialized: boolean;
	tornDown: boolean;
	bootTimer: ReturnType<typeof setTimeout> | undefined;
	/** Which snapshot pieces this view instance has already received. */
	sent: {
		input: boolean;
		result: boolean;
		cancelled: boolean;
		partialArgs: string | undefined;
	};
};

const sleep = (ms: number): Promise<void> =>
	new Promise((resolve) => setTimeout(resolve, ms));

const teardownSession = async (session: BridgeSession): Promise<void> => {
	if (!session.initialized || session.tornDown) {
		return;
	}
	session.tornDown = true;
	await Promise.race([
		session.bridge.teardownResource({}).catch(() => undefined),
		sleep(TEARDOWN_TIMEOUT_MS),
	]);
};

const closeSession = async (session: BridgeSession): Promise<void> => {
	clearTimeout(session.bootTimer);
	await teardownSession(session);
	await session.bridge.close().catch(() => undefined);
};

const toCallToolResult = (
	result: Record<string, unknown> | undefined,
): CallToolResult | undefined => {
	if (!result) {
		return undefined;
	}
	const parsed = CallToolResultSchema.safeParse(result);
	if (parsed.success) {
		return parsed.data;
	}
	return {
		content: [{ type: "text", text: JSON.stringify(result) }],
		isError: true,
	};
};

const isHttpUrl = (value: string): boolean => {
	try {
		const url = new URL(value);
		return url.protocol === "http:" || url.protocol === "https:";
	} catch {
		return false;
	}
};

const textFromContent = (
	content: readonly { type: string; text?: string }[],
): string =>
	content
		.filter((block) => block.type === "text" && typeof block.text === "string")
		.map((block) => block.text ?? "")
		.join("\n");

const buildHostContext = ({
	theme,
	containerSize,
}: {
	theme: "light" | "dark";
	containerSize: McpAppContainerSize | undefined;
}): McpUiHostContext => ({
	theme,
	displayMode: "inline",
	availableDisplayModes: ["inline"],
	platform: "web",
	userAgent: "coder",
	locale: navigator.language,
	timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
	...(containerSize && containerSize.width > 0 && containerSize.height > 0
		? { containerDimensions: containerSize }
		: {}),
});

export type UseMcpAppBridgeOptions = {
	chatId: string;
	mcpServerConfigId: string;
	resourceUri: string;
	/** Tab ID under which model context updates are stored. */
	appId: string;
	store: Pick<ChatStore, "setMcpAppContext">;
	hostVersion: string;
	theme: "light" | "dark";
	containerSize: McpAppContainerSize | undefined;
	/** Undefined until the sandbox iframe window exists. */
	transport: Transport | undefined;
	/** Undefined until the view resource has been read. */
	view: McpAppView | undefined;
	/**
	 * Tool call this tab renders. A change is forwarded to an initialized view
	 * of the same resource; otherwise the view is torn down and reloaded.
	 */
	toolCallId: string | undefined;
	boundCall: BoundCall | undefined;
	/** Named in the boot timeout message. */
	sandboxOrigin: string | undefined;
	/** Undefined when the chat cannot accept messages; ui/message is then rejected. */
	onAppMessage: ((text: string) => Promise<void>) | undefined;
	/** Resolves once the user let the link open; rejects when declined. */
	onOpenLink: (url: string) => Promise<void>;
	/** When true the view is torn down and `onClosed` fires once that finished. */
	closing: boolean;
	onClosed: () => void;
};

/**
 * Owns one `AppBridge` per sandbox iframe instance. The connection lives in
 * a single effect keyed on the transport; every other input is read through
 * effect events so handler registration never restarts the connection.
 */
export const useMcpAppBridge = ({
	chatId,
	mcpServerConfigId,
	resourceUri,
	appId,
	store,
	hostVersion,
	theme,
	containerSize,
	transport,
	view,
	toolCallId,
	boundCall,
	sandboxOrigin,
	onAppMessage,
	onOpenLink,
	closing,
	onClosed,
}: UseMcpAppBridgeOptions): McpAppLifecycleState => {
	const [lifecycle, dispatch] = useReducer(
		mcpAppLifecycleReducer,
		initialMcpAppLifecycleState,
	);
	const sessionRef = useRef<BridgeSession | null>(null);
	const boundRef = useRef<{ toolCallId: string; resourceUri: string } | null>(
		null,
	);
	const { mutateAsync: callTool } = useMutation(
		callChatMCPAppTool(chatId, mcpServerConfigId),
	);
	const { mutateAsync: readResource } = useMutation(
		readChatMCPAppResource(chatId, mcpServerConfigId),
	);

	const handleSandboxReady = useEffectEvent((session: BridgeSession) => {
		if (sessionRef.current !== session || !view) {
			return;
		}
		dispatch({ type: "sandboxReady" });
		void session.bridge.sendSandboxResourceReady({
			html: view.html,
			sandbox: MCP_APP_VIEW_SANDBOX,
			...(view.csp ? { csp: view.csp } : {}),
			...(view.permissions ? { permissions: view.permissions } : {}),
		});
	});

	const flushBoundCall = useEffectEvent((session: BridgeSession) => {
		if (!session.initialized || session.tornDown || !boundCall) {
			return;
		}
		const { bridge, sent } = session;
		if (!sent.input) {
			if (boundCall.argsComplete) {
				sent.input = true;
				void bridge.sendToolInput({ arguments: boundCall.args });
			} else if (boundCall.args) {
				const serialized = JSON.stringify(boundCall.args);
				if (serialized !== sent.partialArgs) {
					sent.partialArgs = serialized;
					void bridge.sendToolInputPartial({ arguments: boundCall.args });
				}
			}
		}
		if (sent.result || sent.cancelled) {
			return;
		}
		const result = toCallToolResult(boundCall.result);
		if (result) {
			sent.result = true;
			void bridge.sendToolResult(result);
		} else if (boundCall.cancelled) {
			sent.cancelled = true;
			void bridge.sendToolCancelled({ reason: "The chat stopped" });
		}
	});

	const handleInitialized = useEffectEvent((session: BridgeSession) => {
		if (sessionRef.current !== session) {
			return;
		}
		session.initialized = true;
		clearTimeout(session.bootTimer);
		dispatch({ type: "initialized" });
		session.bridge.setHostContext(buildHostContext({ theme, containerSize }));
		flushBoundCall(session);
	});

	const handleCallTool = useEffectEvent(
		async (params: { name: string; arguments?: Record<string, unknown> }) => {
			const response = await callTool({
				name: params.name,
				arguments: params.arguments,
			});
			const parsed = CallToolResultSchema.safeParse(response.result);
			if (!parsed.success) {
				throw new Error("The MCP server returned an invalid tool result");
			}
			return parsed.data;
		},
	);

	const handleReadResource = useEffectEvent(
		async (params: { uri: string }): Promise<ReadResourceResult> => {
			const response = await readResource({ uri: params.uri });
			const parsed = ReadResourceResultSchema.safeParse(response.result);
			if (!parsed.success) {
				throw new Error("The MCP server returned an invalid resource");
			}
			return parsed.data;
		},
	);

	const handleModelContext = useEffectEvent(
		(params: Parameters<typeof flattenModelContext>[0]) => {
			store.setMcpAppContext(appId, {
				mcpServerConfigId,
				resourceUri,
				text: flattenModelContext(params),
			});
		},
	);

	const handleMessage = useEffectEvent(async (text: string) => {
		if (!onAppMessage) {
			throw new Error("This chat does not accept messages from apps");
		}
		await onAppMessage(text);
	});

	const handleOpenLink = useEffectEvent((url: string) => onOpenLink(url));

	const handleBootTimeout = useEffectEvent((session: BridgeSession) => {
		if (sessionRef.current !== session || session.initialized) {
			return;
		}
		dispatch({
			type: "fail",
			message: `The app did not start within ${BOOT_TIMEOUT_MS / 1000} seconds. Check that the wildcard access URL is configured and that ${sandboxOrigin ?? "the sandbox host"} resolves to this deployment.`,
		});
	});

	const finishClose = useEffectEvent(() => onClosed());

	const handleFailure = useEffectEvent(
		(session: BridgeSession, error: unknown) => {
			if (sessionRef.current !== session) {
				return;
			}
			dispatch({
				type: "fail",
				message:
					error instanceof Error ? error.message : "The app failed to start",
			});
		},
	);

	// Binding a new tool call to an initialized view of the same resource
	// keeps the iframe and the connection: only the per-call bookkeeping is
	// reset so the next flush sends the new call's input and result. Any other
	// change tears the current view down first, then bumps the generation so
	// the iframe reloads.
	useEffect(() => {
		if (!toolCallId) {
			return;
		}
		const bound = boundRef.current;
		if (bound?.toolCallId === toolCallId && bound.resourceUri === resourceUri) {
			return;
		}
		const session = sessionRef.current;
		if (
			bound?.resourceUri === resourceUri &&
			session?.initialized &&
			!session.tornDown
		) {
			boundRef.current = { toolCallId, resourceUri };
			session.sent = {
				input: false,
				result: false,
				cancelled: false,
				partialArgs: undefined,
			};
			dispatch({ type: "bind", reload: false });
			flushBoundCall(session);
			return;
		}
		let cancelled = false;
		const rebind = async () => {
			if (session) {
				await teardownSession(session);
			}
			if (!cancelled) {
				boundRef.current = { toolCallId, resourceUri };
				dispatch({ type: "bind", reload: true });
			}
		};
		void rebind();
		return () => {
			cancelled = true;
		};
	}, [toolCallId, resourceUri]);

	// Closing tears the view down while the panel is still mounted, then
	// reports back so the tab can be removed.
	useEffect(() => {
		if (!closing) {
			return;
		}
		let cancelled = false;
		const close = async () => {
			const session = sessionRef.current;
			if (session) {
				await teardownSession(session);
			}
			if (!cancelled) {
				finishClose();
			}
		};
		void close();
		return () => {
			cancelled = true;
		};
	}, [closing]);

	useEffect(() => {
		if (view) {
			dispatch({ type: "resourceLoaded" });
		}
	}, [view]);

	const createBridge = useEffectEvent(
		() =>
			new AppBridge(
				null,
				{ name: "coder", version: hostVersion },
				{
					serverTools: {},
					serverResources: {},
					openLinks: {},
					logging: {},
					updateModelContext: { text: {} },
					message: { text: {} },
				},
				{ hostContext: buildHostContext({ theme, containerSize }) },
			),
	);

	useEffect(() => {
		if (!transport) {
			return;
		}
		const bridge = createBridge();
		const session: BridgeSession = {
			bridge,
			initialized: false,
			tornDown: false,
			bootTimer: undefined,
			sent: {
				input: false,
				result: false,
				cancelled: false,
				partialArgs: undefined,
			},
		};
		sessionRef.current = session;

		bridge.oncalltool = (params) => handleCallTool(params);
		bridge.onreadresource = (params) => handleReadResource(params);
		bridge.onopenlink = async ({ url }) => {
			if (!isHttpUrl(url)) {
				throw new Error("Only http(s) links can be opened");
			}
			await handleOpenLink(url);
			return {};
		};
		bridge.onmessage = async ({ content }) => {
			await handleMessage(textFromContent(content));
			return {};
		};
		bridge.onupdatemodelcontext = async (params) => {
			handleModelContext(params);
			return {};
		};
		bridge.onrequestdisplaymode = async () => ({ mode: "inline" });
		bridge.onloggingmessage = (params) => {
			console.info("[mcp-app]", params.level, params.data);
		};
		bridge.addEventListener("sandboxready", () => handleSandboxReady(session));
		bridge.addEventListener("initialized", () => handleInitialized(session));

		bridge.connect(transport).catch((error: unknown) => {
			handleFailure(session, error);
		});
		session.bootTimer = setTimeout(
			() => handleBootTimeout(session),
			BOOT_TIMEOUT_MS,
		);

		return () => {
			if (sessionRef.current === session) {
				sessionRef.current = null;
			}
			void closeSession(session);
		};
	}, [transport]);

	useEffect(() => {
		const session = sessionRef.current;
		if (session) {
			flushBoundCall(session);
		}
	}, [boundCall]);

	useEffect(() => {
		const session = sessionRef.current;
		if (!session?.initialized || session.tornDown) {
			return;
		}
		session.bridge.setHostContext(buildHostContext({ theme, containerSize }));
	}, [theme, containerSize]);

	return lifecycle;
};
