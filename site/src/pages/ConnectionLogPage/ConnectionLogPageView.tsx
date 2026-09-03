import type { ComponentProps, FC } from "react";
import type { ConnectionLog } from "#/api/typesGenerated";
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
import { SettingsHeaderDocsLink } from "#/components/SettingsHeader/SettingsHeader";
import { Table, TableBody } from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { Timeline } from "#/components/Timeline/Timeline";
import { PremiumPaywall } from "#/modules/paywall/PremiumPaywall";
import type { Permissions } from "#/modules/permissions";
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
	permissions: Permissions;
}

export const ConnectionLogPageView: FC<ConnectionLogPageViewProps> = ({
	connectionLogs,
	isNonInitialPage,
	isConnectionLogVisible,
	error,
	filterProps,
	connectionLogsQuery: paginationResult,
	permissions,
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
					View workspace connection events.{" "}
					<SettingsHeaderDocsLink
						href={docs("/admin/monitoring/connection-logs")}
					/>
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
				<PremiumPaywall
					source="connection_log"
					message="Connection logs"
					description="Track every SSH, IDE & port-forward connection."
					features={[
						"Full record of SSH, IDE & app sessions",
						"Filter by organization, user & type",
						"Export to Splunk & other SIEMs",
					]}
					canViewPremium={permissions.viewAllLicenses}
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
			<TableEmpty message="An error occurred while loading connection logs" />
		);
	}
	if (isLoading) {
		return <TableLoader />;
	}
	if (isEmpty) {
		const emptyMessage = isNonInitialPage
			? "No connection logs available on this page"
			: "No connection logs available";
		return <TableEmpty message={emptyMessage} />;
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
