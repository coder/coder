import type { ChatCostSummary } from "api/typesGenerated";
import { BarChart3Icon } from "lucide-react";
import type { FC } from "react";
import { ChatCostSummaryView } from "./components/ChatCostSummaryView";
import { SectionHeader } from "./components/SectionHeader";

interface AgentAnalyticsPageViewProps {
	summary: ChatCostSummary | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	rangeLabel: string;
}

export const AgentAnalyticsPageView: FC<AgentAnalyticsPageViewProps> = ({
	summary,
	isLoading,
	error,
	onRetry,
	rangeLabel,
}) => {
	return (
		<div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 pt-8 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
			<div className="mx-auto w-full max-w-3xl">
				<SectionHeader
					label="Analytics"
					description="Review your personal chat usage and cost breakdowns."
					action={
						<div className="flex items-center gap-2 text-xs text-content-secondary">
							<BarChart3Icon className="h-4 w-4" />
							<span>{rangeLabel}</span>
						</div>
					}
				/>

				<ChatCostSummaryView
					summary={summary}
					isLoading={isLoading}
					error={error}
					onRetry={onRetry}
					loadingLabel="Loading analytics"
					emptyMessage="No usage data for you in this period."
				/>
			</div>
		</div>
	);
};
