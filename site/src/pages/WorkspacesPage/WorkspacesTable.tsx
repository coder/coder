import Checkbox from "@mui/material/Checkbox";
import Skeleton from "@mui/material/Skeleton";
import { templateVersion } from "api/queries/templates";
import { apiKey } from "api/queries/users";
import {
	cancelBuild,
	deleteWorkspace,
	startWorkspace,
	stopWorkspace,
} from "api/queries/workspaces";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import { VSCodeInsidersIcon } from "components/Icons/VSCodeInsidersIcon";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
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
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { StarIcon } from "lucide-react";
import { EllipsisVertical } from "lucide-react";
import {
	BanIcon,
	PlayIcon,
	RefreshCcwIcon,
	SquareTerminalIcon,
} from "lucide-react";
import {
	getTerminalHref,
	getVSCodeHref,
	openAppInNewWindow,
} from "modules/apps/apps";
import { useAppLink } from "modules/apps/useAppLink";
import { useDashboard } from "modules/dashboard/useDashboard";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import { WorkspaceDormantBadge } from "modules/workspaces/WorkspaceDormantBadge/WorkspaceDormantBadge";
import { WorkspaceMoreActions } from "modules/workspaces/WorkspaceMoreActions/WorkspaceMoreActions";
import { WorkspaceOutdatedTooltip } from "modules/workspaces/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { WorkspaceStatusIndicator } from "modules/workspaces/WorkspaceStatusIndicator/WorkspaceStatusIndicator";
import {
	WorkspaceUpdateDialogs,
	useWorkspaceUpdate,
} from "modules/workspaces/WorkspaceUpdateDialogs";
import { abilitiesByWorkspaceStatus } from "modules/workspaces/actions";
import type React from "react";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useMemo,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { cn } from "utils/cn";
import {
	getDisplayWorkspaceTemplateName,
	lastUsedMessage,
} from "utils/workspace";
import { WorkspacesEmpty } from "./WorkspacesEmpty";

export interface WorkspacesTableProps {
	workspaces?: readonly Workspace[];
	checkedWorkspaces: readonly Workspace[];
	error?: unknown;
	isUsingFilter: boolean;
	onCheckChange: (checkedWorkspaces: readonly Workspace[]) => void;
	canCheckWorkspaces: boolean;
	templates?: Template[];
	canCreateTemplate: boolean;
	onActionSuccess: () => Promise<void>;
	onActionError: (error: unknown) => void;
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({
	workspaces,
	checkedWorkspaces,
	isUsingFilter,
	onCheckChange,
	canCheckWorkspaces,
	templates,
	canCreateTemplate,
	onActionSuccess,
	onActionError,
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
												{workspace.favorite && (
													<StarIcon className="size-icon-xs" />
												)}
												{workspace.outdated && (
													<WorkspaceOutdatedTooltip workspace={workspace} />
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
								onActionError={onActionError}
							/>
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
				<TableCell className="w-0 ">
					<div className="flex gap-1 justify-end">
						<Skeleton variant="rounded" width={40} height={40} />
						<Button size="icon-lg" variant="subtle" disabled>
							<EllipsisVertical aria-hidden="true" />
						</Button>
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

const WorkspaceStatusCell: FC<WorkspaceStatusCellProps> = ({ workspace }) => {
	return (
		<TableCell>
			<div className="flex flex-col">
				<WorkspaceStatusIndicator workspace={workspace}>
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
				</WorkspaceStatusIndicator>
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
	onActionError: (error: unknown) => void;
};

const WorkspaceActionsCell: FC<WorkspaceActionsCellProps> = ({
	workspace,
	onActionSuccess,
	onActionError,
}) => {
	const { user } = useAuthenticated();

	const queryClient = useQueryClient();
	const abilities = abilitiesByWorkspaceStatus(workspace, {
		canDebug: false,
		isOwner: user.roles.find((role) => role.name === "owner") !== undefined,
	});

	const startWorkspaceOptions = startWorkspace(workspace, queryClient);
	const startWorkspaceMutation = useMutation({
		...startWorkspaceOptions,
		onSuccess: async (build) => {
			startWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
		onError: onActionError,
	});

	const stopWorkspaceOptions = stopWorkspace(workspace, queryClient);
	const stopWorkspaceMutation = useMutation({
		...stopWorkspaceOptions,
		onSuccess: async (build) => {
			stopWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
		onError: onActionError,
	});

	const cancelJobOptions = cancelBuild(workspace, queryClient);
	const cancelBuildMutation = useMutation({
		...cancelJobOptions,
		onSuccess: async () => {
			cancelJobOptions.onSuccess();
			await onActionSuccess();
		},
		onError: onActionError,
	});

	const { data: latestVersion } = useQuery({
		...templateVersion(workspace.template_active_version_id),
		enabled: workspace.outdated,
	});
	const workspaceUpdate = useWorkspaceUpdate({
		workspace,
		latestVersion,
		onSuccess: onActionSuccess,
		onError: onActionError,
	});

	const deleteWorkspaceOptions = deleteWorkspace(workspace, queryClient);
	const deleteWorkspaceMutation = useMutation({
		...deleteWorkspaceOptions,
		onSuccess: async (build) => {
			deleteWorkspaceOptions.onSuccess(build);
			await onActionSuccess();
		},
		onError: onActionError,
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
		<TableCell
			onClick={(e) => {
				// Prevent the click in the actions to trigger the row click
				e.stopPropagation();
			}}
		>
			<div className="flex gap-1 justify-end">
				{workspace.latest_build.status === "running" && (
					<WorkspaceApps workspace={workspace} />
				)}

				{abilities.actions.includes("start") && (
					<PrimaryAction
						onClick={() => startWorkspaceMutation.mutate({})}
						isLoading={startWorkspaceMutation.isLoading}
						label="Start workspace"
					>
						<PlayIcon />
					</PrimaryAction>
				)}

				{abilities.actions.includes("updateAndStart") && (
					<>
						<PrimaryAction
							onClick={workspaceUpdate.update}
							isLoading={workspaceUpdate.isUpdating}
							label="Update and start workspace"
						>
							<PlayIcon />
						</PrimaryAction>
						<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />
					</>
				)}

				{abilities.canCancel && (
					<PrimaryAction
						onClick={cancelBuildMutation.mutate}
						isLoading={cancelBuildMutation.isLoading}
						label="Cancel build"
					>
						<BanIcon />
					</PrimaryAction>
				)}

				{abilities.actions.includes("retry") && (
					<PrimaryAction
						onClick={retry}
						isLoading={isRetrying}
						label="Retry build"
					>
						<RefreshCcwIcon />
					</PrimaryAction>
				)}

				<WorkspaceMoreActions
					workspace={workspace}
					disabled={!abilities.canAcceptJobs}
				/>
			</div>
		</TableCell>
	);
};

type PrimaryActionProps = PropsWithChildren<{
	label: string;
	isLoading?: boolean;
	onClick: () => void;
}>;

const PrimaryAction: FC<PrimaryActionProps> = ({
	onClick,
	isLoading,
	label,
	children,
}) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button variant="outline" size="icon-lg" onClick={onClick}>
						<Spinner loading={isLoading}>{children}</Spinner>
						<span className="sr-only">{label}</span>
					</Button>
				</TooltipTrigger>
				<TooltipContent>{label}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

// The total number of apps that can be displayed in the workspace row
const WORKSPACE_APPS_SLOTS = 4;

type WorkspaceAppsProps = {
	workspace: Workspace;
};

const WorkspaceApps: FC<WorkspaceAppsProps> = ({ workspace }) => {
	const { data: apiKeyResponse } = useQuery(apiKey());
	const token = apiKeyResponse?.key;

	/**
	 * Coder is pretty flexible and allows an enormous variety of use cases, such
	 * as having multiple resources with many agents, but they are not common. The
	 * most common scenario is to have one single compute resource with one single
	 * agent containing all the apps. Lets test this getting the apps for the
	 * first resource, and first agent - they are sorted to return the compute
	 * resource first - and see what customers and ourselves, using dogfood, think
	 * about that.
	 */
	const agent = workspace.latest_build.resources
		.filter((r) => !r.hide)
		.at(0)
		?.agents?.at(0);
	if (!agent) {
		return null;
	}

	const builtinApps = new Set(agent.display_apps);
	builtinApps.delete("port_forwarding_helper");
	builtinApps.delete("ssh_helper");

	const remainingSlots = WORKSPACE_APPS_SLOTS - builtinApps.size;
	const userApps = agent.apps
		.filter((app) => app.health === "healthy" && !app.hidden)
		.slice(0, remainingSlots);

	const buttons: ReactNode[] = [];

	if (builtinApps.has("vscode")) {
		buttons.push(
			<BaseIconLink
				key="vscode"
				isLoading={!token}
				label="Open VSCode"
				href={getVSCodeHref("vscode", {
					owner: workspace.owner_name,
					workspace: workspace.name,
					agent: agent.name,
					token: token ?? "",
					folder: agent.expanded_directory,
				})}
			>
				<VSCodeIcon />
			</BaseIconLink>,
		);
	}

	if (builtinApps.has("vscode_insiders")) {
		buttons.push(
			<BaseIconLink
				key="vscode-insiders"
				label="Open VSCode Insiders"
				isLoading={!token}
				href={getVSCodeHref("vscode-insiders", {
					owner: workspace.owner_name,
					workspace: workspace.name,
					agent: agent.name,
					token: token ?? "",
					folder: agent.expanded_directory,
				})}
			>
				<VSCodeInsidersIcon />
			</BaseIconLink>,
		);
	}

	for (const app of userApps) {
		buttons.push(
			<IconAppLink
				key={app.id}
				app={app}
				workspace={workspace}
				agent={agent}
			/>,
		);
	}

	if (builtinApps.has("web_terminal")) {
		const href = getTerminalHref({
			username: workspace.owner_name,
			workspace: workspace.name,
			agent: agent.name,
		});
		buttons.push(
			<BaseIconLink
				key="terminal"
				href={href}
				onClick={(e) => {
					e.preventDefault();
					openAppInNewWindow(href);
				}}
				label="Open Terminal"
			>
				<SquareTerminalIcon />
			</BaseIconLink>,
		);
	}

	buttons.push();

	return buttons;
};

type IconAppLinkProps = {
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
};

const IconAppLink: FC<IconAppLinkProps> = ({ app, workspace, agent }) => {
	const link = useAppLink(app, {
		workspace,
		agent,
	});

	return (
		<BaseIconLink
			key={app.id}
			label={`Open ${link.label}`}
			href={link.href}
			onClick={link.onClick}
		>
			<ExternalImage src={app.icon ?? "/icon/widgets.svg"} />
		</BaseIconLink>
	);
};

type BaseIconLinkProps = PropsWithChildren<{
	label: string;
	href: string;
	isLoading?: boolean;
	onClick?: (e: React.MouseEvent<HTMLAnchorElement>) => void;
}>;

const BaseIconLink: FC<BaseIconLinkProps> = ({
	href,
	isLoading,
	label,
	children,
	onClick,
}) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button variant="outline" size="icon-lg" asChild>
						<a
							className={isLoading ? "animate-pulse" : ""}
							href={href}
							onClick={(e) => {
								e.stopPropagation();
								onClick?.(e);
							}}
						>
							{children}
							<span className="sr-only">{label}</span>
						</a>
					</Button>
				</TooltipTrigger>
				<TooltipContent>{label}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
