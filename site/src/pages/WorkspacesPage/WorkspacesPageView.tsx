import { hasError, isApiValidationError } from "api/errors";
import type { Template, Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { PaginationHeader } from "components/PaginationWidget/PaginationHeader";
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { TableToolbar } from "components/TableToolbar/TableToolbar";
import {
	ChevronDownIcon,
	CloudIcon,
	PlayIcon,
	SquareIcon,
	TrashIcon,
} from "lucide-react";
import { WorkspacesTable } from "pages/WorkspacesPage/WorkspacesTable";
import type { FC } from "react";
import type { UseQueryResult } from "react-query";
import { mustUpdateWorkspace } from "utils/workspace";
import {
	type WorkspaceFilterState,
	WorkspacesFilter,
} from "./filter/WorkspacesFilter";
import { WorkspaceHelpTooltip } from "./WorkspaceHelpTooltip";
import { WorkspacesButton } from "./WorkspacesButton";

const Language = {
	pageTitle: "Workspaces",
	yourWorkspacesButton: "Your workspaces",
	allWorkspacesButton: "All workspaces",
	runningWorkspacesButton: "Running workspaces",
	seeAllTemplates: "See all templates",
	template: "Template",
};

type TemplateQuery = UseQueryResult<Template[]>;
interface WorkspacesPageViewProps {
	error: unknown;
	workspaces?: readonly Workspace[];
	checkedWorkspaces: readonly Workspace[];
	count?: number;
	filterState: WorkspaceFilterState;
	page: number;
	limit: number;
	onPageChange: (page: number) => void;
	onCheckChange: (checkedWorkspaces: readonly Workspace[]) => void;
	isRunningBatchAction: boolean;
	onBatchDeleteTransition: () => void;
	onBatchUpdateTransition: () => void;
	onBatchStartTransition: () => void;
	onBatchStopTransition: () => void;
	canCheckWorkspaces: boolean;
	templatesFetchStatus: TemplateQuery["status"];
	templates: TemplateQuery["data"];
	canCreateTemplate: boolean;
	canChangeVersions: boolean;
	onActionSuccess: () => Promise<void>;
	onActionError: (error: unknown) => void;
}

export const WorkspacesPageView: FC<WorkspacesPageViewProps> = ({
	workspaces,
	error,
	limit,
	count,
	filterState,
	onPageChange,
	page,
	checkedWorkspaces,
	onCheckChange,
	onBatchDeleteTransition,
	onBatchUpdateTransition,
	onBatchStopTransition,
	onBatchStartTransition,
	isRunningBatchAction,
	canCheckWorkspaces,
	templates,
	templatesFetchStatus,
	canCreateTemplate,
	canChangeVersions,
	onActionSuccess,
	onActionError,
}) => {
	// Let's say the user has 5 workspaces, but tried to hit page 100, which
	// does not exist. In this case, the page is not valid and we want to show a
	// better error message.
	const pageNumberIsInvalid = page !== 1 && workspaces?.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader
				actions={
					<WorkspacesButton
						templates={templates}
						templatesFetchStatus={templatesFetchStatus}
					>
						New workspace
					</WorkspacesButton>
				}
			>
				<PageHeaderTitle>
					<Stack direction="row" spacing={1} alignItems="center">
						<span>{Language.pageTitle}</span>
						<WorkspaceHelpTooltip />
					</Stack>
				</PageHeaderTitle>
			</PageHeader>

			<Stack>
				{hasError(error) && !isApiValidationError(error) && (
					<ErrorAlert error={error} />
				)}
				<WorkspacesFilter
					filter={filterState.filter}
					error={error}
					statusMenu={filterState.menus.status}
					templateMenu={filterState.menus.template}
					userMenu={filterState.menus.user}
					organizationsMenu={filterState.menus.organizations}
				/>
			</Stack>

			<TableToolbar>
				{checkedWorkspaces.length > 0 ? (
					<>
						<div>
							Selected <strong>{checkedWorkspaces.length}</strong> of{" "}
							<strong>{workspaces?.length}</strong>{" "}
							{workspaces?.length === 1 ? "workspace" : "workspaces"}
						</div>

						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									disabled={isRunningBatchAction}
									variant="outline"
									size="sm"
									className="ml-auto"
								>
									Bulk actions
									<Spinner loading={isRunningBatchAction}>
										<ChevronDownIcon className="size-4" />
									</Spinner>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									disabled={
										!checkedWorkspaces?.every(
											(w) =>
												w.latest_build.status === "stopped" &&
												!mustUpdateWorkspace(w, canChangeVersions),
										)
									}
									onClick={onBatchStartTransition}
								>
									<PlayIcon /> Start
								</DropdownMenuItem>
								<DropdownMenuItem
									disabled={
										!checkedWorkspaces?.every(
											(w) => w.latest_build.status === "running",
										)
									}
									onClick={onBatchStopTransition}
								>
									<SquareIcon /> Stop
								</DropdownMenuItem>
								<DropdownMenuSeparator />
								<DropdownMenuItem onClick={onBatchUpdateTransition}>
									<CloudIcon
										className="size-icon-sm"
										data-testid="bulk-action-update"
									/>{" "}
									Update&hellip;
								</DropdownMenuItem>
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={onBatchDeleteTransition}
								>
									<TrashIcon /> Delete&hellip;
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</>
				) : (
					!pageNumberIsInvalid && (
						<PaginationHeader
							paginationUnitLabel="workspaces"
							limit={limit}
							totalRecords={count}
							currentOffsetStart={(page - 1) * limit + 1}
							className="pb-0"
						/>
					)
				)}
			</TableToolbar>
			{pageNumberIsInvalid ? (
				<EmptyState
					className="border border-solid border-zinc-700 rounded-lg"
					message="Page not found"
					description="The page you are trying to access does not exist."
					cta={
						<Button
							onClick={() => {
								onPageChange(1);
							}}
						>
							Back to the first page
						</Button>
					}
				/>
			) : (
				<WorkspacesTable
					canCreateTemplate={canCreateTemplate}
					workspaces={workspaces}
					isUsingFilter={filterState.filter.used}
					checkedWorkspaces={checkedWorkspaces}
					onCheckChange={onCheckChange}
					canCheckWorkspaces={canCheckWorkspaces}
					templates={templates}
					onActionSuccess={onActionSuccess}
					onActionError={onActionError}
				/>
			)}

			{count !== undefined && (
				// Temporary styling stopgap before component is migrated to using
				// PaginationContainer (which renders PaginationWidgetBase using CSS
				// flexbox gaps)
				<div className="pt-4">
					<PaginationWidgetBase
						totalRecords={count}
						pageSize={limit}
						onPageChange={onPageChange}
						currentPage={page}
					/>
				</div>
			)}
		</Margins>
	);
};
