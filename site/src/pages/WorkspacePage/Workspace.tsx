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
import type { FC } from "react";
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
	const tasks = workspace.latest_build.resources
		.flatMap(resource => resource.agents || [])
		.flatMap(agent => agent.tasks || [])
	const waitingForUserInput = workspace.latest_build.resources
		.flatMap(resource => resource.agents || []).filter(agent => agent.task_waiting_for_user_input).length >= 1

	// Function to get a random thinking message
	const getRandomThinkingMessage = () => {
		const messages = [
			"Analyzing code patterns and potential solutions...",
			"Searching through documentation and best practices...",
			"Connecting the dots between your requirements...",
			"Brewing some code magic for you...",
			"Exploring the digital universe for answers...",
			"Consulting my silicon brain cells...",
			"Calculating the optimal approach...",
			"Assembling the perfect solution...",
			"Processing at the speed of light...",
			"Translating your needs into code..."
		];
		return messages[Math.floor(Math.random() * messages.length)];
	};

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

					<div css={{ display: "flex", flexDirection: "row", gap: 24 }}>
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

						{tasks.length && (
							<div css={{
								background: "#09090b", padding: 20, borderRadius: 8, border: "1px solid rgba(78, 208, 126, 0.2)",
								maxHeight: 800, overflowY: "auto",
								minWidth: 360,
								display: "flex",
								flexDirection: "column",
							}}>
								{waitingForUserInput ? <div>
									<Alert severity="info" css={{
										marginBottom: 16,
									}}>
										<AlertTitle>The agent needs your input...</AlertTitle>
									</Alert>
								</div> : <div css={{
									display: "flex",
									flexDirection: "column",
									alignItems: "center",
									justifyContent: "center",
									padding: "16px 8px",
									background: "rgba(24, 24, 27, 0.6)",
									borderRadius: 8,
									marginBottom: 16
								}}>
									<div css={{
										display: "flex",
										alignItems: "center",
										gap: 12,
										marginBottom: 8
									}}>
										<div css={{
											display: "flex",
											alignItems: "center",
											justifyContent: "center"
										}}>
											<div css={{
												width: 24,
												height: 24,
												borderRadius: "50%",
												border: "3px solid rgba(158, 88, 255, 0.2)",
												borderTopColor: "#9e58ff",
												animation: "spin 1s linear infinite",
												"@keyframes spin": {
													to: { transform: "rotate(360deg)" }
												}
											}} />
										</div>
										<div css={{
											fontSize: 16,
											fontWeight: 500,
											color: "#fff"
										}}>
											Thinking...
										</div>
									</div>
									<div css={{
										fontSize: 12,
										color: "rgba(255, 255, 255, 0.6)",
										textAlign: "center",
										maxWidth: 280
									}}>
										{getRandomThinkingMessage()}
									</div>
								</div>}
								<div css={{ width: "300px" }}>
									{tasks.map((task, index) => (
										<div key={index} css={{
											background: "#18181b",
											padding: 8,
											borderRadius: 8,
											marginBottom: 8,
											display: "flex",
											flexDirection: "column",
											border: "1px solid rgba(255, 255, 255, 0.05)",
											boxShadow: "0 2px 4px rgba(0, 0, 0, 0.1)",
											transition: "transform 0.2s ease, box-shadow 0.2s ease",
										}}>
											<div css={{
												display: "flex",
												alignItems: "center",
												gap: 10,
												position: "relative",
											}}>
												<div css={{
													fontSize: "12px",
													color: "#fff",
													lineHeight: 1.4,
													margin: 4,
												}}>
													{task.icon} {task.summary}
												</div>
											</div>

											{task.url && (
												<a href={task.url} target="_blank" rel="noopener noreferrer" css={{
													display: "inline-flex",
													alignItems: "center",
													gap: 6,
													fontSize: "12px",
													color: "#9e58ff",
													textDecoration: "none",
													transition: "color 0.2s ease",

													":hover": {
														color: "#b47aff",
														textDecoration: "underline",
													}
												}}>
													<svg css={{ minWidth: 14, }} width="14" height="14" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
														<path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
														<path d="M15 3h6v6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
														<path d="M10 14L21 3" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
													</svg>
													<span css={{
														overflow: "hidden",
														textOverflow: "ellipsis",
														whiteSpace: "nowrap",
													}}>
														{task.url}
													</span>
												</a>
											)}

											<div css={{
												fontSize: "11px",
												color: "rgba(255, 255, 255, 0.5)",
												marginTop: "auto",
											}}>
												{new Date(task.created_at).toLocaleString()}
											</div>
										</div>
									))}
								</div>
							</div>

						)}

					</div>

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
