import { useTheme } from "@emotion/react";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import QuotaIcon from "@mui/icons-material/MonetizationOnOutlined";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import { workspaceQuota } from "api/queries/workspaceQuota";
import type * as TypesGen from "api/typesGenerated";
import { ExternalAvatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import {
	Topbar,
	TopbarAvatar,
	TopbarData,
	TopbarDivider,
	TopbarIcon,
	TopbarIconButton,
} from "components/FullPageLayout/Topbar";
import { HelpTooltipContent } from "components/HelpTooltip/HelpTooltip";
import { Popover, PopoverTrigger } from "components/Popover/Popover";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { useDashboard } from "modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "modules/navigation";
import { WorkspaceStatusBadge } from "modules/workspaces/WorkspaceStatusBadge/WorkspaceStatusBadge";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router-dom";
import { displayDormantDeletion } from "utils/dormant";
import { WorkspaceActions } from "./WorkspaceActions/WorkspaceActions";
import { WorkspaceNotifications } from "./WorkspaceNotifications/WorkspaceNotifications";
import { WorkspaceScheduleControls } from "./WorkspaceScheduleControls";
import type { WorkspacePermissions } from "./permissions";

export type WorkspaceError =
	| "getBuildsError"
	| "buildError"
	| "cancellationError";

export type WorkspaceErrors = Partial<Record<WorkspaceError, unknown>>;

export interface WorkspaceProps {
	handleStart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleStop: () => void;
	handleRestart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleDelete: () => void;
	handleUpdate: () => void;
	handleCancel: () => void;
	handleSettings: () => void;
	handleChangeVersion: () => void;
	handleDormantActivate: () => void;
	isUpdating: boolean;
	isRestarting: boolean;
	workspace: TypesGen.Workspace;
	canUpdateWorkspace: boolean;
	canChangeVersions: boolean;
	canDebugMode: boolean;
	handleRetry: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleDebug: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	isOwner: boolean;
	template: TypesGen.Template;
	permissions: WorkspacePermissions;
	latestVersion?: TypesGen.TemplateVersion;
	handleToggleFavorite: () => void;
}

export const WorkspaceTopbar: FC<WorkspaceProps> = ({
	handleStart,
	handleStop,
	handleRestart,
	handleDelete,
	handleUpdate,
	handleCancel,
	handleSettings,
	handleChangeVersion,
	handleDormantActivate,
	handleToggleFavorite,
	workspace,
	isUpdating,
	isRestarting,
	canUpdateWorkspace,
	canChangeVersions,
	canDebugMode,
	handleRetry,
	handleDebug,
	isOwner,
	template,
	latestVersion,
	permissions,
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

	const isImmutable =
		workspace.latest_build.status === "deleted" ||
		workspace.latest_build.status === "deleting";

	const templateLink = getLink(
		linkToTemplate(workspace.organization_name, workspace.template_name),
	);

	return (
		<Topbar css={{ gridArea: "topbar" }}>
			<Tooltip title="Back to workspaces">
				<TopbarIconButton component={RouterLink} to="/workspaces">
					<ArrowBackOutlined />
				</TopbarIconButton>
			</Tooltip>

			<div
				css={{
					display: "flex",
					alignItems: "center",
					columnGap: 24,
					rowGap: 8,
					flexWrap: "wrap",
					// 12px - It is needed to keep vertical spacing when the content is wrapped
					padding: "12px",
					marginRight: "auto",
				}}
			>
				<TopbarData>
					<Tooltip title={`Owner: ${workspace.owner_name}`}>
						<span
							css={{
								display: "flex",
								flexFlow: "row nowrap",
								gap: "8px",
								maxWidth: "160px",
								textOverflow: "ellipsis",
								overflowX: "hidden",
								whiteSpace: "nowrap",
								cursor: "default",
							}}
						>
							<UserAvatar
								size="xs"
								username={workspace.owner_name}
								avatarURL={workspace.owner_avatar_url}
							/>

							{workspace.owner_name}
						</span>
					</Tooltip>

					{showOrganizations && (
						<>
							<TopbarDivider />

							<Tooltip title={`Organization: ${workspace.organization_name}`}>
								<span
									css={{
										display: "flex",
										flexFlow: "row nowrap",
										gap: "8px",
										maxWidth: "160px",
										textOverflow: "ellipsis",
										overflowX: "hidden",
										whiteSpace: "nowrap",
										cursor: "default",
									}}
								>
									{activeOrg && (
										<UserAvatar
											size="xs"
											username={activeOrg.display_name}
											avatarURL={activeOrg.icon}
										/>
									)}

									{workspace.organization_name}
								</span>
							</Tooltip>
						</>
					)}

					<TopbarDivider />

					<Popover mode="hover">
						<PopoverTrigger>
							<span
								css={{
									display: "flex",
									alignItems: "center",
									gap: 8,
									cursor: "default",
									padding: "4px 0",
								}}
							>
								<TopbarAvatar src={workspace.template_icon} />
								<span css={{ fontWeight: 500 }}>{workspace.name}</span>
							</span>
						</PopoverTrigger>

						<HelpTooltipContent
							anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
							transformOrigin={{ vertical: "top", horizontal: "center" }}
						>
							<AvatarData
								title={
									<Link
										component={RouterLink}
										to={templateLink}
										css={{ color: "inherit" }}
									>
										{workspace.template_display_name.length > 0
											? workspace.template_display_name
											: workspace.template_name}
									</Link>
								}
								subtitle={
									<Link
										component={RouterLink}
										to={`${templateLink}/versions/${workspace.latest_build.template_version_name}`}
										css={{ color: "inherit" }}
									>
										{workspace.latest_build.template_version_name}
									</Link>
								}
								avatar={
									workspace.template_icon !== "" && (
										<ExternalAvatar
											src={workspace.template_icon}
											variant="square"
											fitImage
										/>
									)
								}
							/>
						</HelpTooltipContent>
					</Popover>
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
								? `See affected workspaces for ${workspace.organization_name}`
								: "See affected workspaces"
						}
					>
						<TopbarData>
							<TopbarIcon>
								<QuotaIcon aria-label="Daily usage" />
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
							<DeleteOutline />
						</TopbarIcon>
						<Link
							component={RouterLink}
							to={`${templateLink}/settings/schedule`}
							title="Schedule settings"
							css={{ color: "inherit" }}
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
				<div
					css={{
						display: "flex",
						alignItems: "center",
						gap: 8,
					}}
				>
					<WorkspaceScheduleControls
						workspace={workspace}
						template={template}
						canUpdateSchedule={
							canUpdateWorkspace && template.allow_user_autostop
						}
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
					<WorkspaceStatusBadge workspace={workspace} />
					<WorkspaceActions
						workspace={workspace}
						handleStart={handleStart}
						handleStop={handleStop}
						handleRestart={handleRestart}
						handleDelete={handleDelete}
						handleUpdate={handleUpdate}
						handleCancel={handleCancel}
						handleSettings={handleSettings}
						handleRetry={handleRetry}
						handleDebug={handleDebug}
						handleChangeVersion={handleChangeVersion}
						handleDormantActivate={handleDormantActivate}
						handleToggleFavorite={handleToggleFavorite}
						canDebug={canDebugMode}
						canChangeVersions={canChangeVersions}
						isUpdating={isUpdating}
						isRestarting={isRestarting}
						isOwner={isOwner}
					/>
				</div>
			)}
		</Topbar>
	);
};
