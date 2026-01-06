import { type Interpolation, type Theme, useTheme } from "@emotion/react";
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
import { formatDate } from "utils/time";
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
		<Topbar css={{ gridArea: "topbar" }}>
			<Tooltip>
				<TooltipTrigger asChild>
					<TopbarIconButton component={RouterLink} to="/workspaces">
						<ChevronLeftIcon className="size-icon-sm" />
					</TopbarIconButton>
				</TooltipTrigger>
				<TooltipContent side="bottom">Back to workspaces</TooltipContent>
			</Tooltip>

			<div css={styles.topbarLeft}>
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
						css={{ color: "inherit" }}
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
								<span css={{ color: theme.palette.text.secondary }}>
									credits of
								</span>{" "}
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
							css={{ color: "inherit" }}
						>
							{workspace.deleting_at ? (
								<>Deletion on {formatDate(new Date(workspace.deleting_at))}</>
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
				<span css={styles.breadcrumbSegment}>
					<Avatar size="md" fallback={ownerName} src={ownerAvatarUrl} />
					<span css={styles.breadcrumbText}>{ownerName}</span>
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
				<span css={styles.breadcrumbSegment}>
					<Avatar
						size="md"
						variant="icon"
						src={orgIconUrl}
						fallback={orgName}
					/>
					<span css={styles.breadcrumbText}>{orgName}</span>
				</span>
			</HelpTooltipTrigger>

			<HelpTooltipContent align="center">
				<AvatarData
					title={
						orgPageUrl ? (
							<Link
								component={RouterLink}
								to={orgPageUrl}
								css={{ color: "inherit" }}
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
							<Avatar
								variant="icon"
								src={orgIconUrl}
								fallback={orgName}
								size="md"
							/>
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
					<span css={styles.breadcrumbSegment}>
						<TopbarAvatar
							src={templateIconUrl}
							fallback={templateDisplayName}
						/>

						<span css={[styles.breadcrumbText, { fontWeight: 500 }]}>
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
								css={{ color: "inherit" }}
							>
								{templateDisplayName}
							</Link>
						}
						subtitle={
							<Link
								component={RouterLink}
								to={`${rootTemplateUrl}/versions/${encodeURIComponent(templateVersionName)}`}
								css={{ color: "inherit" }}
							>
								Version: {latestBuildVersionName}
							</Link>
						}
						avatar={
							<Avatar
								variant="icon"
								src={templateIconUrl}
								fallback={templateDisplayName}
								size="md"
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

const styles = {
	topbarLeft: {
		display: "flex",
		alignItems: "center",
		columnGap: 24,
		rowGap: 8,
		flexWrap: "wrap",
		// 12px - It is needed to keep vertical spacing when the content is wrapped
		padding: "12px",
		marginRight: "auto",
	},

	breadcrumbSegment: {
		display: "flex",
		alignItems: "center",
		flexFlow: "row nowrap",
		gap: "8px",
		maxWidth: "160px",
		whiteSpace: "nowrap",
		cursor: "default",
	},

	breadcrumbText: {
		overflowX: "hidden",
		textOverflow: "ellipsis",
	},
} satisfies Record<string, Interpolation<Theme>>;
