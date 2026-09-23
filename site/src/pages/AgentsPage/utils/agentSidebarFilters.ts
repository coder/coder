import type { SetURLSearchParams } from "react-router";
import {
	CHAT_LIST_PR_STATUS_ORDER,
	CHAT_SOURCE_ORDER,
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
const AGENT_SOURCE_ORDER = CHAT_SOURCE_ORDER;
export const AGENT_TIME_RANGE_ORDER = [
	"1d",
	"7d",
	"15d",
	"30d",
	"all",
] as const;

export type AgentArchiveStatusFilter =
	(typeof AGENT_ARCHIVE_STATUS_ORDER)[number];
type AgentChatStatusFilter = ChatListStatusFilter;
export type AgentPRStatusFilter = ChatListPRStatusFilter;
export type AgentSidebarGroupBy = "date" | "chat_status";
export type AgentSourceFilter = (typeof AGENT_SOURCE_ORDER)[number];
export type AgentTimeRangeFilter = (typeof AGENT_TIME_RANGE_ORDER)[number];

/**
 * `timeRange` is URL state only; the chats search API has no matching term,
 * so it is not applied to the chat list query.
 */
export type AgentSidebarFilters = Readonly<{
	archiveStatus: AgentArchiveStatusFilter;
	groupBy: AgentSidebarGroupBy;
	prStatuses: readonly AgentPRStatusFilter[];
	chatStatuses: readonly AgentChatStatusFilter[];
	sources: readonly AgentSourceFilter[];
	timeRange: AgentTimeRangeFilter;
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
	sources: ["created_by_me", "shared_with_me"],
	timeRange: "all",
};

const clearSidebarFilterParams = (searchParams: URLSearchParams) => {
	searchParams.delete("archived");
	searchParams.delete("group_by");
	searchParams.delete("pr_status");
	searchParams.delete("chat_status");
	searchParams.delete("source");
	searchParams.delete("time_range");
};

const isTimeRange = (value: string): value is AgentTimeRangeFilter =>
	AGENT_TIME_RANGE_ORDER.some((range) => range === value);

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

	if (filters.prStatuses.length > 0) {
		searchParams.set("pr_status", filters.prStatuses.join(","));
	}

	if (filters.chatStatuses.length === 1) {
		searchParams.set("chat_status", filters.chatStatuses[0]);
	}

	if (
		filters.sources.length !== DEFAULT_AGENT_SIDEBAR_FILTERS.sources.length ||
		filters.sources.some(
			(source) => !DEFAULT_AGENT_SIDEBAR_FILTERS.sources.includes(source),
		)
	) {
		searchParams.set("source", filters.sources.join(","));
	}

	if (filters.timeRange !== DEFAULT_AGENT_SIDEBAR_FILTERS.timeRange) {
		searchParams.set("time_range", filters.timeRange);
	}
};

export const getAgentSidebarFilters = (
	searchParams: URLSearchParams,
	setSearchParams: SetURLSearchParams,
): AgentSidebarFiltersResult => {
	const prStatuses = canonicalizeChatListPRStatuses(
		(searchParams.get("pr_status") ?? "").split(",").filter(Boolean),
	);
	const rawChatStatuses = (searchParams.get("chat_status") ?? "")
		.split(",")
		.filter(Boolean);
	const chatStatuses = AGENT_CHAT_STATUS_ORDER.filter((status) =>
		rawChatStatuses.includes(status),
	);
	const rawSources = (searchParams.get("source") ?? "")
		.split(",")
		.filter(Boolean);
	const sources = AGENT_SOURCE_ORDER.filter((source) =>
		rawSources.includes(source),
	);
	const rawTimeRange = searchParams.get("time_range") ?? "";

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
		sources:
			sources.length > 0 ? sources : DEFAULT_AGENT_SIDEBAR_FILTERS.sources,
		timeRange: isTimeRange(rawTimeRange)
			? rawTimeRange
			: DEFAULT_AGENT_SIDEBAR_FILTERS.timeRange,
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
