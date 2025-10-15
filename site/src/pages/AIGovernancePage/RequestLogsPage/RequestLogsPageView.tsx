import type { AIBridgeInterception } from "api/typesGenerated";
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
import { RequestLogsRow } from "./RequestLogsRow/RequestLogsRow";

interface RequestLogsPageViewProps {
	isLoading: boolean;
	isRequestLogsVisible: boolean;
	interceptions?: readonly AIBridgeInterception[];
}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = ({
	isLoading,
	isRequestLogsVisible,
	interceptions,
}) => {
	if (!isRequestLogsVisible) {
		return (
			<Paywall
				message="AI Governance"
				description="AI Governance allows you to monitor and manage AI requests. You need an Premium license to use this feature."
				// TODO: Add documentation link
				// documentationLink={docs("/admin/security/ai-governance")}
			/>
		);
	}

	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead></TableHead>
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
	);
};
