import type { ComponentProps, FC } from "react";
import type { AuditLog } from "#/api/typesGenerated";
import { ChooseOne, Cond } from "#/components/Conditionals/ChooseOne";
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
}

export const AuditPageView: FC<AuditPageViewProps> = ({
	auditLogs,
	isNonInitialPage,
	isAuditLogVisible,
	error,
	filterProps,
	auditsQuery: paginationResult,
	showOrgDetails,
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

			<ChooseOne>
				<Cond condition={isAuditLogVisible}>
					<AuditFilter {...filterProps} />

					<PaginationContainer
						query={paginationResult}
						paginationUnitLabel="logs"
					>
						<Table>
							<TableBody>
								<ChooseOne>
									{/* Error condition should just show an empty table. */}
									<Cond condition={Boolean(error)}>
										<TableRow>
											<TableCell colSpan={999}>
												<EmptyState message="An error occurred while loading audit logs" />
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
														<EmptyState message="No audit logs available on this page" />
													</TableCell>
												</TableRow>
											</Cond>

											<Cond>
												<TableRow>
													<TableCell colSpan={999}>
														<EmptyState message="No audit logs available" />
													</TableCell>
												</TableRow>
											</Cond>
										</ChooseOne>
									</Cond>

									<Cond>
										{auditLogs && (
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
										)}
									</Cond>
								</ChooseOne>
							</TableBody>
						</Table>
					</PaginationContainer>
				</Cond>

				<Cond>
					<PaywallPremium
						message="Audit logs"
						description="Audit logs allow you to monitor user operations on your deployment. You need a Premium license to use this feature."
						documentationLink={docs("/admin/security/audit-logs")}
					/>
				</Cond>
			</ChooseOne>
		</Margins>
	);
};
