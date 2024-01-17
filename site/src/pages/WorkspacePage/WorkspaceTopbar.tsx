import { Link as RouterLink } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { WorkspaceActions } from "pages/WorkspacePage/WorkspaceActions/WorkspaceActions";
import {
  Topbar,
  TopbarAvatar,
  TopbarData,
  TopbarDivider,
  TopbarIcon,
  TopbarIconButton,
} from "components/FullPageLayout/Topbar";
import Tooltip from "@mui/material/Tooltip";
import ArrowBackOutlined from "@mui/icons-material/ArrowBackOutlined";
import ScheduleOutlined from "@mui/icons-material/ScheduleOutlined";
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge";
import {
  WorkspaceScheduleControls,
  shouldDisplayScheduleControls,
} from "./WorkspaceScheduleControls";
import { workspaceQuota } from "api/queries/workspaceQuota";
import { useQuery } from "react-query";
import MonetizationOnOutlined from "@mui/icons-material/MonetizationOnOutlined";
import { useTheme } from "@mui/material/styles";
import Link from "@mui/material/Link";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { displayDormantDeletion } from "utils/dormant";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import PersonOutline from "@mui/icons-material/PersonOutline";
import { Popover, PopoverTrigger } from "components/Popover/Popover";
import { HelpTooltipContent } from "components/HelpTooltip/HelpTooltip";
import { AvatarData } from "components/AvatarData/AvatarData";
import { ExternalAvatar } from "components/Avatar/Avatar";
import { WorkspaceNotifications } from "./WorkspaceNotifications/WorkspaceNotifications";
import { WorkspacePermissions } from "./permissions";

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
  canRetryDebugMode: boolean;
  handleBuildRetry: () => void;
  handleBuildRetryDebug: () => void;
  isOwner: boolean;
  template: TypesGen.Template;
  permissions: WorkspacePermissions;
  latestVersion?: TypesGen.TemplateVersion;
}

export const WorkspaceTopbar = (props: WorkspaceProps) => {
  const {
    handleStart,
    handleStop,
    handleRestart,
    handleDelete,
    handleUpdate,
    handleCancel,
    handleSettings,
    handleChangeVersion,
    handleDormantActivate,
    workspace,
    isUpdating,
    isRestarting,
    canUpdateWorkspace,
    canChangeVersions,
    canRetryDebugMode,
    handleBuildRetry,
    handleBuildRetryDebug,
    isOwner,
    template,
    latestVersion,
    permissions,
  } = props;
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
          <TopbarIcon>
            <PersonOutline />
          </TopbarIcon>
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
              Deletion on{" "}
              <span data-chromatic="ignore">
                {new Date(workspace.deleting_at!).toLocaleString()}
              </span>
            </Link>
          </TopbarData>
        )}

        {shouldDisplayScheduleControls(workspace) && (
          <TopbarData>
            <TopbarIcon>
              <Tooltip title="Schedule">
                <ScheduleOutlined aria-label="Schedule" />
              </Tooltip>
            </TopbarIcon>
            <WorkspaceScheduleControls
              workspace={workspace}
              canUpdateSchedule={canUpdateWorkspace}
            />
          </TopbarData>
        )}

        {quota && (
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
          handleRetry={handleBuildRetry}
          handleRetryDebug={handleBuildRetryDebug}
          handleChangeVersion={handleChangeVersion}
          handleDormantActivate={handleDormantActivate}
          canRetryDebug={canRetryDebugMode}
          canChangeVersions={canChangeVersions}
          isUpdating={isUpdating}
          isRestarting={isRestarting}
          isOwner={isOwner}
        />
      </div>
    </Topbar>
  );
};
