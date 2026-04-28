import { API } from "api/api";
import { hasError, isApiValidationError } from "api/errors";
import type { Template, Workspace, WorkspaceStatus } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChevronDownIcon } from "components/AnimatedIcons/ChevronDown";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	type FilterCategory,
	FilterSearchField,
	type SearchResult,
} from "components/FilterSearchField/FilterSearchField";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { PaginationAmount } from "components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import {
	StatusIndicatorDot,
	type StatusIndicatorDotProps,
} from "components/StatusIndicator/StatusIndicator";
import { TableToolbar } from "components/TableToolbar/TableToolbar";
import { useAuthenticated } from "hooks";
import {
	Building2,
	CircleDot,
	CloudIcon,
	LayoutPanelTop,
	PlayIcon,
	SquareIcon,
	TrashIcon,
	User,
} from "lucide-react";
import { WorkspacesTable } from "pages/WorkspacesPage/WorkspacesTable";
import { type FC, useCallback, useMemo } from "react";
import type { UseQueryResult } from "react-query";
import {
	getDisplayWorkspaceStatus,
	mustUpdateWorkspace,
} from "utils/workspace";
import type { WorkspaceFilterState } from "./filter/WorkspacesFilter";
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

	const { user: me } = useAuthenticated();

	const categories = useMemo<FilterCategory[]>(() => {
		const cats: FilterCategory[] = [];

		// Owner category.
		cats.push({
			key: "owner",
			label: "Owner",
			icon: <User className="size-4" />,
			getOptions: async (query) => {
				const usersRes = await API.getUsers({ q: query, limit: 25 });
				const options = usersRes.users.map((user) => ({
					label: user.username,
					value: user.username,
					startIcon: (
						<Avatar fallback={user.username} src={user.avatar_url} size="sm" />
					),
				}));
				// Pin the current user first.
				const filtered = options.filter((o) => o.value !== me.username);
				return [
					{
						label: me.username,
						value: me.username,
						startIcon: (
							<Avatar fallback={me.username} src={me.avatar_url} size="sm" />
						),
					},
					...filtered,
				];
			},
		});

		// Template category.
		cats.push({
			key: "template",
			label: "Template",
			icon: <LayoutPanelTop className="size-4" />,
			getOptions: async (query) => {
				const templates = await API.getTemplates();
				const filtered = templates.filter(
					(t) =>
						t.name.toLowerCase().includes(query.toLowerCase()) ||
						t.display_name.toLowerCase().includes(query.toLowerCase()),
				);
				return filtered.map((t) => ({
					label: t.display_name || t.name,
					value: t.name,
					startIcon: (
						<Avatar
							size="sm"
							variant="icon"
							src={t.icon}
							fallback={t.display_name || t.name}
						/>
					),
				}));
			},
		});

		// Status category.
		const statusesToFilter: WorkspaceStatus[] = [
			"running",
			"stopped",
			"failed",
			"pending",
		];
		cats.push({
			key: "status",
			label: "Status",
			icon: <CircleDot className="size-4" />,
			getOptions: async () => {
				return statusesToFilter.map((status) => {
					const display = getDisplayWorkspaceStatus(status);
					return {
						label: display.text,
						value: status,
						startIcon: (
							<StatusIndicatorDot variant={getStatusVariant(status)} />
						),
					};
				});
			},
		});

		// Organization category.
		cats.push({
			key: "organization",
			label: "Organization",
			icon: <Building2 className="size-4" />,
			getOptions: async () => {
				const organizations = await API.getOrganizations();
				return organizations.map((org) => ({
					label: org.display_name || org.name,
					value: org.name,
					startIcon: (
						<Avatar
							size="sm"
							fallback={org.display_name || org.name}
							src={org.icon}
						/>
					),
				}));
			},
		});
		return cats;
	}, [me.username, me.avatar_url]);

	const getSearchResults = useCallback(
		async (query: string): Promise<SearchResult[]> => {
			const { workspaces } = await API.getWorkspaces({
				q: query,
				limit: 5,
			});
			return workspaces.map((ws) => {
				const display = getDisplayWorkspaceStatus(ws.latest_build.status);
				return {
					label: ws.name,
					value: ws.name,
					startIcon: (
						<Avatar
							size="sm"
							variant="icon"
							src={ws.template_icon}
							fallback={ws.name}
						/>
					),
					subtitle: `${ws.owner_name} · ${display.text}`,
				};
			});
		},
		[],
	);

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
				<FilterSearchField
					value={filterState.filter.query}
					onChange={filterState.filter.update}
					categories={categories}
					getSearchResults={getSearchResults}
					placeholder="Search workspaces..."
					aria-label="Filter workspaces"
					className="w-fit min-w-[min(550px,100%)] max-w-full"
				/>{" "}
			</Stack>
			{checkedWorkspaces.length > 0 && (
				<TableToolbar>
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
				</TableToolbar>
			)}
			{pageNumberIsInvalid ? (
				<EmptyState
					css={(theme) => ({
						border: `1px solid ${theme.palette.divider}`,
						borderRadius: theme.shape.borderRadius,
					})}
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
					templates={templates}
					onActionSuccess={onActionSuccess}
					onActionError={onActionError}
				/>
			)}
			{!pageNumberIsInvalid && count !== undefined && (
				<div className="flex flex-col gap-y-4 pt-4">
					<PaginationAmount
						paginationUnitLabel="workspaces"
						limit={limit}
						totalRecords={count}
						currentOffsetStart={(page - 1) * limit + 1}
						className="justify-end"
					/>
					<PaginationWidgetBase
						totalRecords={count}
						pageSize={limit}
						onPageChange={onPageChange}
						currentPage={page}
					/>
				</div>
			)}{" "}
		</Margins>
	);
};

const getStatusVariant = (
	status: WorkspaceStatus,
): StatusIndicatorDotProps["variant"] => {
	const display = getDisplayWorkspaceStatus(status);
	const variantByStatusType: Record<
		string,
		StatusIndicatorDotProps["variant"]
	> = {
		active: "pending",
		inactive: "inactive",
		success: "success",
		error: "failed",
		danger: "warning",
		warning: "warning",
	};
	return variantByStatusType[display.type] ?? "inactive";
};
