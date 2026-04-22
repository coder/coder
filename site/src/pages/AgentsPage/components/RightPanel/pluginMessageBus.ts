/**
 * Message bus protocol for Coder agent UI plugins.
 *
 * Plugins run in sandboxed iframes and communicate with the host
 * via postMessage. This file defines the message types and a
 * host-side helper to validate incoming messages.
 */

// ─── Host → Plugin Messages ───────────────────────────────────

interface PluginInitMessage {
	type: "coder-plugin:init";
	payload: {
		apiUrl: string;
		pluginToken: string;
		workspaceId: string;
		agentId: string;
		chatId: string;
		pluginSlug: string;
	};
}

interface PluginTokenRefreshMessage {
	type: "coder-plugin:token-refresh";
	payload: {
		pluginToken: string;
	};
}

interface PluginPortResponseMessage {
	type: "coder-plugin:port-response";
	payload: {
		port: number;
		url: string;
		requestId: string;
	};
}

type HostToPluginMessage =
	| PluginInitMessage
	| PluginTokenRefreshMessage
	| PluginPortResponseMessage;

// ─── Plugin → Host Messages ───────────────────────────────────

interface PluginReadyMessage {
	type: "coder-plugin:ready";
}

interface PluginPortRequestMessage {
	type: "coder-plugin:port-request";
	payload: {
		port: number;
		requestId: string;
	};
}

type PluginToHostMessage = PluginReadyMessage | PluginPortRequestMessage;

// ─── Aggregate + Context ──────────────────────────────────────

// All message types (used internally for type guards).
type PluginMessage = HostToPluginMessage | PluginToHostMessage;

export interface PluginContext {
	apiUrl: string;
	workspaceId: string;
	agentId: string;
	chatId: string;
}

// ─── Host-Side Helpers ────────────────────────────────────────

export function isPluginMessage(data: unknown): data is PluginMessage {
	return (
		typeof data === "object" &&
		data !== null &&
		"type" in data &&
		typeof (data as { type: unknown }).type === "string" &&
		(data as { type: string }).type.startsWith("coder-plugin:")
	);
}
