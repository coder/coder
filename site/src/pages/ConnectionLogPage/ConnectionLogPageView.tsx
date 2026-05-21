import type { ComponentProps, FC } from "react";
import type { ConnectionLog } from "#/api/typesGenerated";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from "#/components/Table/Table";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { Timeline } from "#/components/Timeline/Timeline";
import { docs } from "#/utils/docs";
import { ConnectionLogFilter } from "./ConnectionLogFilter";
import { ConnectionLogHelpPopover } from "./ConnectionLogHelpPopover";
import { ConnectionLogRow } from "./ConnectionLogRow/ConnectionLogRow";

interface ConnectionLogPageViewProps {
	connectionLogs?: readonly ConnectionLog[];
	isNonInitialPage: boolean;
	isConnectionLogVisible: boolean;
	error?: unknown;
	filterProps: ComponentProps<typeof ConnectionLogFilter>;
	connectionLogsQuery: PaginationResult;
}

export const ConnectionLogPageView: FC<ConnectionLogPageViewProps> = ({
	connectionLogs,
	isNonInitialPage,
	isConnectionLogVisible,
	error,
	filterProps,
	connectionLogsQuery: paginationResult,
}) => {
	const isLoading =
		(connectionLogs === undefined ||
			paginationResult.totalRecords === undefined) &&
		!error;

	const isEmpty = !isLoading && connectionLogs?.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex flex-row gap-2 items-center">
						<span>Connection Log</span>
						<ConnectionLogHelpPopover />
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					View workspace connection events.
				</PageHeaderSubtitle>
			</PageHeader>

			{isConnectionLogVisible ? (
				<>
					<ConnectionLogFilter {...filterProps} />

					<PaginationContainer
						query={paginationResult}
						paginationUnitLabel="logs"
					>
						<Table>
							<TableBody>
								<ConnectionLogTableBody
									connectionLogs={connectionLogs}
									error={error}
									isLoading={isLoading}
									isEmpty={isEmpty}
									isNonInitialPage={isNonInitialPage}
								/>
							</TableBody>
						</Table>
					</PaginationContainer>
				</>
			) : (
				<PaywallPremium
					message="Connection logs"
					description="Connection logs allow you to see how and when users connect to workspaces. You need a Premium license to use this feature."
					documentationLink={docs("/admin/monitoring/connection-logs")}
				/>
			)}
		</Margins>
	);
};

interface ConnectionLogTableBodyProps {
	connectionLogs: readonly ConnectionLog[] | undefined;
	error: unknown;
	isLoading: boolean;
	isEmpty: boolean;
	isNonInitialPage: boolean;
}

const ConnectionLogTableBody: FC<ConnectionLogTableBodyProps> = ({
	connectionLogs,
	error,
	isLoading,
	isEmpty,
	isNonInitialPage,
}) => {
	// An error renders as an empty table.
	if (error) {
		return (
			<TableRow>
				<TableCell colSpan={999}>
					<EmptyState message="An error occurred while loading connection logs" />
				</TableCell>
			</TableRow>
		);
	}
	if (isLoading) {
		return <TableLoader />;
	}
	if (isEmpty) {
		const emptyMessage = isNonInitialPage
			? "No connection logs available on this page"
			: "No connection logs available";
		return (
			<TableRow>
				<TableCell colSpan={999}>
					<EmptyState message={emptyMessage} />
				</TableCell>
			</TableRow>
		);
	}
	if (!connectionLogs) {
		return null;
	}
	return (
		<Timeline
			items={connectionLogs}
			getDate={(log) => new Date(log.connect_time)}
			row={(log) => <ConnectionLogRow key={log.id} connectionLog={log} />}
		/>
	);
};
