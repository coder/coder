import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import Star from "@mui/icons-material/Star";
import Checkbox from "@mui/material/Checkbox";
import Skeleton from "@mui/material/Skeleton";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
	WorkspaceBuild,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
import { Stack } from "components/Stack/Stack";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { useDashboard } from "modules/dashboard/useDashboard";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import { WorkspaceDormantBadge } from "modules/workspaces/WorkspaceDormantBadge/WorkspaceDormantBadge";
import { WorkspaceOutdatedTooltip } from "modules/workspaces/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { type FC, type ReactNode, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { cn } from "utils/cn";
import {
	type DisplayWorkspaceStatusType,
	getDisplayWorkspaceStatus,
	getDisplayWorkspaceTemplateName,
	lastUsedMessage,
} from "utils/workspace";
import { WorkspacesEmpty } from "./WorkspacesEmpty";
import { BanIcon, PlayIcon, RefreshCcwIcon, SquareIcon } from "lucide-react";
import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useMutation, useQueryClient } from "react-query";
import {
	cancelBuild,
	deleteWorkspace,
	startWorkspace,
	stopWorkspace,
	updateWorkspace,
} from "api/queries/workspaces";
import { Spinner } from "components/Spinner/Spinner";
import { abilitiesByWorkspaceStatus } from "modules/workspaces/actions";

dayjs.extend(relativeTime);

export interface WorkspacesTableProps {
	workspaces?: readonly Workspace[];
	checkedWorkspaces: readonly Workspace[];
	error?: unknown;
	isUsingFilter: boolean;
	onUpdateWorkspace: (workspace: Workspace) => void;
	onCheckChange: (checkedWorkspaces: readonly Workspace[]) => void;
	canCheckWorkspaces: boolean;
	templates?: Template[];
	canCreateTemplate: boolean;
	onActionSuccess: () => Promise<void>;
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({
	workspaces,
	checkedWorkspaces,
	isUsingFilter,
	onUpdateWorkspace,
	onCheckChange,
	canCheckWorkspaces,
	templates,
	canCreateTemplate,
	onActionSuccess,
}) => {
	const dashboard = useDashboard();
	const workspaceIDToAppByStatus = useMemo(() => {
		return (
			workspaces?.reduce(
				(acc, workspace) => {
					if (!workspace.latest_app_status) {
						return acc;
					}
					for (const resource of workspace.latest_build.resources) {
						for (const agent of resource.agents ?? []) {
							for (const app of agent.apps ?? []) {
								if (app.id === workspace.latest_app_status.app_id) {
									acc[workspace.id] = { app, agent };
									break;
								}
							}
						}
					}
					return acc;
				},
				{} as Record<
					string,
					{
						app: WorkspaceApp;
						agent: WorkspaceAgent;
					}
				>,
			) || {}
		);
	}, [workspaces]);
	const hasAppStatus = useMemo(
		() => Object.keys(workspaceIDToAppByStatus).length > 0,
		[workspaceIDToAppByStatus],
	);

	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead className={hasAppStatus ? "w-1/6" : "w-2/6"}>
						<div className="flex items-center gap-2">
							{canCheckWorkspaces && (
								<Checkbox
									className="-my-[9px]"
									disabled={!workspaces || workspaces.length === 0}
									checked={checkedWorkspaces.length === workspaces?.length}
									size="xsmall"
									onChange={(_, checked) => {
										if (!workspaces) {
											return;
										}

										if (!checked) {
											onCheckChange([]);
										} else {
											onCheckChange(workspaces);
										}
									}}
								/>
							)}
							Name
						</div>
					</TableHead>
					{hasAppStatus && <TableHead className="w-2/6">Activity</TableHead>}
					<TableHead className="w-2/6">Template</TableHead>
					<TableHead className="w-2/6">Status</TableHead>
					<TableHead className="w-0" />
					<TableHead className="w-0" />
				</TableRow>
			</TableHeader>
			<TableBody className="[&_td]:h-[72px]">
				{!workspaces && <TableLoader canCheckWorkspaces={canCheckWorkspaces} />}
				{workspaces && workspaces.length === 0 && (
					<TableRow>
						<TableCell colSpan={999}>
							<WorkspacesEmpty
								templates={templates}
								isUsingFilter={isUsingFilter}
								canCreateTemplate={canCreateTemplate}
							/>
						</TableCell>
					</TableRow>
				)}
				{workspaces?.map((workspace) => {
					const checked = checkedWorkspaces.some((w) => w.id === workspace.id);
					const activeOrg = dashboard.organizations.find(
						(o) => o.id === workspace.organization_id,
					);

					return (
						<WorkspacesRow
							workspace={workspace}
							key={workspace.id}
							checked={checked}
						>
							<TableCell>
								<div className="flex items-center gap-2">
									{canCheckWorkspaces && (
										<Checkbox
											data-testid={`checkbox-${workspace.id}`}
											size="xsmall"
											disabled={cantBeChecked(workspace)}
											checked={checked}
											onClick={(e) => {
												e.stopPropagation();
											}}
											onChange={(e) => {
												if (e.currentTarget.checked) {
													onCheckChange([...checkedWorkspaces, workspace]);
												} else {
													onCheckChange(
														checkedWorkspaces.filter(
															(w) => w.id !== workspace.id,
														),
													);
												}
											}}
										/>
									)}
									<AvatarData
										title={
											<Stack direction="row" spacing={0.5} alignItems="center">
												{workspace.name}
												{workspace.favorite && <Star className="w-4 h-4" />}
												{workspace.outdated && (
													<WorkspaceOutdatedTooltip
														organizationName={workspace.organization_name}
														templateName={workspace.template_name}
														latestVersionId={
															workspace.template_active_version_id
														}
														onUpdateVersion={() => {
															onUpdateWorkspace(workspace);
														}}
													/>
												)}
											</Stack>
										}
										subtitle={
											<div>
												<span className="sr-only">Owner: </span>
												{workspace.owner_name}
											</div>
										}
										avatar={
											<Avatar
												src={workspace.owner_avatar_url}
												fallback={workspace.owner_name}
												size="lg"
											/>
										}
									/>
								</div>
							</TableCell>

							{hasAppStatus && (
								<TableCell>
									<WorkspaceAppStatus
										workspace={workspace}
										agent={workspaceIDToAppByStatus[workspace.id]?.agent}
										app={workspaceIDToAppByStatus[workspace.id]?.app}
										status={workspace.latest_app_status}
									/>
								</TableCell>
							)}

							<TableCell>
								<AvatarData
									title={getDisplayWorkspaceTemplateName(workspace)}
									subtitle={
										dashboard.showOrganizations && (
											<>
												<span className="sr-only">Organization:</span>{" "}
												{activeOrg?.display_name || workspace.organization_name}
											</>
										)
									}
									avatar={
										<Avatar
											variant="icon"
											src={workspace.template_icon}
											fallback={getDisplayWorkspaceTemplateName(workspace)}
											size="lg"
										/>
									}
								/>
							</TableCell>

							<WorkspaceStatusCell workspace={workspace} />
							<WorkspaceActionsCell
								workspace={workspace}
								onActionSuccess={onActionSuccess}
							/>
							<TableCell>
								<div className="flex">
									<KeyboardArrowRight className="text-content-secondary size-icon-sm" />
								</div>
							</TableCell>
						</WorkspacesRow>
					);
				})}
			</TableBody>
		</Table>
	);
};

interface WorkspacesRowProps {
	workspace: Workspace;
	children?: ReactNode;
	checked: boolean;
}

const WorkspacesRow: FC<WorkspacesRowProps> = ({
	workspace,
	children,
	checked,
}) => {
	const navigate = useNavigate();

	const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`;
	const openLinkInNewTab = () => window.open(workspacePageLink, "_blank");
	const { role, hover, ...clickableProps } = useClickableTableRow({
		onMiddleClick: openLinkInNewTab,
		onClick: (event) => {
			// Order of booleans actually matters here for Windows-Mac compatibility;
			// meta key is Cmd on Macs, but on Windows, it's either the Windows key,
			// or the key does nothing at all (depends on the browser)
			const shouldOpenInNewTab =
				event.shiftKey || event.metaKey || event.ctrlKey;

			if (shouldOpenInNewTab) {
				openLinkInNewTab();
			} else {
				navigate(workspacePageLink);
			}
		},
	});

	return (
		<TableRow
			{...clickableProps}
			data-testid={`workspace-${workspace.id}`}
			className={cn([
				checked ? "bg-muted hover:bg-muted" : undefined,
				clickableProps.className,
			])}
		>
			{children}
		</TableRow>
	);
};

interface TableLoaderProps {
	canCheckWorkspaces?: boolean;
}

const TableLoader: FC<TableLoaderProps> = ({ canCheckWorkspaces }) => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell className="w-2/6">
					<div className="flex items-center gap-2">
						{canCheckWorkspaces && <Checkbox size="small" disabled />}
						<AvatarDataSkeleton />
					</div>
				</TableCell>
				<TableCell className="w-2/6">
					<AvatarDataSkeleton />
				</TableCell>
				<TableCell className="w-2/6">
					<Skeleton variant="text" width="50%" />
				</TableCell>
				<TableCell>
					<div className="flex">
						<KeyboardArrowRight className="text-content-disabled size-icon-sm" />
					</div>
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

const cantBeChecked = (workspace: Workspace) => {
	return ["deleting", "pending"].includes(workspace.latest_build.status);
};

type WorkspaceStatusCellProps = {
	workspace: Workspace;
};

const variantByStatusType: Record<
	DisplayWorkspaceStatusType,
	StatusIndicatorProps["variant"]
> = {
	active: "pending",
	inactive: "inactive",
	success: "success",
	error: "failed",
	danger: "warning",
	warning: "warning",
};

const WorkspaceStatusCell: FC<WorkspaceStatusCellProps> = ({ workspace }) => {
	const { text, type } = getDisplayWorkspaceStatus(
		workspace.latest_build.status,
		workspace.latest_build.job,
	);

	return (
		<TableCell>
			<div className="flex flex-col">
				<StatusIndicator variant={variantByStatusType[type]}>
					<StatusIndicatorDot />
					{text}
					{workspace.latest_build.status === "running" &&
						!workspace.health.healthy && (
							<InfoTooltip
								type="warning"
								title="Workspace is unhealthy"
								message="Your workspace is running but some agents are unhealthy."
							/>
						)}
					{workspace.dormant_at && (
						<WorkspaceDormantBadge workspace={workspace} />
					)}
				</StatusIndicator>
				<span className="text-xs font-medium text-content-secondary ml-6">
					{lastUsedMessage(workspace.last_used_at)}
				</span>
			</div>
		</TableCell>
	);
};

type WorkspaceActionsCellProps = {
	workspace: Workspace;
	onActionSuccess: () => Promise<void>;
};

const WorkspaceActionsCell: FC<WorkspaceActionsCellProps> = ({
	workspace,
	onActionSuccess,
}) => {
	const queryClient = useQueryClient();
	const abilities = abilitiesByWorkspaceStatus(workspace, false);

	const startWorkspaceOptions = startWorkspace(workspace, queryClient);
	const startWorkspaceMutation = useMutation({
		...startWorkspaceOptions,
		onSuccess: async (build) => {
			startWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
	});

	const stopWorkspaceOptions = stopWorkspace(workspace, queryClient);
	const stopWorkspaceMutation = useMutation({
		...stopWorkspaceOptions,
		onSuccess: async (build) => {
			stopWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
	});

	const cancelJobOptions = cancelBuild(workspace, queryClient);
	const cancelBuildMutation = useMutation({
		...cancelJobOptions,
		onSuccess: async () => {
			cancelJobOptions.onSuccess();
			await onActionSuccess();
		},
	});

	const updateWorkspaceOptions = updateWorkspace(workspace, queryClient);
	const updateWorkspaceMutation = useMutation({
		...updateWorkspaceOptions,
		onSuccess: async (build) => {
			updateWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
	});

	const deleteWorkspaceOptions = deleteWorkspace(workspace, queryClient);
	const deleteWorkspaceMutation = useMutation({
		...deleteWorkspaceOptions,
		onSuccess: async (build) => {
			deleteWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
	});

	const isRetrying =
		startWorkspaceMutation.isLoading ||
		stopWorkspaceMutation.isLoading ||
		deleteWorkspaceMutation.isLoading;

	const retry = () => {
		switch (workspace.latest_build.transition) {
			case "start":
				startWorkspaceMutation.mutate({});
				break;
			case "stop":
				stopWorkspaceMutation.mutate({});
				break;
			case "delete":
				deleteWorkspaceMutation.mutate({});
				break;
		}
	};

	return (
		<TableCell>
			{abilities.actions.includes("start") && (
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="icon-lg"
								onClick={(e) => {
									e.stopPropagation();
									startWorkspaceMutation.mutate({});
								}}
							>
								<Spinner loading={startWorkspaceMutation.isLoading}>
									<PlayIcon />
								</Spinner>
								<span className="sr-only">Start workspace</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Start workspace</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}

			{abilities.actions.includes("updateAndStart") && (
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="icon-lg"
								onClick={(e) => {
									e.stopPropagation();
									updateWorkspaceMutation.mutate(undefined);
								}}
							>
								<Spinner loading={startWorkspaceMutation.isLoading}>
									<PlayIcon />
								</Spinner>
								<span className="sr-only">Update and start workspace</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Update and start workspace</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}

			{abilities.actions.includes("stop") && (
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="icon-lg"
								onClick={(e) => {
									e.stopPropagation();
									stopWorkspaceMutation.mutate({});
								}}
							>
								<Spinner loading={stopWorkspaceMutation.isLoading}>
									<SquareIcon />
								</Spinner>
								<span className="sr-only">Stop workspace</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Stop workspace</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}

			{abilities.canCancel && (
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="icon-lg"
								onClick={(e) => {
									e.stopPropagation();
									cancelBuildMutation.mutate();
								}}
							>
								<Spinner loading={cancelBuildMutation.isLoading}>
									<BanIcon />
								</Spinner>
								<span className="sr-only">Cancel current job</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Cancel current job</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}

			{abilities.actions.includes("retry") && (
				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="outline"
								size="icon-lg"
								onClick={(e) => {
									e.stopPropagation();
									retry();
								}}
							>
								<Spinner loading={isRetrying}>
									<RefreshCcwIcon />
								</Spinner>
								<span className="sr-only">Retry</span>
							</Button>
						</TooltipTrigger>
						<TooltipContent>Retry</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}
		</TableCell>
	);
};
