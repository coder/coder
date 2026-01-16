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
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import { VSCodeInsidersIcon } from "components/Icons/VSCodeInsidersIcon";
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
import {
	BanIcon,
	CloudIcon,
	EllipsisVertical,
	ExternalLinkIcon,
	FileIcon,
	PlayIcon,
	RefreshCcwIcon,
	SquareTerminalIcon,
	StarIcon,
} from "lucide-react";
import {
	getTerminalHref,
	getVSCodeHref,
	openAppInNewWindow,
} from "modules/apps/apps";
import { useAppLink } from "modules/apps/useAppLink";
import { useDashboard } from "modules/dashboard/useDashboard";
import { abilitiesByWorkspaceStatus } from "modules/workspaces/actions";
import { WorkspaceBuildCancelDialog } from "modules/workspaces/WorkspaceBuildCancelDialog/WorkspaceBuildCancelDialog";
import { WorkspaceMoreActions } from "modules/workspaces/WorkspaceMoreActions/WorkspaceMoreActions";
import { WorkspaceOutdatedTooltip } from "modules/workspaces/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { WorkspaceStatus } from "modules/workspaces/WorkspaceStatus/WorkspaceStatus";
import {
	useWorkspaceUpdate,
	WorkspaceUpdateDialogs,
} from "modules/workspaces/WorkspaceUpdateDialogs";
import type React from "react";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { cn } from "utils/cn";
import { getDisplayWorkspaceTemplateName } from "utils/workspace";
import { WorkspaceSharingIndicator } from "./WorkspaceSharingIndicator";
import { WorkspacesEmpty } from "./WorkspacesEmpty";

interface WorkspacesTableProps {
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

	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead className="w-1/3">
						<div className="flex items-center gap-4">
							{canCheckWorkspaces && (
								<Checkbox
									disabled={!workspaces || workspaces.length === 0}
									checked={
										workspaces &&
										workspaces.length > 0 &&
										checkedWorkspaces.length === workspaces.length
									}
									onCheckedChange={(checked) => {
										if (!workspaces) {
											return;
										}

										if (!checked) {
											onCheckChange([]);
										} else {
											onCheckChange(workspaces);
										}
									}}
									aria-label="Select all workspaces"
								/>
							)}
							Name
						</div>
					</TableHead>
					<TableHead className="w-1/3">Template</TableHead>
					<TableHead className="w-1/3">Status</TableHead>
					<TableHead className="w-0">
						<span className="sr-only">Actions</span>
					</TableHead>
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
								<div className="flex items-center gap-4">
									{canCheckWorkspaces && (
										<Checkbox
											data-testid={`checkbox-${workspace.id}`}
											disabled={cantBeChecked(workspace)}
											checked={checked}
											onClick={(e) => {
												e.stopPropagation();
											}}
											onCheckedChange={(checked) => {
												if (checked) {
													onCheckChange([...checkedWorkspaces, workspace]);
												} else {
													onCheckChange(
														checkedWorkspaces.filter(
															(w) => w.id !== workspace.id,
														),
													);
												}
											}}
											aria-label={`Select workspace ${workspace.name}`}
										/>
									)}
									<AvatarData
										title={
											<Stack direction="row" spacing={0.5} alignItems="center">
												<span className="whitespace-nowrap">
													{workspace.name}
												</span>
												{workspace.favorite && (
													<StarIcon className="size-icon-xs" />
												)}
												{workspace.outdated && (
													<WorkspaceOutdatedTooltip workspace={workspace} />
												)}
												{workspace.task_id && (
													<Badge size="xs" variant="default">
														Task
													</Badge>
												)}
											</Stack>
										}
										subtitle={
											<div className="flex items-center gap-1">
												<span className="sr-only">Owner: </span>
												<div className="flex gap-2">
													{workspace.owner_name}
													{workspace.shared_with &&
														workspace.shared_with.length > 0 && (
															<WorkspaceSharingIndicator
																sharedWith={workspace.shared_with}
																settingsPath={`/@${workspace.owner_name}/${workspace.name}/settings/sharing`}
															/>
														)}
												</div>
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

							<TableCell>
								<AvatarData
									title={
										<span className="whitespace-nowrap block max-w-52 text-ellipsis overflow-hidden">
											{getDisplayWorkspaceTemplateName(workspace)}
										</span>
									}
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

							<TableCell>
								<WorkspaceStatus workspace={workspace} />
							</TableCell>

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
					<div className="flex items-center gap-4">
						{canCheckWorkspaces && <Checkbox disabled />}
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

	const [isStopConfirmOpen, setIsStopConfirmOpen] = useState(false);
	const [isCancelConfirmOpen, setIsCancelConfirmOpen] = useState(false);

	const isRetrying =
		startWorkspaceMutation.isPending ||
		stopWorkspaceMutation.isPending ||
		deleteWorkspaceMutation.isPending;

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
				{workspace.latest_build.status === "running" &&
					(workspace.latest_app_status ? (
						<WorkspaceAppStatusLinks workspace={workspace} />
					) : (
						<WorkspaceApps workspace={workspace} />
					))}

				{abilities.actions.includes("start") && (
					<PrimaryAction
						onClick={() => startWorkspaceMutation.mutate({})}
						isLoading={startWorkspaceMutation.isPending}
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
							<CloudIcon />
						</PrimaryAction>
						<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />
					</>
				)}

				{abilities.actions.includes("updateAndStartRequireActiveVersion") && (
					<>
						<PrimaryAction
							onClick={workspaceUpdate.update}
							isLoading={workspaceUpdate.isUpdating}
							label="This template requires automatic updates on workspace startup. Contact your administrator if you want to preserve the template version."
						>
							<PlayIcon />
						</PrimaryAction>
						<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />
					</>
				)}

				{abilities.actions.includes("updateAndRestart") && (
					<>
						<PrimaryAction
							onClick={workspaceUpdate.update}
							isLoading={workspaceUpdate.isUpdating}
							label="Update and restart workspace"
						>
							<CloudIcon />
						</PrimaryAction>
						<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />
					</>
				)}

				{abilities.actions.includes("updateAndRestartRequireActiveVersion") && (
					<>
						<PrimaryAction
							onClick={workspaceUpdate.update}
							isLoading={workspaceUpdate.isUpdating}
							label="This template requires automatic updates on workspace restart. Contact your administrator if you want to preserve the template version."
						>
							<PlayIcon />
						</PrimaryAction>
						<WorkspaceUpdateDialogs {...workspaceUpdate.dialogs} />
					</>
				)}

				{abilities.canCancel && (
					<PrimaryAction
						onClick={() => setIsCancelConfirmOpen(true)}
						isLoading={cancelBuildMutation.isPending}
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
					onStop={
						abilities.actions.includes("stop")
							? () => setIsStopConfirmOpen(true)
							: undefined
					}
					isStopping={stopWorkspaceMutation.isPending}
				/>
			</div>

			{/* Stop workspace confirmation dialog */}
			<ConfirmDialog
				open={isStopConfirmOpen}
				title="Stop workspace"
				description={`Are you sure you want to stop the workspace "${workspace.name}"? This will terminate all running processes and disconnect any active sessions.`}
				confirmText="Stop"
				onClose={() => setIsStopConfirmOpen(false)}
				onConfirm={() => {
					stopWorkspaceMutation.mutate({});
					setIsStopConfirmOpen(false);
				}}
				type="delete"
			/>

			<WorkspaceBuildCancelDialog
				open={isCancelConfirmOpen}
				onClose={() => setIsCancelConfirmOpen(false)}
				onConfirm={() => {
					cancelBuildMutation.mutate();
					setIsCancelConfirmOpen(false);
				}}
				workspace={workspace}
			/>
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
					<Button
						variant="outline"
						size="icon-lg"
						onClick={onClick}
						disabled={isLoading}
					>
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
		.filter(
			(app) =>
				(app.health === "healthy" || app.health === "disabled") && !app.hidden,
		)
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
				<SquareTerminalIcon className="!size-7" />
			</BaseIconLink>,
		);
	}

	buttons.push();

	return buttons;
};

type WorkspaceAppStatusLinksProps = {
	workspace: Workspace;
};

const WorkspaceAppStatusLinks: FC<WorkspaceAppStatusLinksProps> = ({
	workspace,
}) => {
	const status = workspace.latest_app_status;
	const agent = workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.find((a) => a?.id === status?.agent_id);
	const app = agent?.apps.find((a) => a.id === status?.app_id);

	return (
		<>
			{agent && app && (
				<IconAppLink app={app} workspace={workspace} agent={agent} />
			)}

			{status?.uri && status?.uri !== "n/a" && (
				<BaseIconLink label={status.uri} href={status.uri} target="_blank">
					{status.uri.startsWith("file://") ? (
						<FileIcon />
					) : (
						<ExternalLinkIcon />
					)}
				</BaseIconLink>
			)}
		</>
	);
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
	target?: string;
}>;

const BaseIconLink: FC<BaseIconLinkProps> = ({
	href,
	isLoading,
	label,
	children,
	target,
	onClick,
}) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button variant="outline" size="icon-lg" asChild>
						<a
							target={target}
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
