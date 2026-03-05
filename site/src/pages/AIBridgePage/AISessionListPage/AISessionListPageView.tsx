import type { AIBridgeInterception } from "api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import {
	PaginationContainer,
	type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import { PaywallAIGovernance } from "components/Paywall/PaywallAIGovernance";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { InfoIcon } from "lucide-react";
import type { ComponentProps, FC } from "react";
import { docs } from "utils/docs";
import { DATE_FORMAT, formatDateTime } from "utils/time";
import { RequestLogsFilter } from "../RequestLogsPage/RequestLogsFilter/RequestLogsFilter";
import { AISessionListRow } from "./AISessionListRow/AISessionListRow";

export interface AISessionListPageViewProps {
	isLoading: boolean;
	isRequestLogsEntitled: boolean;
	isRequestLogsEnabled: boolean;
	interceptions?: readonly AIBridgeInterception[];
	interceptionsQuery: PaginationResult;
	filterProps: ComponentProps<typeof RequestLogsFilter>;
}

export const AISessionListPageView: FC<AISessionListPageViewProps> = ({
	isLoading,
	isRequestLogsEntitled,
	isRequestLogsEnabled,
	interceptions,
	interceptionsQuery,
	filterProps,
}) => {
	if (!isRequestLogsEntitled) {
		return <PaywallAIGovernance />;
	}

	if (!isRequestLogsEnabled) {
		return (
			<Alert className="mb-12" severity="warning" prominent>
				<AlertTitle>
					AI Bridge is included in your license, but not set up yet.
				</AlertTitle>
				<AlertDescription>
					You have access to AI Governance, but it still needs to be setup.
					Check out the{" "}
					<Link href={docs("/ai-coder/ai-bridge")} target="_blank">
						AI Bridge
					</Link>{" "}
					documentation to get started.
				</AlertDescription>
			</Alert>
		);
	}

	const utcOffset = formatDateTime(new Date(), DATE_FORMAT.UTC_OFFSET);

	return (
		<>
			<RequestLogsFilter {...filterProps} />

			<PaginationContainer
				query={interceptionsQuery}
				paginationUnitLabel="interceptions"
			>
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
								<div className="min-w-0">
									<TooltipProvider>
										<Tooltip>
											<TooltipTrigger asChild>
												<div className="flex-shrink-0 flex items-center">
													<InfoIcon className="size-icon-xs" />
												</div>
											</TooltipTrigger>
											<TooltipContent
												side="top"
												align="end"
												className="max-w-xs"
											>
												<p className="text-sm">
													A thread is a multi-part interaction between human and
													agent involving an initial human prompt and a
													subsequent agentic loop.
												</p>
												<p>
													<Link href="TODO docs page" target="_blank">
														View session terminology
													</Link>
												</p>
											</TooltipContent>
										</Tooltip>
									</TooltipProvider>
								</div>
							</TableHead>
							<TableHead>Timestamp [UTC{utcOffset}]</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableLoader />
						) : interceptions?.length === 0 ? (
							<TableEmpty message="No session logs available" />
						) : (
							interceptions?.map((interception) => (
								<AISessionListRow
									interception={interception}
									key={interception.id}
								/>
							))
						)}
					</TableBody>
				</Table>
			</PaginationContainer>
		</>
	);
};
