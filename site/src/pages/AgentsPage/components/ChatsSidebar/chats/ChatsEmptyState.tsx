import type { FC } from "react";
import {
	AGENT_CHAT_STATUS_ORDER,
	type AgentSidebarFilters,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";

/** True when a result filter (status, PR status, or source) narrows the list. */
export const hasAppliedResultFilters = (
	filters: AgentSidebarFilters,
): boolean =>
	filters.prStatuses.length > 0 ||
	filters.chatStatuses.length !== AGENT_CHAT_STATUS_ORDER.length ||
	filters.sources.length !== DEFAULT_AGENT_SIDEBAR_FILTERS.sources.length ||
	filters.sources.some(
		(source) => !DEFAULT_AGENT_SIDEBAR_FILTERS.sources.includes(source),
	);

const emptyStateMessage = (filters: AgentSidebarFilters): string =>
	hasAppliedResultFilters(filters)
		? "No agents match these filters"
		: filters.archiveStatus === "archived"
			? "No archived agents"
			: "No agents yet";

/** The same filters with every result filter reset; archive status is kept. */
const clearedResultFilters = (
	filters: AgentSidebarFilters,
): AgentSidebarFilters => ({
	...filters,
	prStatuses: [],
	chatStatuses: AGENT_CHAT_STATUS_ORDER,
	sources: DEFAULT_AGENT_SIDEBAR_FILTERS.sources,
});

interface ChatsEmptyStateProps {
	readonly filters: AgentSidebarFilters;
	readonly onFiltersChange: (filters: AgentSidebarFilters) => void;
}

export const ChatsEmptyState: FC<ChatsEmptyStateProps> = ({
	filters,
	onFiltersChange,
}) => (
	<div className="rounded-lg border border-dashed border-border-default bg-surface-primary p-4 text-center text-xs text-content-secondary">
		<p className="m-0">{emptyStateMessage(filters)}</p>
		{hasAppliedResultFilters(filters) && (
			<button
				type="button"
				className="mt-2 cursor-pointer border-none bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary hover:underline"
				onClick={() => onFiltersChange(clearedResultFilters(filters))}
			>
				Clear filters
			</button>
		)}
	</div>
);
