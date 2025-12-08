import { useTheme } from "@emotion/react";
import Link from "@mui/material/Link";
import { workspaceQuota } from "api/queries/workspaceQuota";
import type * as TypesGen from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { CopyButton } from "components/CopyButton/CopyButton";
import {
	Topbar,
	TopbarAvatar,
	TopbarData,
	TopbarDivider,
	TopbarIcon,
	TopbarIconButton,
} from "components/FullPageLayout/Topbar";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ChevronLeftIcon, CircleDollarSign, TrashIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "modules/navigation";
import { WorkspaceStatusIndicator } from "modules/workspaces/WorkspaceStatusIndicator/WorkspaceStatusIndicator";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { displayDormantDeletion } from "utils/dormant";
import type { WorkspacePermissions } from "../../modules/workspaces/permissions";
import { WorkspaceActions } from "./WorkspaceActions/WorkspaceActions";
import { WorkspaceNotifications } from "./WorkspaceNotifications/WorkspaceNotifications";
import { WorkspaceScheduleControls } from "./WorkspaceScheduleControls";

interface WorkspaceProps {
	isUpdating: boolean;
	isRestarting: boolean;
	workspace: TypesGen.Workspace;
	template: TypesGen.Template;
	permissions: WorkspacePermissions;
	latestVersion?: TypesGen.TemplateVersion;
	handleStart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleStop: () => void;
	handleRestart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleUpdate: () => void;
	handleCancel: () => void;
	handleDormantActivate: () => void;
	handleRetry: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleDebug: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleToggleFavorite: () => void;
}

export const WorkspaceTopbar: FC<WorkspaceProps> = ({
	workspace,
	template,
	latestVersion,
	permissions,
	isUpdating,
	isRestarting,
	handleStart,
	handleStop,
	handleRestart,
	handleUpdate,
	handleCancel,
	handleDormantActivate,
	handleToggleFavorite,
	handleRetry,
	handleDebug,
}) => {
	const { entitlements, organizations, showOrganizations } = useDashboard();
	const getLink = useLinks();
	const theme = useTheme();

	// Quota
	const hasDailyCost = workspace.latest_build.daily_cost > 0;
	const { data: quota } = useQuery({
		...workspaceQuota(workspace.organization_name, workspace.owner_name),

		// Don't need to tie the enabled condition to showOrganizations because
		// even if the customer hasn't enabled the orgs enterprise feature, all
		// workspaces have an associated organization under the hood
		enabled: hasDailyCost,
	});

	// Dormant
	const allowAdvancedScheduling =
		entitlements.features.advanced_template_scheduling.enabled;
	// This check can be removed when https://github.com/coder/coder/milestone/19
	// is merged up
	const shouldDisplayDormantData = displayDormantDeletion(
		workspace,
		allowAdvancedScheduling,
	);

	const activeOrg = organizations.find(
		(org) => org.id === workspace.organization_id,
	);

	const orgDisplayName = activeOrg?.display_name || workspace.organization_name;

	const isImmutable =
		workspace.latest_build.status === "deleted" ||
		workspace.latest_build.status === "deleting";

	const templateLink = getLink(
		linkToTemplate(workspace.organization_name, workspace.template_name),
	);

	return (
		<Topbar className="[grid-area:topbar]">
			<Tooltip>
				<TooltipTrigger asChild>
					<TopbarIconButton component={RouterLink} to="/workspaces">
						<ChevronLeftIcon className="size-icon-sm" />
					</TopbarIconButton>
				</TooltipTrigger>
				<TooltipContent side="bottom">Back to workspaces</TooltipContent>
			</Tooltip>

			<div className="flex items-center gap-x-6 gap-y-2 flex-wrap p-3 mr-auto">
				<TopbarData>
					<OwnerBreadcrumb
						ownerName={workspace.owner_name}
						ownerAvatarUrl={workspace.owner_avatar_url}
					/>

					{showOrganizations && (
						<>
							<TopbarDivider />
							<OrganizationBreadcrumb
								orgName={orgDisplayName}
								orgIconUrl={activeOrg?.icon}
								orgPageUrl={`/organizations/${encodeURIComponent(workspace.organization_name)}`}
							/>
						</>
					)}

					<TopbarDivider />

					<WorkspaceBreadcrumb
						workspaceName={workspace.name}
						templateIconUrl={workspace.template_icon}
						rootTemplateUrl={templateLink}
						templateVersionName={workspace.latest_build.template_version_name}
						templateDisplayName={
							workspace.template_display_name || workspace.template_name
						}
						latestBuildVersionName={
							workspace.latest_build.template_version_name
						}
					/>
				</TopbarData>

				{quota && quota.budget > 0 && (
					<Link
						component={RouterLink}
						className="text-inherit"
						to={
							showOrganizations
								? `/workspaces?filter=organization:${encodeURIComponent(workspace.organization_name)}`
								: "/workspaces"
						}
						title={
							showOrganizations
								? `See affected workspaces for ${orgDisplayName}`
								: "See affected workspaces"
						}
					>
						<TopbarData>
							<TopbarIcon>
								<CircleDollarSign
									className="size-icon-sm"
									aria-label="Daily usage"
								/>
							</TopbarIcon>

							<span>
								{workspace.latest_build.daily_cost}{" "}
								<span className="text-content-secondary">credits of</span>{" "}
								{quota.budget}
							</span>
						</TopbarData>
					</Link>
				)}

				{shouldDisplayDormantData && (
					<TopbarData>
						<TopbarIcon>
							<TrashIcon />
						</TopbarIcon>
						<Link
							component={RouterLink}
							to={`${templateLink}/settings/schedule`}
							title="Schedule settings"
							className="text-inherit"
						>
							{workspace.deleting_at ? (
								<>
									Deletion on {new Date(workspace.deleting_at).toLocaleString()}
								</>
							) : (
								"Deletion soon"
							)}
						</Link>
					</TopbarData>
				)}
			</div>

			{!isImmutable && (
				<div className="flex items-center gap-4">
					<WorkspaceScheduleControls
						workspace={workspace}
						template={template}
						canUpdateSchedule={permissions.updateWorkspace}
					/>

					<WorkspaceNotifications
						workspace={workspace}
						template={template}
						latestVersion={latestVersion}
						permissions={permissions}
						onRestartWorkspace={handleRestart}
						onUpdateWorkspace={handleUpdate}
						onActivateWorkspace={handleDormantActivate}
					/>

					<WorkspaceStatusIndicator workspace={workspace} />

					<WorkspaceActions
						workspace={workspace}
						permissions={permissions}
						isUpdating={isUpdating}
						isRestarting={isRestarting}
						handleStart={handleStart}
						handleStop={handleStop}
						handleRestart={handleRestart}
						handleUpdate={handleUpdate}
						handleCancel={handleCancel}
						handleRetry={handleRetry}
						handleDebug={handleDebug}
						handleDormantActivate={handleDormantActivate}
						handleToggleFavorite={handleToggleFavorite}
					/>
				</div>
			)}
		</Topbar>
	);
};

type OwnerBreadcrumbProps = Readonly<{
	ownerName: string;
	ownerAvatarUrl: string;
}>;

const OwnerBreadcrumb: FC<OwnerBreadcrumbProps> = ({
	ownerName,
	ownerAvatarUrl,
}) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild>
				<span className="flex items-center flex-row flex-nowrap gap-2 max-w-40 whitespace-nowrap cursor-default">
					<Avatar size="sm" fallback={ownerName} src={ownerAvatarUrl} />
					<span className="overflow-x-hidden text-ellipsis">{ownerName}</span>
				</span>
			</HelpTooltipTrigger>

			<HelpTooltipContent align="center">
				<AvatarData title={ownerName} subtitle="Owner" src={ownerAvatarUrl} />
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

type OrganizationBreadcrumbProps = Readonly<{
	orgName: string;
	orgPageUrl?: string;
	orgIconUrl?: string;
}>;

const OrganizationBreadcrumb: FC<OrganizationBreadcrumbProps> = ({
	orgName,
	orgPageUrl,
	orgIconUrl,
}) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild>
				<span className="flex items-center flex-row flex-nowrap gap-2 max-w-40 whitespace-nowrap cursor-default">
					<Avatar
						size="sm"
						variant="icon"
						src={orgIconUrl}
						fallback={orgName}
					/>
					<span className="overflow-x-hidden text-ellipsis">{orgName}</span>
				</span>
			</HelpTooltipTrigger>

			<HelpTooltipContent align="center">
				<AvatarData
					title={
						orgPageUrl ? (
							<Link
								component={RouterLink}
								to={orgPageUrl}
								className="text-inherit"
							>
								{orgName}
							</Link>
						) : (
							orgName
						)
					}
					subtitle="Organization"
					avatar={
						orgIconUrl && (
							<Avatar variant="icon" src={orgIconUrl} fallback={orgName} />
						)
					}
					imgFallbackText={orgName}
				/>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

type WorkspaceBreadcrumbProps = Readonly<{
	workspaceName: string;
	templateIconUrl: string;
	rootTemplateUrl: string;
	templateVersionName: string;
	latestBuildVersionName: string;
	templateDisplayName: string;
}>;

const WorkspaceBreadcrumb: FC<WorkspaceBreadcrumbProps> = ({
	workspaceName,
	templateIconUrl,
	rootTemplateUrl,
	templateVersionName,
	latestBuildVersionName,
	templateDisplayName,
}) => {
	return (
		<div className="flex items-center">
			<HelpTooltip>
				<HelpTooltipTrigger asChild>
					<span className="flex items-center flex-row flex-nowrap gap-2 max-w-40 whitespace-nowrap cursor-default">
						<TopbarAvatar
							src={templateIconUrl}
							fallback={templateDisplayName}
						/>

						<span className="overflow-x-hidden text-ellipsis font-medium">
							{workspaceName}
						</span>
					</span>
				</HelpTooltipTrigger>

				<HelpTooltipContent align="center">
					<AvatarData
						title={
							<Link
								component={RouterLink}
								to={rootTemplateUrl}
								className="text-inherit"
							>
								{templateDisplayName}
							</Link>
						}
						subtitle={
							<Link
								component={RouterLink}
								to={`${rootTemplateUrl}/versions/${encodeURIComponent(templateVersionName)}`}
								className="text-inherit"
							>
								Version: {latestBuildVersionName}
							</Link>
						}
						avatar={
							<Avatar
								variant="icon"
								src={templateIconUrl}
								fallback={templateDisplayName}
							/>
						}
						imgFallbackText={templateDisplayName}
					/>
				</HelpTooltipContent>
			</HelpTooltip>
			<CopyButton text={workspaceName} label="Copy workspace name" />
		</div>
	);
};
