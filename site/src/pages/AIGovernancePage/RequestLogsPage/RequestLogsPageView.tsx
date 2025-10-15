import type { AIBridgeInterception } from "api/typesGenerated";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import { RequestLogsRow } from "./RequestLogsRow/RequestLogsRow";

interface RequestLogsPageViewProps {
	interceptions?: readonly AIBridgeInterception[];
}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = ({
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
					<TableHead>Status</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{interceptions?.map((interception) => (
					<RequestLogsRow interception={interception} key={interception.id} />
				))}
			</TableBody>
		</Table>
	);
};
