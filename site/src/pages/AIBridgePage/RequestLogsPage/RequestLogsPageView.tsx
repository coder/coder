import type { ComponentProps, FC } from "react";
import { docs } from "utils/docs";
import type { AIBridgeInterception } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
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
import { RequestLogsFilter } from "./RequestLogsFilter/RequestLogsFilter";
import { RequestLogsRow } from "./RequestLogsRow/RequestLogsRow";

interface RequestLogsPageViewProps {
	isLoading: boolean;
	isRequestLogsEntitled: boolean;
	isRequestLogsEnabled: boolean;
	interceptions?: readonly AIBridgeInterception[];
	interceptionsQuery: PaginationResult;
	filterProps: ComponentProps<typeof RequestLogsFilter>;
}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = ({
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
							<TableHead>Timestamp</TableHead>
							<TableHead>Initiator</TableHead>
							<TableHead>Tokens</TableHead>
							<TableHead>Client</TableHead>
							<TableHead>Model</TableHead>
							<TableHead>Tool Calls</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableLoader />
						) : interceptions?.length === 0 ? (
							<TableEmpty message="No request logs available" />
						) : (
							interceptions?.map((interception) => (
								<RequestLogsRow
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
