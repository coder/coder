import { PlayIcon, RotateCcwIcon, SquareIcon, TrashIcon } from "lucide-react";
import { type UseQueryResult, useQueries } from "react-query";
import { hasError, isApiValidationError } from "#/api/errors";
import { workspacePermissions } from "#/api/queries/workspaces";
import type { Template, Workspace } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { PaginationAmount } from "#/components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { Spinner } from "#/components/Spinner/Spinner";
import { TableToolbar } from "#/components/TableToolbar/TableToolbar";
import { canEditWorkspaceParameters } from "#/modules/workspaces/permissions";
import { WorkspacesTable } from "#/pages/WorkspacesPage/WorkspacesTable";
import { mustUpdateWorkspace } from "#/utils/workspace";
import { WorkspacesFilter } from "./filter/WorkspacesFilter";
import { WorkspaceHelpPopover } from "./WorkspaceHelpPopover";
import { WorkspacesButton } from "./WorkspacesButton";

type TemplateQuery = UseQueryResult<Template[]>;
type WorkspacesPageViewProps = {
	error: unknown;
	workspaces?: readonly Workspace[];
	checkedWorkspaces: readonly Workspace[];
	count?: number;
	filter: UseFilterResult;
	page: number;
	limit: number;
	onPageChange: (page: number) => void;
	onCheckChange: (checkedWorkspaces: readonly Workspace[]) => void;
	isRunningBatchAction: boolean;
	onBatchDeleteTransition: () => void;
	onBatchUpdateTransition: () => void;
	onBatchStartTransition: () => void;
	onBatchStopTransition: () => void;
	templatesFetchStatus: TemplateQuery["status"];
	templates: TemplateQuery["data"];
	canCreateTemplate: boolean;
	canCreateWorkspace: boolean;
	canChangeVersions: boolean;
	onActionSuccess: () => Promise<void>;
	onActionError: (error: unknown) => void;
	chatsByWorkspace?: Record<string, string>;
};

export const WorkspacesPageView: React.FC<WorkspacesPageViewProps> = ({
	workspaces,
	error,
	limit,
	count,
	filter,
	onPageChange,
	page,
	checkedWorkspaces,
	onCheckChange,
	onBatchDeleteTransition,
	onBatchUpdateTransition,
	onBatchStopTransition,
	onBatchStartTransition,
	isRunningBatchAction,
	templates,
	templatesFetchStatus,
	canCreateTemplate,
	canCreateWorkspace,
	canChangeVersions,
	onActionSuccess,
	onActionError,
	chatsByWorkspace,
}) => {
	const selectedPermissions = useQueries({
		queries: checkedWorkspaces.map((workspace) =>
			workspacePermissions(workspace),
		),
	});
	const canBatchStart = checkedWorkspaces.every(
		(workspace, index) =>
			workspace.latest_build.status !== "stopped" ||
			selectedPermissions[index]?.data?.startWorkspace,
	);
	const canBatchStop = checkedWorkspaces.every(
		(workspace, index) =>
			workspace.latest_build.status !== "running" ||
			selectedPermissions[index]?.data?.stopWorkspace,
	);
	const canBatchDelete = checkedWorkspaces.every(
		(_, index) => selectedPermissions[index]?.data?.deleteWorkspace,
	);
	const canBatchUpdate = checkedWorkspaces.every(
		(workspace, index) =>
			!workspace.outdated ||
			Boolean(workspace.dormant_at) ||
			canEditWorkspaceParameters(workspace, selectedPermissions[index]?.data),
	);

	const hasStartableWorkspace = checkedWorkspaces.some(
		(workspace) =>
			workspace.latest_build.status === "stopped" &&
			!mustUpdateWorkspace(workspace, canChangeVersions),
	);
	const hasRunningWorkspace = checkedWorkspaces.some(
		(workspace) => workspace.latest_build.status === "running",
	);
	const permissionsError = selectedPermissions.find(
		(query) => query.error,
	)?.error;
	const restrictedActions = [
		hasStartableWorkspace && !canBatchStart && "Start",
		hasRunningWorkspace && !canBatchStop && "Stop",
		!canBatchUpdate && "Update",
		!canBatchDelete && "Delete",
	].filter(Boolean);
	const permissionsMessage = selectedPermissions.some(
		(query) => query.isLoadingError,
	)
		? "Unable to load permissions for all selected workspaces."
		: selectedPermissions.some((query) => query.isPending)
			? "Checking workspace permissions…"
			: restrictedActions.length > 0
				? `Missing permissions for ${restrictedActions.join(", ")} on one or more selected workspaces.`
				: undefined;

	// Let's say the user has 5 workspaces, but tried to hit page 100, which
	// does not exist. In this case, the page is not valid and we want to show a
	// better error message.
	const pageNumberIsInvalid = page !== 1 && workspaces?.length === 0;

	return (
		<Margins className="pb-12">
			<PageHeader
				actions={
					canCreateWorkspace && (
						<WorkspacesButton
							templates={templates}
							templatesFetchStatus={templatesFetchStatus}
						>
							New workspace
						</WorkspacesButton>
					)
				}
			>
				<PageHeaderTitle tooltip={<WorkspaceHelpPopover />}>
					Workspaces
				</PageHeaderTitle>
			</PageHeader>

			<div className="mb-4 flex flex-col gap-4">
				{hasError(error) && !isApiValidationError(error) && (
					<ErrorAlert error={error} />
				)}
				{Boolean(permissionsError) && <ErrorAlert error={permissionsError} />}
				<WorkspacesFilter filter={filter} error={error} />
			</div>

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
										<ChevronDownIcon />
									</Spinner>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end" description={permissionsMessage}>
								<DropdownMenuItem
									disabled={!canBatchStart || !hasStartableWorkspace}
									onSelect={onBatchStartTransition}
								>
									<PlayIcon /> Start
								</DropdownMenuItem>
								<DropdownMenuItem
									disabled={!canBatchStop || !hasRunningWorkspace}
									onSelect={onBatchStopTransition}
								>
									<SquareIcon /> Stop
								</DropdownMenuItem>
								<DropdownMenuSeparator />
								<DropdownMenuItem
									disabled={!canBatchUpdate}
									onSelect={onBatchUpdateTransition}
								>
									<RotateCcwIcon
										className="size-icon-sm"
										data-testid="bulk-action-update"
									/>{" "}
									Update&hellip;
								</DropdownMenuItem>
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									disabled={!canBatchDelete}
									onSelect={onBatchDeleteTransition}
								>
									<TrashIcon /> Delete&hellip;
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</>
				) : (
					!pageNumberIsInvalid && (
						<PaginationAmount
							paginationUnitLabel={{
								singular: "workspace",
								plural: "workspaces",
							}}
							limit={limit}
							totalRecords={count}
							currentOffsetStart={(page - 1) * limit + 1}
							isFiltered={filter.query.trim() !== ""}
						/>
					)
				)}
			</TableToolbar>

			{pageNumberIsInvalid ? (
				<EmptyState
					className="border rounded-lg"
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
					canCreateWorkspace={canCreateWorkspace}
					workspaces={workspaces}
					isUsingFilter={filter.used}
					onClearFilter={() => filter.update("")}
					checkedWorkspaces={checkedWorkspaces}
					onCheckChange={onCheckChange}
					templates={templates}
					onActionSuccess={onActionSuccess}
					onActionError={onActionError}
					chatsByWorkspace={chatsByWorkspace}
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
