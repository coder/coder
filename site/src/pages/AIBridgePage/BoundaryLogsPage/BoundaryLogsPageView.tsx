import type { BoundaryAuditLog } from "api/typesGenerated";
import {
	PaginationContainer,
	type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import type { ComponentProps, FC } from "react";
import { BoundaryLogsFilter } from "./filter/BoundaryLogsFilter";
import { BoundaryLogsRow } from "./BoundaryLogsRow";

interface BoundaryLogsPageViewProps {
	isLoading: boolean;
	logs?: readonly BoundaryAuditLog[];
	boundaryLogsQuery: PaginationResult;
	filterProps: ComponentProps<typeof BoundaryLogsFilter>;
}

export const BoundaryLogsPageView: FC<BoundaryLogsPageViewProps> = ({
	isLoading,
	logs,
	boundaryLogsQuery,
	filterProps,
}) => {
	return (
		<>
			<BoundaryLogsFilter {...filterProps} />

			<PaginationContainer
				query={boundaryLogsQuery}
				paginationUnitLabel="logs"
			>
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Timestamp</TableHead>
							<TableHead>Workspace</TableHead>
							<TableHead>Owner</TableHead>
							<TableHead>Agent</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Resource</TableHead>
							<TableHead>Operation</TableHead>
							<TableHead>Decision</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isLoading ? (
							<TableLoader />
						) : logs?.length === 0 ? (
							<TableEmpty message={"No boundary logs available"} />
						) : (
							logs?.map((log) => (
								<BoundaryLogsRow log={log} key={log.id} />
							))
						)}
					</TableBody>
				</Table>
			</PaginationContainer>
		</>
	);
};
