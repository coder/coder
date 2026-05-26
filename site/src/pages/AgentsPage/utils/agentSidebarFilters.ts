import type { SetURLSearchParams } from "react-router";
import {
	CHAT_LIST_PR_STATUS_ORDER,
	type ChatListPRStatusFilter,
	type ChatListStatusFilter,
	canonicalizeChatListPRStatuses,
} from "#/api/queries/chats";

export const AGENT_ARCHIVE_STATUS_ORDER = ["active", "archived"] as const;
export const AGENT_CHAT_STATUS_ORDER = [
	"unread",
	"read",
] as const satisfies readonly ChatListStatusFilter[];
export const AGENT_PR_STATUS_ORDER = CHAT_LIST_PR_STATUS_ORDER;

export type AgentArchiveStatusFilter =
	(typeof AGENT_ARCHIVE_STATUS_ORDER)[number];
export type AgentChatStatusFilter = ChatListStatusFilter;
export type AgentPRStatusFilter = ChatListPRStatusFilter;
export type AgentSidebarGroupBy = "date" | "chat_status";

export type AgentSidebarFilters = Readonly<{
	archiveStatus: AgentArchiveStatusFilter;
	groupBy: AgentSidebarGroupBy;
	prStatuses: readonly AgentPRStatusFilter[];
	chatStatuses: readonly AgentChatStatusFilter[];
}>;

type AgentSidebarFiltersResult = readonly [
	filters: AgentSidebarFilters,
	setFilters: (next: AgentSidebarFilters) => void,
];

export const DEFAULT_AGENT_SIDEBAR_FILTERS: AgentSidebarFilters = {
	archiveStatus: "active",
	groupBy: "date",
	prStatuses: [],
	chatStatuses: AGENT_CHAT_STATUS_ORDER,
};

const agentChatStatusSet = new Set<AgentChatStatusFilter>(
	AGENT_CHAT_STATUS_ORDER,
);

const canonicalizeChatStatuses = (
	values: Iterable<string>,
): readonly AgentChatStatusFilter[] => {
	const selected = new Set<AgentChatStatusFilter>();
	for (const value of values) {
		if (agentChatStatusSet.has(value as AgentChatStatusFilter)) {
			selected.add(value as AgentChatStatusFilter);
		}
	}
	return AGENT_CHAT_STATUS_ORDER.filter((status) => selected.has(status));
};

const clearSidebarFilterParams = (searchParams: URLSearchParams) => {
	searchParams.delete("archived");
	searchParams.delete("group_by");
	searchParams.delete("pr_status");
	searchParams.delete("chat_status");
};

const writeSidebarFilters = (
	searchParams: URLSearchParams,
	filters: AgentSidebarFilters,
) => {
	clearSidebarFilterParams(searchParams);

	if (filters.archiveStatus === "archived") {
		searchParams.set("archived", "archived");
	}

	if (filters.groupBy === "chat_status") {
		searchParams.set("group_by", "chat_status");
	}

	const prStatuses = canonicalizeChatListPRStatuses(filters.prStatuses);
	if (prStatuses.length > 0) {
		searchParams.set("pr_status", prStatuses.join(","));
	}

	const chatStatuses = canonicalizeChatStatuses(filters.chatStatuses);
	if (chatStatuses.length === 1) {
		searchParams.set("chat_status", chatStatuses[0]);
	}
};

export const getAgentSidebarFilters = (
	searchParams: URLSearchParams,
	setSearchParams: SetURLSearchParams,
): AgentSidebarFiltersResult => {
	const prStatuses = canonicalizeChatListPRStatuses(
		(searchParams.get("pr_status") ?? "").split(",").filter(Boolean),
	);
	const chatStatuses = canonicalizeChatStatuses(
		(searchParams.get("chat_status") ?? "").split(",").filter(Boolean),
	);

	const filters: AgentSidebarFilters = {
		archiveStatus:
			searchParams.get("archived") === "archived" ? "archived" : "active",
		groupBy:
			searchParams.get("group_by") === "chat_status"
				? "chat_status"
				: DEFAULT_AGENT_SIDEBAR_FILTERS.groupBy,
		prStatuses,
		chatStatuses:
			chatStatuses.length > 0
				? chatStatuses
				: DEFAULT_AGENT_SIDEBAR_FILTERS.chatStatuses,
	};

	const setFilters = (next: AgentSidebarFilters) => {
		setSearchParams(
			(prev) => {
				const updated = new URLSearchParams(prev);
				writeSidebarFilters(updated, next);
				return updated;
			},
			{ replace: true },
		);
	};

	return [filters, setFilters];
};
