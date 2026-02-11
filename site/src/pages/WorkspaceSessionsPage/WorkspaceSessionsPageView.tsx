import type { Workspace, WorkspaceSession } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import {
	PaginationContainer,
	type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import { Table, TableBody } from "components/Table/Table";
import { TableLoader } from "components/TableLoader/TableLoader";
import { Timeline } from "components/Timeline/Timeline";
import { ArrowLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { WorkspaceSessionRow } from "./WorkspaceSessionRow";

interface WorkspaceSessionsPageViewProps {
	workspace?: Workspace;
	sessions?: readonly WorkspaceSession[];
	sessionsQuery: PaginationResult;
	isNonInitialPage: boolean;
	error?: unknown;
}

export const WorkspaceSessionsPageView: FC<WorkspaceSessionsPageViewProps> = ({
	workspace,
	sessions,
	sessionsQuery,
	isNonInitialPage,
	error,
}) => {
	const isLoading =
		(sessions === undefined || sessionsQuery.totalRecords === undefined) &&
		!error;
	const isEmpty = !isLoading && sessions?.length === 0;

	return (
		<Margins className="pb-12">
			{workspace && (
				<Link
					to={`/@${workspace.owner_name}/${workspace.name}`}
					className="inline-flex items-center gap-1.5 text-sm text-content-secondary hover:text-content-primary py-4"
				>
					<ArrowLeftIcon className="size-4" />
					Back to workspace
				</Link>
			)}

			<PageHeader>
				<PageHeaderTitle>Session History</PageHeaderTitle>
			</PageHeader>

			<ChooseOne>
				<Cond condition={Boolean(error)}>
					<EmptyState message="Failed to load session history." />
				</Cond>

				<Cond condition={isEmpty && !isNonInitialPage}>
					<EmptyState message="No sessions found for this workspace." />
				</Cond>

				<Cond>
					<PaginationContainer
						query={sessionsQuery}
						paginationUnitLabel="sessions"
					>
						<Table>
							<TableBody>
								<ChooseOne>
									<Cond condition={isLoading}>
										<TableLoader />
									</Cond>

									<Cond>
										{sessions && (
											<Timeline
												items={sessions}
												getDate={(session) =>
													new Date(session.started_at)
												}
												row={(session) => (
													<WorkspaceSessionRow
														key={
															session.id ??
															session.started_at
														}
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
			</ChooseOne>
		</Margins>
	);
};
