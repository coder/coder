import type { Interpolation, Theme } from "@emotion/react";
import { useTheme } from "@emotion/react";
import HistoryOutlined from "@mui/icons-material/HistoryOutlined";
import HubOutlined from "@mui/icons-material/HubOutlined";
import AlertTitle from "@mui/material/AlertTitle";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { SidebarIconButton } from "components/FullPageLayout/Sidebar";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { ProvisionerStatusAlert } from "modules/provisioners/ProvisionerStatusAlert";
import { AgentRow } from "modules/resources/AgentRow";
import { WorkspaceTimings } from "modules/workspaces/WorkspaceTiming/WorkspaceTimings";
import { useMemo, type FC } from "react";
import { useNavigate } from "react-router-dom";
import { HistorySidebar } from "./HistorySidebar";
import { ResourceMetadata } from "./ResourceMetadata";
import { ResourcesSidebar } from "./ResourcesSidebar";
import { WorkspaceBuildLogsSection } from "./WorkspaceBuildLogsSection";
import {
	ActiveTransition,
	WorkspaceBuildProgress,
} from "./WorkspaceBuildProgress";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";
import { WorkspaceTopbar } from "./WorkspaceTopbar";
import type { WorkspacePermissions } from "./permissions";
import { resourceOptionValue, useResourcesNav } from "./useResourcesNav";
import { AppStatuses } from "./AppStatuses";
import { WorkspaceApp } from "api/typesGenerated";
import { Box, Typography } from "@mui/material";

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
	buildLogs?: TypesGen.ProvisionerJobLog[];
	latestVersion?: TypesGen.TemplateVersion;
	permissions: WorkspacePermissions;
	isOwner: boolean;
	timings?: TypesGen.WorkspaceBuildTimings;
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
	timings,
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

	const workspaceRunning = workspace.latest_build.status === "running";
	const workspacePending = workspace.latest_build.status === "pending";
	const haveBuildLogs = (buildLogs ?? []).length > 0;
	const shouldShowBuildLogs = haveBuildLogs && !workspaceRunning;
	const provisionersHealthy =
		(workspace.latest_build.matched_provisioners?.available ?? 1) > 0;
	const shouldShowProvisionerAlert =
		workspacePending && !haveBuildLogs && !provisionersHealthy && !isRestarting;

	const hasAppStatus = useMemo(() => {
		return selectedResource?.agents?.some((agent) => {
			return agent.apps?.some((app) => {
				return app.statuses?.length > 0;
			});
		});
	}, [selectedResource]);

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
							handleClick={() => navigate("/templates")}
						/>
					)}

					{shouldShowProvisionerAlert && (
						<ProvisionerStatusAlert
							matchingProvisioners={
								workspace.latest_build.matched_provisioners?.count
							}
							availableProvisioners={
								workspace.latest_build.matched_provisioners?.available ?? 0
							}
							tags={workspace.latest_build.job.tags}
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

					{shouldShowBuildLogs && (
						<WorkspaceBuildLogsSection logs={buildLogs} />
					)}

					{/* Container for Agent Rows + Activity Sidebar */}
					{selectedResource && (
						<Box sx={{ display: "flex", gap: 3, alignItems: "flex-start" }}>
							{/* Left Side: Agent Rows */}
							<section
								css={{
									display: "flex",
									flexDirection: "column",
									gap: 24,
									flexGrow: 1,
									minWidth: 0 /* Prevent overflow */,
								}}
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

							{/* Right Side: Activity Box */}
							{hasAppStatus && (
								<Box
									sx={{
										// Mimic AgentRow styling but with subtler border
										border: `1px solid ${theme.palette.divider}`, // Use divider color
										borderRadius: "8px",
										boxShadow: theme.shadows[3],
										width: 360,
										flexShrink: 0,
										bgcolor: "background.default", // Add background color
										overflow: "hidden",
									}}
								>
									{/* Activity Header */}
									<Box
										sx={{
											display: "flex",
											justifyContent: "space-between",
											alignItems: "center",
											px: 2,
											pt: 1.5,
											pb: 1,
											bgcolor: "background.paper", // Add background to header
											borderBottom: `1px solid ${theme.palette.divider}`, // Add separator
										}}
									>
										<Typography
											sx={{
												fontWeight: 500,
												fontSize: 14,
											}}
										>
											Activity
										</Typography>
										<Typography variant="caption" color="text.secondary">
											{
												// Calculate total status count
												selectedResource.agents
													?.flatMap((agent) => agent.apps ?? [])
													.reduce(
														(count, app) => count + (app.statuses?.length ?? 0),
														0,
													)
											}{" "}
											Total
										</Typography>
									</Box>

									<Box
										sx={{
											maxHeight: 800,
											overflowY: "auto",
											// Thin scrollbar styles
											"&::-webkit-scrollbar": {
												width: "6px",
											},
											"&::-webkit-scrollbar-track": {
												background: theme.palette.background.paper, // Match header background
											},
											"&::-webkit-scrollbar-thumb": {
												backgroundColor: theme.palette.divider, // Use divider color
												borderRadius: "3px",
											},
											"&::-webkit-scrollbar-thumb:hover": {
												backgroundColor: theme.palette.text.secondary, // Darken on hover
											},
										}}
									>
										<AppStatuses
											apps={
												selectedResource.agents?.flatMap(
													(agent) => agent.apps ?? [],
												) as WorkspaceApp[]
											}
											workspace={workspace}
											agents={selectedResource.agents || []}
										/>
									</Box>
								</Box>
							)}
						</Box>
					)}

					<WorkspaceTimings
						provisionerTimings={timings?.provisioner_timings}
						agentScriptTimings={timings?.agent_script_timings}
						agentConnectionTimings={timings?.agent_connection_timings}
					/>
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
