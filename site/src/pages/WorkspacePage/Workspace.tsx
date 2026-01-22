import type * as TypesGen from "api/typesGenerated";
import type { WorkspaceAgentStatus } from "api/typesGenerated";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { SidebarIconButton } from "components/FullPageLayout/Sidebar";
import { Link } from "components/Link/Link";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { BlocksIcon, HistoryIcon } from "lucide-react";
import { ProvisionerStatusAlert } from "modules/provisioners/ProvisionerStatusAlert";
import { AgentRow } from "modules/resources/AgentRow";
import { WorkspaceTimings } from "modules/workspaces/WorkspaceTiming/WorkspaceTimings";
import type { FC } from "react";
import { useNavigate } from "react-router";
import type { WorkspacePermissions } from "../../modules/workspaces/permissions";
import { HistorySidebar } from "./HistorySidebar";
import { ResourceMetadata } from "./ResourceMetadata";
import { ResourcesSidebar } from "./ResourcesSidebar";
import { resourceOptionValue, useResourcesNav } from "./useResourcesNav";
import { WorkspaceBuildLogsSection } from "./WorkspaceBuildLogsSection";
import {
	getActiveTransitionStats,
	WorkspaceBuildProgress,
} from "./WorkspaceBuildProgress";
import { WorkspaceDeletedBanner } from "./WorkspaceDeletedBanner";
import { NotificationActionButton } from "./WorkspaceNotifications/Notifications";
import { findTroubleshootingURL } from "./WorkspaceNotifications/WorkspaceNotifications";
import { WorkspaceTopbar } from "./WorkspaceTopbar";

interface WorkspaceProps {
	workspace: TypesGen.Workspace;
	template: TypesGen.Template;
	permissions: WorkspacePermissions;
	isUpdating: boolean;
	isRestarting: boolean;
	buildLogs?: TypesGen.ProvisionerJobLog[];
	latestVersion?: TypesGen.TemplateVersion;
	timings?: TypesGen.WorkspaceBuildTimings;
	sharingDisabled?: boolean;
	handleStart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleStop: () => void;
	handleRestart: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleUpdate: () => void;
	handleCancel: () => void;
	handleDormantActivate: () => void;
	handleToggleFavorite: () => void;
	handleRetry: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
	handleDebug: (buildParameters?: TypesGen.WorkspaceBuildParameter[]) => void;
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: FC<WorkspaceProps> = ({
	workspace,
	isUpdating,
	isRestarting,
	template,
	buildLogs,
	latestVersion,
	permissions,
	timings,
	sharingDisabled,
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
	const navigate = useNavigate();

	const transitionStats =
		template !== undefined
			? getActiveTransitionStats(template, workspace)
			: undefined;

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
	const troubleshootingURL = findTroubleshootingURL(workspace.latest_build);

	return (
		<div className="flex flex-col flex-1 min-h-0">
			<WorkspaceTopbar
				workspace={workspace}
				template={template}
				permissions={permissions}
				latestVersion={latestVersion}
				isUpdating={isUpdating}
				isRestarting={isRestarting}
				sharingDisabled={sharingDisabled}
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

			<div className="flex flex-1 min-h-0">
				<div className="flex">
					<div className="flex flex-col h-full overflow-y-auto border-solid border-0 border-r border-r-border">
						<SidebarIconButton
							isActive={sidebarOption.value === "resources"}
							onClick={() => {
								setSidebarOption("resources");
							}}
						>
							<BlocksIcon className="size-icon-sm" />
							<span className="sr-only">Resources</span>
						</SidebarIconButton>
						<SidebarIconButton
							isActive={sidebarOption.value === "history"}
							onClick={() => {
								setSidebarOption("history");
							}}
						>
							<HistoryIcon className="size-icon-sm" />
							<span className="sr-only">History</span>
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
				</div>

				<div
					style={{
						background: `radial-gradient(
			circle at 1px 1px,
			hsl(var(--surface-invert-secondary)) 0,
			transparent 1px
		) -2px -2px / 16px 16px`,
					}}
					className="p-8 overflow-y-auto relative w-full"
				>
					{selectedResource && (
						<ResourceMetadata
							resource={selectedResource}
							className="-mx-8 -mt-8 mb-6"
						/>
					)}
					<div className="flex flex-col gap-6 max-w-[1200px] m-auto">
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
							<Alert severity="error" prominent>
								<AlertTitle>Workspace build failed</AlertTitle>
								<AlertDetail>{workspace.latest_build.job.error}</AlertDetail>
							</Alert>
						)}

						{!workspace.health.healthy && (
							<UnhealthyWorkspaceAlert
								workspace={workspace}
								troubleshootingURL={troubleshootingURL}
							/>
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

						{selectedResource && (
							<section className="flex flex-col gap-6 flex-grow min-w-0">
								{selectedResource.agents
									// If an agent has a `parent_id`, that means it is
									// child of another agent. We do not want these agents
									// to be displayed at the top-level on this page. We
									// want them to display _as children_ of their parents.
									?.filter((agent) => agent.parent_id === null)
									.map((agent) => (
										<AgentRow
											key={agent.id}
											agent={agent}
											subAgents={selectedResource.agents?.filter(
												(a) => a.parent_id === agent.id,
											)}
											workspace={workspace}
											template={template}
											onUpdateAgent={handleUpdate} // On updating the workspace the agent version is also updated
										/>
									))}

								{(!selectedResource.agents ||
									selectedResource.agents?.length === 0) && (
									<div className="flex justify-center items-center w-full h-full">
										<div>
											<h4 className="text-base font-medium">
												No agents are currently assigned to this resource.
											</h4>
										</div>
									</div>
								)}
							</section>
						)}

						<WorkspaceTimings
							provisionerTimings={timings?.provisioner_timings}
							agentScriptTimings={timings?.agent_script_timings}
							agentConnectionTimings={timings?.agent_connection_timings}
						/>
					</div>
				</div>
			</div>
		</div>
	);
};

interface UnhealthyWorkspaceAlertProps {
	workspace: TypesGen.Workspace;
	troubleshootingURL: string | undefined;
}

const UnhealthyWorkspaceAlert: FC<UnhealthyWorkspaceAlertProps> = ({
	workspace,
	troubleshootingURL,
}) => {
	const failingAgentCount = workspace.health.failing_agents.length;
	const failureSet = new Set<WorkspaceAgentStatus>();

	workspace.latest_build.resources.forEach((resource) => {
		resource.agents?.forEach((agent) => {
			failureSet.add(agent.status);
		});
	});

	var title = "Workspace agents are not connected";
	var message = "Your workspace cannot be used until an agent connects.";

	// Disconnected is a more serious failure than timeout, so we can
	// prioritize handling it first.
	if (failureSet.has("disconnected")) {
		title = "Workspace agents are disconnected";
		message =
			"The agents have disconnected. If logs are streaming, the agent may still connect if you wait. Otherwise restarting the workspace can be done to try again.";

	} else if (failureSet.has("timeout")) {
		// Handle timeout case
		title = "Workspace agents have timed out";
		message =
			"The agents did not connect within the expected time. If logs are still streaming, they may finish connecting if you wait. Otherwise, restart the workspace to try again.";
	}

	return (
		<Alert severity="warning" prominent>
			<AlertTitle>{title}</AlertTitle>
			<AlertDetail>
				<p>Your workspace is running but{" "}
					{failingAgentCount > 1
						? `${failingAgentCount} agents have not connected yet.`
						: "the agent has not connected yet."}
					.{" "}</p>
				<p>{message}</p>
				<p>
					{troubleshootingURL && (
						<Link href={troubleshootingURL} target="_blank">
							View docs to troubleshoot
						</Link>
					)}
				</p>
			</AlertDetail>
		</Alert>
	);
};

const countAgents = (resource: TypesGen.WorkspaceResource) => {
	return resource.agents ? resource.agents.length : 0;
};
