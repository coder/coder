import type { Interpolation, Theme } from "@emotion/react";
import { useTheme } from "@emotion/react";
import HistoryOutlined from "@mui/icons-material/HistoryOutlined";
import HubOutlined from "@mui/icons-material/HubOutlined";
import AlertTitle from "@mui/material/AlertTitle";
import type { FC } from "react";
import { useNavigate } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { SidebarIconButton } from "components/FullPageLayout/Sidebar";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { AgentRow } from "modules/resources/AgentRow";
import { HistorySidebar } from "./HistorySidebar";
import type { WorkspacePermissions } from "./permissions";
import { ResourceMetadata } from "./ResourceMetadata";
import { ResourcesSidebar } from "./ResourcesSidebar";
import { resourceOptionValue, useResourcesNav } from "./useResourcesNav";
import {
  ActiveTransition,
  WorkspaceBuildProgress,
} from "./WorkspaceBuildProgress";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";
import { WorkspaceTopbar } from "./WorkspaceTopbar";

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
  handleToggleFavorite: () => void;
  isUpdating: boolean;
  isRestarting: boolean;
  workspace: TypesGen.Workspace;
  canChangeVersions: boolean;
  hideSSHButton?: boolean;
  hideVSCodeDesktopButton?: boolean;
  buildInfo?: TypesGen.BuildInfoResponse;
  sshPrefix?: string;
  template: TypesGen.Template;
  canDebugMode: boolean;
  handleRetry: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
  handleDebug: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
  buildLogs?: React.ReactNode;
  latestVersion?: TypesGen.TemplateVersion;
  permissions: WorkspacePermissions;
  isOwner: boolean;
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: FC<WorkspaceProps> = ({
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
  canChangeVersions,
  hideSSHButton,
  hideVSCodeDesktopButton,
  buildInfo,
  sshPrefix,
  template,
  canDebugMode,
  handleRetry,
  handleDebug,
  buildLogs,
  latestVersion,
  permissions,
  isOwner,
}) => {
  const navigate = useNavigate();
  const theme = useTheme();

  const transitionStats =
    template !== undefined ? ActiveTransition(template, workspace) : undefined;

  const sidebarOption = useSearchParamsKey({ key: "sidebar" });
  const setSidebarOption = (newOption: string) => {
    if (sidebarOption.value === newOption) {
      sidebarOption.deleteValue();
    } else {
      sidebarOption.setValue(newOption);
    }
  };

  const resources = [...workspace.latest_build.resources].sort(
    (a, b) => countAgents(b) - countAgents(a),
  );
  const resourcesNav = useResourcesNav(resources);
  const selectedResource = resources.find(
    (r) => resourceOptionValue(r) === resourcesNav.value,
  );

  return (
    <div
      css={{
        flex: 1,
        display: "grid",
        gridTemplate: `
          "topbar topbar topbar" auto
          "leftbar sidebar content" 1fr / auto auto 1fr
        `,
        // We need this to make the sidebar scrollable
        overflow: "hidden",
      }}
    >
      <WorkspaceTopbar
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
        canDebugMode={canDebugMode}
        canChangeVersions={canChangeVersions}
        isUpdating={isUpdating}
        isRestarting={isRestarting}
        canUpdateWorkspace={permissions.updateWorkspace}
        isOwner={isOwner}
        template={template}
        permissions={permissions}
        latestVersion={latestVersion}
      />

      <div
        css={{
          gridArea: "leftbar",
          height: "100%",
          overflowY: "auto",
          borderRight: `1px solid ${theme.palette.divider}`,
          display: "flex",
          flexDirection: "column",
        }}
      >
        <SidebarIconButton
          isActive={sidebarOption.value === "resources"}
          onClick={() => {
            setSidebarOption("resources");
          }}
        >
          <HubOutlined />
        </SidebarIconButton>
        <SidebarIconButton
          isActive={sidebarOption.value === "history"}
          onClick={() => {
            setSidebarOption("history");
          }}
        >
          <HistoryOutlined />
        </SidebarIconButton>
      </div>

      {sidebarOption.value === "resources" && (
        <ResourcesSidebar
          failed={workspace.latest_build.status === "failed"}
          resources={resources}
          isSelected={resourcesNav.isSelected}
          onChange={resourcesNav.select}
        />
      )}
      {sidebarOption.value === "history" && (
        <HistorySidebar workspace={workspace} />
      )}

      <div css={[styles.content, styles.dotsBackground]}>
        {selectedResource && (
          <ResourceMetadata
            resource={selectedResource}
            css={{ margin: "-32px -32px 0 -32px", marginBottom: 24 }}
          />
        )}
        <div
          css={{
            display: "flex",
            flexDirection: "column",
            gap: 24,
            maxWidth: 24 * 50,
            margin: "auto",
          }}
        >
          {workspace.latest_build.status === "deleted" && (
            <WorkspaceDeletedBanner
              handleClick={() => navigate(`/templates`)}
            />
          )}

          {workspace.latest_build.job.error && (
            <Alert severity="error">
              <AlertTitle>Workspace build failed</AlertTitle>
              <AlertDetail>{workspace.latest_build.job.error}</AlertDetail>
            </Alert>
          )}

          {transitionStats !== undefined && (
            <WorkspaceBuildProgress
              workspace={workspace}
              transitionStats={transitionStats}
            />
          )}

          {buildLogs}

          {selectedResource && (
            <section
              css={{ display: "flex", flexDirection: "column", gap: 24 }}
            >
              {selectedResource.agents?.map((agent) => (
                <AgentRow
                  key={agent.id}
                  agent={agent}
                  workspace={workspace}
                  template={template}
                  sshPrefix={sshPrefix}
                  showApps={permissions.updateWorkspace}
                  showBuiltinApps={permissions.updateWorkspace}
                  hideSSHButton={hideSSHButton}
                  hideVSCodeDesktopButton={hideVSCodeDesktopButton}
                  serverVersion={buildInfo?.version || ""}
                  serverAPIVersion={buildInfo?.agent_api_version || ""}
                  onUpdateAgent={handleUpdate} // On updating the workspace the agent version is also updated
                />
              ))}

              {(!selectedResource.agents ||
                selectedResource.agents?.length === 0) && (
                <div
                  css={{
                    display: "flex",
                    justifyContent: "center",
                    alignItems: "center",
                    width: "100%",
                    height: "100%",
                  }}
                >
                  <div>
                    <h4 css={{ fontSize: 16, fontWeight: 500 }}>
                      No agents are currently assigned to this resource.
                    </h4>
                  </div>
                </div>
              )}
            </section>
          )}
        </div>
      </div>
    </div>
  );
};

const countAgents = (resource: TypesGen.WorkspaceResource) => {
  return resource.agents ? resource.agents.length : 0;
};

const styles = {
  content: {
    padding: 32,
    gridArea: "content",
    overflowY: "auto",
    position: "relative",
  },

  dotsBackground: (theme) => ({
    "--d": "1px",
    background: `
      radial-gradient(
        circle at
          var(--d)
          var(--d),

        ${theme.palette.dots} calc(var(--d) - 1px),
        ${theme.palette.background.default} var(--d)
      )
      -2px -2px / 16px 16px
    `,
  }),

  actions: (theme) => ({
    [theme.breakpoints.down("md")]: {
      flexDirection: "column",
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;
