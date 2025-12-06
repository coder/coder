import type { BoundaryAuditLog } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import {
	PaginationContainer,
	type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableLoader } from "components/TableLoader/TableLoader";

import type { ComponentProps, FC } from "react";
import { docs } from "utils/docs";
import { BoundaryLogsFilter } from "./BoundaryLogsFilter";
import { BoundaryLogsHelpTooltip } from "./BoundaryLogsHelpTooltip";

const Language = {
	title: "Boundary Logs",
	subtitle: "View resource access events from Boundary in your workspaces.",
};

interface BoundaryLogsPageViewProps {
	logs?: readonly BoundaryAuditLog[];
	isNonInitialPage: boolean;
	isBoundaryLogsVisible: boolean;
	error?: unknown;
	filterProps: ComponentProps<typeof BoundaryLogsFilter>;
	boundaryLogsQuery: PaginationResult;
	showOrgDetails: boolean;
}

export const BoundaryLogsPageView: FC<BoundaryLogsPageViewProps> = ({
	logs,
	isNonInitialPage,
	isBoundaryLogsVisible,
	error,
	filterProps,
	boundaryLogsQuery: paginationResult,
	showOrgDetails,
}) => {
	const isLoading =
		(logs === undefined || paginationResult.totalRecords === undefined) &&
		!error;

	const isEmpty = !isLoading && logs?.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<Stack direction="row" spacing={1} alignItems="center">
						<span>{Language.title}</span>
						<BoundaryLogsHelpTooltip />
					</Stack>
				</PageHeaderTitle>
				<PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
			</PageHeader>

			<ChooseOne>
				<Cond condition={isBoundaryLogsVisible}>
					<BoundaryLogsFilter {...filterProps} />

					<PaginationContainer
						query={paginationResult}
						paginationUnitLabel="logs"
					>
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Timestamp</TableHead>
									<TableHead>Workspace</TableHead>
									<TableHead>Owner</TableHead>
									<TableHead>Agent</TableHead>
									<TableHead>Resource</TableHead>
									<TableHead>Decision</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								<ChooseOne>
									{/* Error condition should just show an empty table. */}
									<Cond condition={Boolean(error)}>
										<TableRow>
											<TableCell colSpan={999}>
												<EmptyState message="An error occurred while loading boundary logs" />
											</TableCell>
										</TableRow>
									</Cond>

									<Cond condition={isLoading}>
										<TableLoader />
									</Cond>

									<Cond condition={isEmpty}>
										<ChooseOne>
											<Cond condition={isNonInitialPage}>
												<TableRow>
													<TableCell colSpan={999}>
														<EmptyState message="No boundary logs available on this page" />
													</TableCell>
												</TableRow>
											</Cond>

											<Cond>
												<TableRow>
													<TableCell colSpan={999}>
														<EmptyState message="No boundary logs available" />
													</TableCell>
												</TableRow>
											</Cond>
										</ChooseOne>
									</Cond>

									<Cond>
										{logs?.map((log) => (
											<BoundaryLogRow
												key={log.id}
												log={log}
												showOrgDetails={showOrgDetails}
											/>
										))}
									</Cond>
								</ChooseOne>
							</TableBody>
						</Table>
					</PaginationContainer>
				</Cond>

				<Cond>
					<Paywall
						message="Boundary Logs"
						description="Boundary logs allow you to monitor resource access by AI agents in your workspaces. You need a Premium license to use this feature."
						documentationLink={docs("/admin/security/audit-logs")}
					/>
				</Cond>
			</ChooseOne>
		</Margins>
	);
};

interface BoundaryLogRowProps {
	log: BoundaryAuditLog;
	showOrgDetails: boolean;
}

const BoundaryLogRow: FC<BoundaryLogRowProps> = ({ log, showOrgDetails }) => {
	return (
		<TableRow data-testid={`boundary-log-${log.id}`}>
			<TableCell className="whitespace-nowrap">
				{new Date(log.time).toLocaleString()}
			</TableCell>
			<TableCell>{log.workspace_name}</TableCell>
			<TableCell>{log.workspace_owner_username}</TableCell>
			<TableCell>{log.agent_name}</TableCell>
			<TableCell className="font-mono text-sm max-w-md truncate" title={log.resource}>
				{log.resource}
			</TableCell>
			<TableCell>
				<Badge variant={log.decision === "allow" ? "green" : "destructive"}>
					{log.decision}
				</Badge>
			</TableCell>
		</TableRow>
	);
};
