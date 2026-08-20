import type { ComponentProps, FC } from "react";
import type { AuditLog } from "#/api/typesGenerated";
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
import { Table, TableBody } from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { Timeline } from "#/components/Timeline/Timeline";
import type { Permissions } from "#/modules/permissions";
import { AuditFilter } from "./AuditFilter";
import { AuditHelpPopover } from "./AuditHelpPopover";
import { AuditLogRow } from "./AuditLogRow/AuditLogRow";

interface AuditPageViewProps {
	auditLogs?: readonly AuditLog[];
	isNonInitialPage: boolean;
	isAuditLogVisible: boolean;
	error?: unknown;
	filterProps: ComponentProps<typeof AuditFilter>;
	auditsQuery: PaginationResult;
	showOrgDetails: boolean;
	permissions: Permissions;
}

export const AuditPageView: FC<AuditPageViewProps> = ({
	auditLogs,
	isNonInitialPage,
	isAuditLogVisible,
	error,
	filterProps,
	auditsQuery: paginationResult,
	showOrgDetails,
	permissions,
}) => {
	const isLoading =
		(auditLogs === undefined || paginationResult.totalRecords === undefined) &&
		!error;

	const isEmpty = !isLoading && auditLogs?.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex flex-row gap-2 items-center">
						<span>Audit</span>
						<AuditHelpPopover />
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>View events in your audit log.</PageHeaderSubtitle>
			</PageHeader>

			{isAuditLogVisible ? (
				<>
					<AuditFilter {...filterProps} />

					<PaginationContainer
						query={paginationResult}
						paginationUnitLabel="logs"
					>
						<Table>
							<TableBody>
								<AuditTableBody
									auditLogs={auditLogs}
									error={error}
									isLoading={isLoading}
									isEmpty={isEmpty}
									isNonInitialPage={isNonInitialPage}
									showOrgDetails={showOrgDetails}
								/>
							</TableBody>
						</Table>
					</PaginationContainer>
				</>
			) : (
				<PaywallPremium
					message="Audit logs"
					description="See exactly who changed what and when, with every workspace, template, and user action logged for compliance and incident response."
					features={[
						"Configurable retention & auto-purge",
						"API export to Splunk, Datadog & more",
						"Meets SOC 2 & HIPAA audit requirements",
					]}
					canViewPremium={permissions.viewAllLicenses}
				/>
			)}
		</Margins>
	);
};

interface AuditTableBodyProps {
	auditLogs: readonly AuditLog[] | undefined;
	error: unknown;
	isLoading: boolean;
	isEmpty: boolean;
	isNonInitialPage: boolean;
	showOrgDetails: boolean;
}

const AuditTableBody: FC<AuditTableBodyProps> = ({
	auditLogs,
	error,
	isLoading,
	isEmpty,
	isNonInitialPage,
	showOrgDetails,
}) => {
	// An error renders as an empty table.
	if (error) {
		return <TableEmpty message="An error occurred while loading audit logs" />;
	}
	if (isLoading) {
		return <TableLoader />;
	}
	if (isEmpty) {
		const emptyMessage = isNonInitialPage
			? "No audit logs available on this page"
			: "No audit logs available";
		return <TableEmpty message={emptyMessage} />;
	}
	if (!auditLogs) {
		return null;
	}
	return (
		<Timeline
			items={auditLogs}
			getDate={(log) => new Date(log.time)}
			row={(log) => (
				<AuditLogRow
					key={log.id}
					auditLog={log}
					showOrgDetails={showOrgDetails}
				/>
			)}
		/>
	);
};
