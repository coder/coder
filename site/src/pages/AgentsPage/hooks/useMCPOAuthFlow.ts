import { useEffect, useEffectEvent, useRef, useState } from "react";
import { mcpServerOAuth2ConnectPath } from "#/api/api";
import type { MCPServerConfig } from "#/api/typesGenerated";

type UseMCPOAuthFlowOptions = {
	organizationId?: string;
	// Called for every same-origin completion message, regardless of
	// which flow (if any) produced it, so callers can refresh server
	// state.
	onAuthComplete?: (serverId: string) => void;
	// Called only for a completion posted by the initiating popup for
	// the initiating server.
	onFlowSuccess: (server: MCPServerConfig) => void;
};

type MCPOAuthFlow = {
	connectingServerId: string | null;
	connect: (server: MCPServerConfig) => void;
};

/**
 * Runs the MCP server OAuth2 popup flow: opens the consent popup,
 * listens for the completion message coderd's callback page posts to
 * the opener, and reports success only for the initiating popup and
 * server.
 */
export const useMCPOAuthFlow = ({
	organizationId,
	onAuthComplete,
	onFlowSuccess,
}: UseMCPOAuthFlowOptions): MCPOAuthFlow => {
	const [connectingServerId, setConnectingServerId] = useState<string | null>(
		null,
	);
	// Correlates a completion message with the initiating flow.
	// Retained after popup close: the callback page posts before
	// closing, and the close poll can run before the queued message is
	// dispatched.
	const flowRef = useRef<{ popup: Window; server: MCPServerConfig } | null>(
		null,
	);

	const handleAuthComplete = useEffectEvent(
		(serverID: string, source: MessageEventSource | null) => {
			onAuthComplete?.(serverID);
			// Only a message from the initiating popup for the initiating
			// server counts as flow success.
			const flow = flowRef.current;
			if (!flow || source !== flow.popup || serverID !== flow.server.id) {
				return;
			}
			flowRef.current = null;
			setConnectingServerId(null);
			onFlowSuccess(flow.server);
		},
	);

	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (event.origin !== location.origin) return;
			if (
				event.data?.type === "mcp-oauth2-complete" &&
				typeof event.data.serverID === "string"
			) {
				handleAuthComplete(event.data.serverID, event.source);
			}
		};
		window.addEventListener("message", handler);
		return () => window.removeEventListener("message", handler);
	}, []);

	// Clear only the connecting indicator when the popup closes; the
	// flow ref stays so a completion message posted before close still
	// correlates.
	useEffect(() => {
		if (!connectingServerId || !flowRef.current) return;
		const interval = setInterval(() => {
			if (flowRef.current?.popup.closed) {
				setConnectingServerId(null);
			}
		}, 500);
		return () => {
			clearInterval(interval);
			const popup = flowRef.current?.popup;
			if (popup && !popup.closed) {
				popup.close();
				flowRef.current = null;
			}
		};
	}, [connectingServerId]);

	const connect = (server: MCPServerConfig) => {
		if (!organizationId) {
			return;
		}
		const connectUrl = mcpServerOAuth2ConnectPath(organizationId, server.id);
		const popup = window.open(connectUrl, "_blank", "width=900,height=600");
		// A blocked popup (window.open returns null) must not enter the
		// connecting state; nothing would ever clear it.
		flowRef.current = popup ? { popup, server } : null;
		setConnectingServerId(popup ? server.id : null);
	};

	return { connectingServerId, connect };
};
