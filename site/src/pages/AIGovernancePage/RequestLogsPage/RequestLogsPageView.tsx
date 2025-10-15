import type { AIBridgeInterception } from "api/typesGenerated";
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
	interceptions?: readonly AIBridgeInterception[];
}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = ({
	isLoading,
	interceptions,
}) => {
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
