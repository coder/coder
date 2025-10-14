import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import { RequestLogsRow } from "./RequestLogsRow/RequestLogsRow";

// biome-ignore lint/suspicious/noEmptyInterface: TODO
interface RequestLogsPageViewProps {}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = () => {
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
				{new Array(5).fill(0).map((_, x) => (
					<RequestLogsRow key={x} />
				))}
			</TableBody>
		</Table>
	);
};
