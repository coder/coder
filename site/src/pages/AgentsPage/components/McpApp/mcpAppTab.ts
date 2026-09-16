import type * as TypesGen from "#/api/typesGenerated";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import type { StreamState } from "../ChatConversation/types";

export type McpAppTab = Extract<UserRightPanelTab, { kind: "mcp_app" }>;

export const mcpAppTabId = (
	mcpServerConfigId: string,
	resourceUri: string,
): string => `mcp_app:${mcpServerConfigId}:${resourceUri}`;

/**
 * Turns the last path segment of a resource URI into a readable name:
 * `ui://taskboard/task-board` becomes "Task board". Returns the whole URI
 * when it has no non-empty segment.
 */
export const humanizeResourceUri = (resourceUri: string): string => {
	const segment = resourceUri
		.split(/[?#]/)[0]
		.split("/")
		.filter((part) => part !== "")
		.at(-1);
	if (!segment || segment.endsWith(":")) {
		return resourceUri;
	}
	const words = segment.replace(/[-_]+/g, " ").trim();
	if (words === "") {
		return resourceUri;
	}
	return words.charAt(0).toUpperCase() + words.slice(1);
};

/**
 * Label for an app tab, derived at render time so server renames apply.
 * Named after the server; when the server has other app tabs open
 * (`siblingCount` above one) the resource name is appended to tell them
 * apart. Falls back to the resource name when the server is unknown.
 */
export const mcpAppTabLabel = ({
	server,
	resourceUri,
	siblingCount,
}: {
	server: Pick<TypesGen.MCPServerConfig, "display_name" | "slug"> | undefined;
	resourceUri: string;
	/** Open app tabs for the same server, including this one. */
	siblingCount: number;
}): string => {
	const serverName = server?.display_name || server?.slug;
	if (!serverName) {
		return humanizeResourceUri(resourceUri);
	}
	return siblingCount > 1
		? `${serverName}: ${humanizeResourceUri(resourceUri)}`
		: serverName;
};

/** A tool call that renders an MCP App, in transcript order. */
export type McpAppToolCallRef = {
	toolCallId: string;
	mcpServerConfigId: string;
	resourceUri: string;
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
			});
		}
	}
	const combined = live.length === 0 ? durable : [...durable, ...live];
	byDurable.set(durable, combined);
	return combined;
};

type McpAppTabPlan = {
	/** Tabs to create or rebind. */
	upserts: McpAppTab[];
	/** Tabs that received a new tool call while not being the selected tab. */
	badgeTabIds: string[];
	/** Dismissals whose tool call is gone or superseded by a newer call. */
	expiredDismissals: string[];
	/** Newest tool call per tab in this transcript; pass back as `seen` next time. */
	seen: Map<string, string>;
};

/**
 * Decides how the tab list reacts to the app tool calls in the transcript.
 * A tab is created for an app that has none, bound to the app's newest call.
 * An existing tab is rebound only when a call arrives that was not in the
 * previous transcript (`seen`), so an explicit older binding survives until
 * the model calls the tool again. Dismissed tabs stay closed while their
 * dismissed call is still the newest. The history loaded with the page
 * (`initial`) only creates missing tabs and never badges, and nothing here
 * selects a tab: a tab whose id is already the selected id becomes visible
 * on its own.
 */
export const planMcpAppTabs = ({
	refs,
	tabs,
	dismissed,
	seen,
	initial,
	selectedTabId,
}: {
	refs: readonly McpAppToolCallRef[];
	tabs: readonly UserRightPanelTab[];
	dismissed: ReadonlyMap<string, string>;
	seen: ReadonlyMap<string, string>;
	initial: boolean;
	selectedTabId: string | null;
}): McpAppTabPlan => {
	const newestByTab = new Map<string, McpAppToolCallRef>();
	for (const ref of refs) {
		newestByTab.set(mcpAppTabId(ref.mcpServerConfigId, ref.resourceUri), ref);
	}
	const plan: McpAppTabPlan = {
		upserts: [],
		badgeTabIds: [],
		expiredDismissals: [],
		seen: new Map(
			Array.from(newestByTab, ([tabId, ref]) => [tabId, ref.toolCallId]),
		),
	};
	for (const [tabId, dismissedCall] of dismissed) {
		if (newestByTab.get(tabId)?.toolCallId !== dismissedCall) {
			plan.expiredDismissals.push(tabId);
		}
	}
	for (const [tabId, ref] of newestByTab) {
		if (dismissed.get(tabId) === ref.toolCallId) {
			continue;
		}
		const existing = tabs.find(
			(tab): tab is McpAppTab => tab.kind === "mcp_app" && tab.id === tabId,
		);
		const isNewCall = !initial && seen.get(tabId) !== ref.toolCallId;
		if (existing && (!isNewCall || existing.toolCallId === ref.toolCallId)) {
			continue;
		}
		plan.upserts.push({
			id: tabId,
			kind: "mcp_app",
			mcpServerConfigId: ref.mcpServerConfigId,
			resourceUri: ref.resourceUri,
			toolCallId: ref.toolCallId,
		});
		if (!initial && selectedTabId !== tabId) {
			plan.badgeTabIds.push(tabId);
		}
	}
	return plan;
};

/** Applies planned upserts to the tab list, replacing existing app tabs in place. */
export const applyMcpAppTabUpserts = (
	tabs: UserRightPanelTab[],
	upserts: readonly McpAppTab[],
): UserRightPanelTab[] => {
	if (upserts.length === 0) {
		return tabs;
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
