import type { GlobalWorkspaceSession } from "api/typesGenerated";
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
import { PaywallPremium } from "components/Paywall/PaywallPremium";
import { Stack } from "components/Stack/Stack";
import { Table, TableBody, TableCell, TableRow } from "components/Table/Table";
import { TableLoader } from "components/TableLoader/TableLoader";
import { Timeline } from "components/Timeline/Timeline";
import type { ComponentProps, FC } from "react";
import { docs } from "utils/docs";
import { ConnectionLogFilter } from "./ConnectionLogFilter";
import { ConnectionLogHelpTooltip } from "./ConnectionLogHelpTooltip";
import { GlobalSessionRow } from "./GlobalSessionRow";

const Language = {
	title: "Connection Log",
	subtitle: "View workspace connection sessions.",
};

interface ConnectionLogPageViewProps {
	sessions?: readonly GlobalWorkspaceSession[];
	isNonInitialPage: boolean;
	isConnectionLogVisible: boolean;
	error?: unknown;
	filterProps: ComponentProps<typeof ConnectionLogFilter>;
	sessionsQuery: PaginationResult;
}

export const ConnectionLogPageView: FC<ConnectionLogPageViewProps> = ({
	sessions,
	isNonInitialPage,
	isConnectionLogVisible,
	error,
	filterProps,
	sessionsQuery: paginationResult,
}) => {
	const isLoading =
		(sessions === undefined ||
			paginationResult.totalRecords === undefined) &&
		!error;

	const isEmpty = !isLoading && sessions?.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<Stack direction="row" spacing={1} alignItems="center">
						<span>{Language.title}</span>
						<ConnectionLogHelpTooltip />
					</Stack>
				</PageHeaderTitle>
				<PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
			</PageHeader>

			<ChooseOne>
				<Cond condition={isConnectionLogVisible}>
					<ConnectionLogFilter {...filterProps} />

					<PaginationContainer
						query={paginationResult}
						paginationUnitLabel="sessions"
					>
						<Table>
							<TableBody>
								<ChooseOne>
									{/* Error condition should just show an empty table. */}
									<Cond condition={Boolean(error)}>
										<TableRow>
											<TableCell colSpan={999}>
												<EmptyState message="An error occurred while loading sessions" />
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
														<EmptyState message="No sessions available on this page" />
													</TableCell>
												</TableRow>
											</Cond>

											<Cond>
												<TableRow>
													<TableCell colSpan={999}>
														<EmptyState message="No sessions available" />
													</TableCell>
												</TableRow>
											</Cond>
										</ChooseOne>
									</Cond>

									<Cond>
										{sessions && (
											<Timeline
												items={sessions}
												getDate={(session) => new Date(session.started_at)}
												row={(session) => (
													<GlobalSessionRow
														key={session.id ?? session.started_at}
														session={session}
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
						message="Connection logs"
						description="Connection logs allow you to see how and when users connect to workspaces. You need a Premium license to use this feature."
						documentationLink={docs("/admin/monitoring/connection-logs")}
					/>
				</Cond>
			</ChooseOne>
		</Margins>
	);
};
