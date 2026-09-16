import type * as TypesGen from "#/api/typesGenerated";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import type { StreamState } from "../ChatConversation/types";

export type McpAppTab = Extract<UserRightPanelTab, { kind: "mcp_app" }>;

export const mcpAppTabId = (
	mcpServerConfigId: string,
	resourceUri: string,
): string => `mcp_app:${mcpServerConfigId}:${resourceUri}`;

/** A tool call that renders an MCP App, in transcript order. */
export type McpAppToolCallRef = {
	toolCallId: string;
	mcpServerConfigId: string;
	resourceUri: string;
	toolName: string;
};

const collectFromParts = (
	parts: readonly TypesGen.ChatMessagePart[] | undefined,
	out: McpAppToolCallRef[],
): void => {
	for (const part of parts ?? []) {
		if (
			part.type === "tool-call" &&
			part.tool_call_id &&
			part.mcp_server_config_id &&
			part.mcp_app_resource_uri
		) {
			out.push({
				toolCallId: part.tool_call_id,
				mcpServerConfigId: part.mcp_server_config_id,
				resourceUri: part.mcp_app_resource_uri,
				toolName: part.tool_name || "Tool",
			});
		}
	}
};

const durableRefsCache = new WeakMap<
	Map<number, TypesGen.ChatMessage>,
	McpAppToolCallRef[]
>();

const combinedRefsCache = new WeakMap<
	StreamState,
	WeakMap<McpAppToolCallRef[], McpAppToolCallRef[]>
>();

/**
 * Lists every tool call with an app resource across persisted messages and
 * the live stream, oldest first. Results are cached per input identity so
 * the returned array is referentially stable between store emissions that
 * did not change either input, which `useSyncExternalStore` requires.
 */
export const collectMcpAppToolCalls = (
	messagesByID: Map<number, TypesGen.ChatMessage>,
	orderedMessageIDs: readonly number[],
	streamState: StreamState | null,
): McpAppToolCallRef[] => {
	let durable = durableRefsCache.get(messagesByID);
	if (!durable) {
		durable = [];
		for (const messageID of orderedMessageIDs) {
			collectFromParts(messagesByID.get(messageID)?.content, durable);
		}
		durableRefsCache.set(messagesByID, durable);
	}
	if (!streamState) {
		return durable;
	}
	let byDurable = combinedRefsCache.get(streamState);
	if (!byDurable) {
		byDurable = new WeakMap();
		combinedRefsCache.set(streamState, byDurable);
	}
	const cached = byDurable.get(durable);
	if (cached) {
		return cached;
	}
	const seen = new Set(durable.map((ref) => ref.toolCallId));
	const live: McpAppToolCallRef[] = [];
	for (const call of Object.values(streamState.toolCalls)) {
		if (
			call.mcpServerConfigId &&
			call.mcpAppResourceUri &&
			!seen.has(call.id)
		) {
			live.push({
				toolCallId: call.id,
				mcpServerConfigId: call.mcpServerConfigId,
				resourceUri: call.mcpAppResourceUri,
				toolName: call.name,
			});
		}
	}
	const combined = live.length === 0 ? durable : [...durable, ...live];
	byDurable.set(durable, combined);
	return combined;
};

type McpAppTabPlan = {
	/** Tabs to create or rebind, keyed by tab ID. */
	upserts: McpAppTab[];
	/** Tab to select, when a brand-new app appeared while the panel was open. */
	activateTabId?: string;
	/** Tabs that received a new tool call without being selected. */
	badgeTabIds: string[];
};

/**
 * Decides how the tab list reacts to the app tool calls in the transcript.
 * Each app keeps one tab bound to its newest tool call. A tab the user closed
 * stays closed until a different tool call arrives for that app. Calls that
 * were already in history when the page loaded (`initial`) never badge or
 * select a tab, and the panel is never opened.
 */
export const planMcpAppTabs = ({
	refs,
	tabs,
	dismissed,
	initial,
	panelOpen,
	activeTabId,
	labelFor,
}: {
	refs: readonly McpAppToolCallRef[];
	tabs: readonly UserRightPanelTab[];
	dismissed: ReadonlyMap<string, string>;
	initial: boolean;
	panelOpen: boolean;
	activeTabId: string | null;
	labelFor: (ref: McpAppToolCallRef) => string;
}): McpAppTabPlan => {
	const newestByTab = new Map<string, McpAppToolCallRef>();
	for (const ref of refs) {
		newestByTab.set(mcpAppTabId(ref.mcpServerConfigId, ref.resourceUri), ref);
	}
	const plan: McpAppTabPlan = { upserts: [], badgeTabIds: [] };
	for (const [tabId, ref] of newestByTab) {
		if (dismissed.get(tabId) === ref.toolCallId) {
			continue;
		}
		const existing = tabs.find(
			(tab): tab is McpAppTab => tab.kind === "mcp_app" && tab.id === tabId,
		);
		if (existing?.toolCallId === ref.toolCallId) {
			continue;
		}
		plan.upserts.push({
			id: tabId,
			kind: "mcp_app",
			mcpServerConfigId: ref.mcpServerConfigId,
			resourceUri: ref.resourceUri,
			toolCallId: ref.toolCallId,
			label: existing?.label ?? labelFor(ref),
		});
		if (initial) {
			continue;
		}
		if (!existing && panelOpen && plan.activateTabId === undefined) {
			plan.activateTabId = tabId;
		} else if (activeTabId !== tabId) {
			plan.badgeTabIds.push(tabId);
		}
	}
	return plan;
};

/** Applies planned upserts to the tab list, replacing existing app tabs in place. */
export const applyMcpAppTabUpserts = (
	tabs: readonly UserRightPanelTab[],
	upserts: readonly McpAppTab[],
): UserRightPanelTab[] => {
	if (upserts.length === 0) {
		return [...tabs];
	}
	const byId = new Map(upserts.map((tab) => [tab.id, tab]));
	const next = tabs.map((tab) => byId.get(tab.id) ?? tab);
	const existingIds = new Set(tabs.map((tab) => tab.id));
	for (const tab of upserts) {
		if (!existingIds.has(tab.id)) {
			next.push(tab);
		}
	}
	return next;
};
