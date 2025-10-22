import type { AIBridgeInterception } from "api/typesGenerated";
import {
	PaginationContainer,
	type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import { Paywall } from "components/Paywall/Paywall";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import type { FC } from "react";
import { docs } from "utils/docs";
import { RequestLogsRow } from "./RequestLogsRow/RequestLogsRow";

interface RequestLogsPageViewProps {
	isLoading: boolean;
	isRequestLogsVisible: boolean;
	interceptions?: readonly AIBridgeInterception[];
	interceptionsQuery: PaginationResult;
}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = ({
	isLoading,
	isRequestLogsVisible,
	interceptions,
	interceptionsQuery,
}) => {
	if (!isRequestLogsVisible) {
		return (
			<Paywall
				message="AI Governance"
				description="AI Governance allows you to monitor and manage AI requests. You need an Premium license to use this feature."
				documentationLink={docs("/ai-coder/ai-bridge")}
			/>
		);
	}

	return (
		<PaginationContainer
			query={interceptionsQuery}
			paginationUnitLabel="interceptions"
		>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Timestamp</TableHead>
						<TableHead>User</TableHead>
						<TableHead>Prompt</TableHead>
						<TableHead>Tokens</TableHead>
						<TableHead>Tool Calls</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading && <TableLoader />}
					{interceptions?.length === 0 && (
						<TableEmpty message={"No request logs available"} />
					)}
					{interceptions?.map((interception) => (
						<RequestLogsRow interception={interception} key={interception.id} />
					))}
				</TableBody>
			</Table>
		</PaginationContainer>
	);
};
