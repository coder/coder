import { InfoIcon } from "lucide-react";
import type { ComponentProps, FC, PropsWithChildren } from "react";
import type { AIBridgeSession } from "#/api/typesGenerated";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
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
	isFetching: boolean;
	isAISessionsEntitled: boolean;
	isAISessionsEnabled: boolean;
	sessions?: readonly AIBridgeSession[];
	sessionsQuery: PaginationResult;
	filterProps: ComponentProps<typeof ListSessionsFilter>;
	onSessionRowClick?: (sessionId: string) => void;
}

const ThreadTooltip: FC<PropsWithChildren> = ({ children }) => (
	<TooltipProvider>
		<Tooltip>
			<TooltipTrigger asChild>
				<div className="flex-shrink-0 flex items-center">{children}</div>
			</TooltipTrigger>
			<TooltipContent side="top" align="end" className="max-w-xs">
				<p className="text-sm">
					A thread is a multi-part interaction between human and agent involving
					an initial human prompt and a subsequent agentic loop.
				</p>
			</TooltipContent>
		</Tooltip>
	</TooltipProvider>
);

export const ListSessionsPageView: FC<ListSessionsPageViewProps> = ({
	isLoading,
	isFetching,
	isAISessionsEntitled,
	isAISessionsEnabled,
	sessions,
	sessionsQuery,
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

			<PaginationContainer query={sessionsQuery} paginationUnitLabel="sessions">
				<Table className="text-sm">
					<TableHeader>
						<TableRow className="text-xs">
							<TableHead>Last Prompt</TableHead>
							<TableHead>User</TableHead>
							<TableHead>Provider</TableHead>
							<TableHead>Client</TableHead>
							<TableHead>In/Out Tokens</TableHead>
							<TableHead className="flex items-center gap-1">
								Threads
								<ThreadTooltip>
									<InfoIcon className="size-icon-xs" />
								</ThreadTooltip>
							</TableHead>
							<TableHead>Timestamp [UTC{utcOffset}]</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading || isFetching ? (
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
			</PaginationContainer>
		</>
	);
};
