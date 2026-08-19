import { InfoIcon } from "lucide-react";
import type { ComponentProps, FC, PropsWithChildren } from "react";
import type { AIBridgeSession } from "#/api/typesGenerated";
import { LoadMoreSentinel } from "#/components/LoadMoreSentinel/LoadMoreSentinel";
import { PaywallAIGovernance } from "#/components/Paywall/PaywallAIGovernance";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { AIBridgeSetupAlert } from "../AIBridgeSetupAlert";
import { ListSessionsFilter } from "./ListSessionsFilter";
import { ListSessionsRow } from "./ListSessionsRow";

interface ListSessionsPageViewProps {
	isLoading: boolean;
	isAISessionsEntitled: boolean;
	isAISessionsEnabled: boolean;
	sessions?: readonly AIBridgeSession[];
	hasNextPage: boolean;
	isFetchingNextPage: boolean;
	onFetchNextPage: () => void;
	filterProps: ComponentProps<typeof ListSessionsFilter>;
	onSessionRowClick?: (sessionId: string) => void;
}

const ThreadTooltip: FC<PropsWithChildren> = ({ children }) => (
	<TooltipProvider>
		<Tooltip>
			<TooltipTrigger asChild>{children}</TooltipTrigger>
			<TooltipContent
				side="top"
				align="end"
				className="max-w-xs text-sm font-normal"
			>
				A thread is a multi-part interaction between human and agent involving
				an initial human prompt and a subsequent agentic loop.
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

export const ListSessionsPageView: FC<ListSessionsPageViewProps> = ({
	isLoading,
	isAISessionsEntitled,
	isAISessionsEnabled,
	sessions,
	hasNextPage,
	isFetchingNextPage,
	onFetchNextPage,
	filterProps,
	onSessionRowClick,
}) => {
	if (!isAISessionsEntitled) {
		return <PaywallAIGovernance />;
	}

	if (!isAISessionsEnabled) {
		return <AIBridgeSetupAlert />;
	}

	const utcOffset = formatDateTime(new Date(), DATE_FORMAT.UTC_OFFSET);

	return (
		<>
			<ListSessionsFilter {...filterProps} />

			<Table className="text-sm font-normal">
				<TableHeader>
					<TableRow>
						<TableHead className="text-nowrap">Last Prompt</TableHead>
						<TableHead className="text-nowrap">User</TableHead>
						<TableHead className="text-nowrap">Provider</TableHead>
						<TableHead className="text-nowrap">Client</TableHead>
						<TableHead className="text-nowrap">In/Out Tokens</TableHead>
						<TableHead className="text-nowrap">Network Requests</TableHead>
						<TableHead className="flex items-center flex-nowrap gap-1">
							Threads
							<ThreadTooltip>
								<InfoIcon className="size-icon-xs" />
							</ThreadTooltip>
						</TableHead>
						<TableHead className="text-nowrap">
							Last Prompt At [UTC{utcOffset}]
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading ? (
						<TableLoader />
					) : sessions?.length === 0 ? (
						<TableEmpty message="No session logs available" />
					) : (
						sessions?.map((session) => (
							<ListSessionsRow
								session={session}
								key={session.id}
								onClick={() => onSessionRowClick?.(session.id)}
							/>
						))
					)}
				</TableBody>
			</Table>
			{hasNextPage && (
				<LoadMoreSentinel
					onLoadMore={onFetchNextPage}
					isFetchingNextPage={isFetchingNextPage}
				/>
			)}
		</>
	);
};
