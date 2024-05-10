import { useTheme } from "@emotion/react";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import MonetizationOnOutlined from "@mui/icons-material/MonetizationOnOutlined";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router-dom";
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
import { WorkspaceStatusBadge } from "modules/workspaces/WorkspaceStatusBadge/WorkspaceStatusBadge";
import { displayDormantDeletion } from "utils/dormant";
import type { WorkspacePermissions } from "./permissions";
import { WorkspaceActions } from "./WorkspaceActions/WorkspaceActions";
import { WorkspaceNotifications } from "./WorkspaceNotifications/WorkspaceNotifications";
import { WorkspaceScheduleControls } from "./WorkspaceScheduleControls";

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
  const theme = useTheme();

  // Quota
  const hasDailyCost = workspace.latest_build.daily_cost > 0;
  const { data: quota } = useQuery({
    ...workspaceQuota(workspace.owner_name),
    enabled: hasDailyCost,
  });

  // Dormant
  const { entitlements } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const shouldDisplayDormantData = displayDormantDeletion(
    workspace,
    allowAdvancedScheduling,
  );

  const isImmutable =
    workspace.latest_build.status === "deleted" ||
    workspace.latest_build.status === "deleting";

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
          padding: "12px 0 12px 16px",
        }}
      >
        <TopbarData>
          <UserAvatar
            size="xs"
            username={workspace.owner_name}
            avatarURL={workspace.owner_avatar_url}
          />
          <Tooltip title="Owner">
            <span>{workspace.owner_name}</span>
          </Tooltip>
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
              anchorOrigin={{
                vertical: "bottom",
                horizontal: "center",
              }}
              transformOrigin={{
                vertical: "top",
                horizontal: "center",
              }}
            >
              <AvatarData
                title={
                  <Link
                    component={RouterLink}
                    to={`/templates/${workspace.template_name}`}
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
                    to={`/templates/${workspace.template_name}/versions/${workspace.latest_build.template_version_name}`}
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

        {!isImmutable && (
          <WorkspaceScheduleControls
            workspace={workspace}
            template={template}
            canUpdateSchedule={
              canUpdateWorkspace && template.allow_user_autostop
            }
          />
        )}

        {shouldDisplayDormantData && (
          <TopbarData>
            <TopbarIcon>
              <DeleteOutline />
            </TopbarIcon>
            <Link
              component={RouterLink}
              to={`/templates/${workspace.template_name}/settings/schedule`}
              title="Schedule settings"
              css={{ color: "inherit" }}
            >
              Deletion on {new Date(workspace.deleting_at!).toLocaleString()}
            </Link>
          </TopbarData>
        )}

        {quota && quota.budget > 0 && (
          <TopbarData>
            <TopbarIcon>
              <Tooltip title="Daily usage">
                <MonetizationOnOutlined aria-label="Daily usage" />
              </Tooltip>
            </TopbarIcon>
            <span>
              {workspace.latest_build.daily_cost}{" "}
              <span css={{ color: theme.palette.text.secondary }}>
                credits of
              </span>{" "}
              {quota.budget}
            </span>
          </TopbarData>
        )}
      </div>

      <div
        css={{
          marginLeft: "auto",
          display: "flex",
          alignItems: "center",
          gap: 12,
        }}
      >
        {!isImmutable && (
          <>
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
          </>
        )}
      </div>
    </Topbar>
  );
};
