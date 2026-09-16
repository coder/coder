import type { Transport } from "@modelcontextprotocol/client";
import { cn } from "cn";
import { type FC, type ReactNode, useEffect, useRef, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { chatMCPAppResource } from "#/api/queries/chats";
import type {
	ChatMCPAppResourceReadResponse,
	ChatStatus,
	MCPServerConfig,
} from "#/api/typesGenerated";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useTheme } from "#/theme/context";
import {
	type ChatStore,
	selectChatStatus,
	useChatSelector,
} from "../ChatConversation/chatStore";
import { MCPIcon } from "../MCPServerPicker";
import {
	McpAppConsentDialog,
	type McpAppConsentRequest,
} from "./McpAppConsentDialog";
import { McpAppFrame } from "./McpAppFrame";
import { deriveBoundCall } from "./mcpAppLifecycle";
import {
	buildBoundTool,
	selectDurableToolParts,
	selectStreamToolCall,
	selectStreamToolResult,
} from "./mcpAppSelectors";
import type { McpAppTab } from "./mcpAppTab";
import {
	buildSandboxUrl,
	resolveViewResource,
	sandboxHostLabel,
} from "./sandboxUrl";
import { type McpAppContainerSize, useMcpAppBridge } from "./useMcpAppBridge";

interface McpAppPanelProps {
	chatId: string;
	tab: McpAppTab;
	store: ChatStore;
	/** Used while the store has not reported a status yet. */
	chatStatus: ChatStatus;
	mcpServer: MCPServerConfig | undefined;
	/** Primary region wildcard hostname such as `*.apps.example.com`. */
	wildcardHostname: string | undefined;
	/**
	 * Sends a user message on behalf of the app after the user confirms.
	 * Undefined when the chat cannot accept messages from this user.
	 */
	submitAppMessage: ((text: string) => Promise<void>) | undefined;
	/** True once the tab close started; the view is torn down first. */
	isClosing: boolean;
	/** Called after teardown finished so the tab can be removed. */
	onClosed: () => void;
}

const PanelMessage: FC<{ message: string; description?: string }> = ({
	message,
	description,
}) => (
	<EmptyState
		isCompact
		className="h-full min-h-0 px-6"
		message={message}
		description={description}
	/>
);

const useContainerSize = (
	element: HTMLElement | null,
): McpAppContainerSize | undefined => {
	const [size, setSize] = useState<McpAppContainerSize | undefined>();
	useEffect(() => {
		if (!element) {
			return;
		}
		const observer = new ResizeObserver((entries) => {
			const rect = entries[0]?.contentRect;
			if (!rect) {
				return;
			}
			const next = {
				width: Math.round(rect.width),
				height: Math.round(rect.height),
			};
			setSize((current) =>
				current?.width === next.width && current?.height === next.height
					? current
					: next,
			);
		});
		observer.observe(element);
		return () => observer.disconnect();
	}, [element]);
	return size;
};

const useSandboxHostLabel = (
	mcpServerConfigId: string,
	chatId: string,
): string | undefined => {
	const [label, setLabel] = useState<string | undefined>();
	useEffect(() => {
		let cancelled = false;
		void sandboxHostLabel(mcpServerConfigId, chatId).then((value) => {
			if (!cancelled) {
				setLabel(value);
			}
		});
		return () => {
			cancelled = true;
		};
	}, [mcpServerConfigId, chatId]);
	return label;
};

// Module-level so react-query reuses the selected value between renders.
const selectViewResource = (data: ChatMCPAppResourceReadResponse) =>
	resolveViewResource(data.result);

const McpAppPanel: FC<McpAppPanelProps> = ({
	chatId,
	tab,
	store,
	chatStatus: fallbackChatStatus,
	mcpServer,
	wildcardHostname,
	submitAppMessage,
	isClosing,
	onClosed,
}) => {
	const theme = useTheme();
	const { buildInfo } = useDashboard();
	const serverName = mcpServer?.display_name || mcpServer?.slug || "MCP";

	const resourceQuery = useQuery({
		...chatMCPAppResource(chatId, tab.mcpServerConfigId, tab.resourceUri),
		enabled: mcpServer !== undefined,
		select: selectViewResource,
	});
	const resolved = resourceQuery.data;
	const view = resolved && !("error" in resolved) ? resolved : undefined;

	const chatStatus =
		useChatSelector(store, selectChatStatus) ?? fallbackChatStatus;
	const durable = useChatSelector(
		store,
		selectDurableToolParts(tab.toolCallId),
	);
	const streamCall = useChatSelector(
		store,
		selectStreamToolCall(tab.toolCallId),
	);
	const streamResult = useChatSelector(
		store,
		selectStreamToolResult(tab.toolCallId),
	);
	const boundTool = buildBoundTool({
		toolCallId: tab.toolCallId,
		durable,
		streamCall,
		streamResult,
		chatStatus,
	});
	const boundCall = deriveBoundCall(
		tab.toolCallId,
		boundTool ? [boundTool] : [],
		chatStatus,
	);

	const label = useSandboxHostLabel(tab.mcpServerConfigId, chatId);

	const [transport, setTransport] = useState<Transport | undefined>();
	const [container, setContainer] = useState<HTMLDivElement | null>(null);
	const containerSize = useContainerSize(container);

	// One consent request at a time. The JSON-RPC request behind it stays
	// open until the user confirms or declines, so the view receives the real
	// outcome as its response.
	const pendingHandlersRef = useRef<{
		resolve: () => void;
		reject: (error: Error) => void;
	} | null>(null);
	const [pendingRequest, setPendingRequest] =
		useState<McpAppConsentRequest | null>(null);
	const settlePendingRequest = (error?: Error) => {
		const handlers = pendingHandlersRef.current;
		pendingHandlersRef.current = null;
		setPendingRequest(null);
		if (!handlers) {
			return;
		}
		if (error) {
			handlers.reject(error);
		} else {
			handlers.resolve();
		}
	};
	const requestConsent = (request: McpAppConsentRequest) =>
		new Promise<void>((resolve, reject) => {
			if (pendingHandlersRef.current) {
				reject(new Error("Another request is awaiting confirmation"));
				return;
			}
			pendingHandlersRef.current = { resolve, reject };
			setPendingRequest(request);
		});
	const sendAppMessage = useMutation({
		mutationFn: (text: string) => {
			if (!submitAppMessage) {
				return Promise.reject(new Error("This chat is read-only."));
			}
			return submitAppMessage(text);
		},
		onSuccess: () => settlePendingRequest(),
		onError: (error: unknown) => {
			toast.error(getErrorMessage(error, "Failed to send the app message."));
			settlePendingRequest(
				error instanceof Error ? error : new Error(getErrorMessage(error, "")),
			);
		},
	});
	const confirmPendingRequest = () => {
		if (!pendingRequest) {
			return;
		}
		if (pendingRequest.kind === "open-link") {
			window.open(pendingRequest.url, "_blank", "noopener,noreferrer");
			settlePendingRequest();
			return;
		}
		sendAppMessage.mutate(pendingRequest.text);
	};
	const declinePendingRequest = () => {
		settlePendingRequest(new Error("The user declined the request"));
	};

	// Context reported by this view is only meaningful while it is shown.
	useEffect(() => {
		return () => store.clearMcpAppContext(tab.id);
	}, [store, tab.id]);

	const sandboxUrl =
		label !== undefined
			? buildSandboxUrl({ wildcardHostname, label, csp: view?.csp })
			: undefined;

	// The iframe only mounts once the resource and the bound call exist, so
	// the transport is only ever set while these hold.
	const canRender = Boolean(mcpServer && wildcardHostname && boundTool && view);
	const lifecycle = useMcpAppBridge({
		chatId,
		mcpServerConfigId: tab.mcpServerConfigId,
		resourceUri: tab.resourceUri,
		appId: tab.id,
		store,
		hostVersion: buildInfo.version || "dev",
		theme: theme.palette.mode === "dark" ? "dark" : "light",
		containerSize,
		transport: canRender ? transport : undefined,
		view,
		toolCallId: canRender ? tab.toolCallId : undefined,
		boundCall,
		sandboxOrigin: sandboxUrl ? new URL(sandboxUrl).origin : undefined,
		onAppMessage: submitAppMessage
			? (text) => requestConsent({ kind: "message", text })
			: undefined,
		onOpenLink: (url) => requestConsent({ kind: "open-link", url }),
		closing: isClosing,
		onClosed,
	});

	let body: ReactNode;
	if (!wildcardHostname) {
		body = (
			<PanelMessage
				message="MCP apps require a wildcard access URL"
				description="Ask a deployment administrator to configure CODER_WILDCARD_ACCESS_URL so app views can run on an isolated origin."
			/>
		);
	} else if (!mcpServer) {
		body = <PanelMessage message="This MCP server is no longer available." />;
	} else if (!boundTool) {
		body = (
			<PanelMessage
				message="Open a tool call for this app to load it"
				description="The tool call this app was opened from is not in the loaded conversation."
			/>
		);
	} else if (resourceQuery.isLoading || label === undefined) {
		body = (
			<div className="flex h-full items-center justify-center">
				<Spinner loading label={`Loading ${serverName} app`} />
			</div>
		);
	} else if (resourceQuery.isError) {
		body = (
			<PanelMessage
				message="Failed to load the app"
				description={getErrorMessage(
					resourceQuery.error,
					"The MCP server did not return the app resource.",
				)}
			/>
		);
	} else if (resolved && "error" in resolved) {
		body = (
			<PanelMessage
				message="This app cannot be displayed"
				description={resolved.error}
			/>
		);
	} else if (lifecycle.phase === "error") {
		body = (
			<PanelMessage
				message="The app failed to start"
				description={lifecycle.error}
			/>
		);
	} else if (sandboxUrl) {
		body = (
			<div
				ref={setContainer}
				className={cn(
					"relative min-h-0 flex-1",
					view?.prefersBorder &&
						"m-2 overflow-hidden rounded-md border border-solid border-border-default",
				)}
			>
				<McpAppFrame
					key={lifecycle.generation}
					sandboxUrl={sandboxUrl}
					title={`${serverName} app`}
					permissions={view?.permissions}
					onTransportChange={setTransport}
				/>
			</div>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default px-3 py-1.5 text-xs text-content-secondary">
				<MCPIcon
					iconUrl={mcpServer?.icon_url ?? ""}
					name={serverName}
					className="size-5"
				/>
				<span className="truncate text-content-primary">{serverName}</span>
				<span className="truncate">{tab.resourceUri}</span>
				{canRender && (isClosing || lifecycle.phase !== "initialized") && (
					<Spinner
						loading
						size="sm"
						className="ml-auto"
						label={isClosing ? "Closing app" : "Connecting to app"}
					/>
				)}
			</div>
			{body}
			<McpAppConsentDialog
				request={pendingRequest}
				serverName={serverName}
				confirmLoading={sendAppMessage.isPending}
				onConfirm={confirmPendingRequest}
				onDecline={declinePendingRequest}
			/>
		</div>
	);
};

export default McpAppPanel;
