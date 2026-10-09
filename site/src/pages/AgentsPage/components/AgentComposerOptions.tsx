import { cn } from "cn";
import type React from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import {
	AgentComposerBadges,
	AgentComposerPlanningBadge,
	type AttachedWorkspaceInfo,
	composerPillSizingClasses,
	type ToolBadgeData,
} from "./AgentComposerBadges";
import { AgentComposerOptionsMenu } from "./AgentComposerOptionsMenu";

export type { AttachedWorkspaceInfo } from "./AgentComposerBadges";

type AgentComposerOptionsProps = {
	isDisabled: boolean;
	showAgentSetupNotice?: boolean;
	hasContextUsage?: boolean;
	onAttachClick?: () => void;
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	planModeEnabled: boolean;
	onPlanModeToggle: (enabled: boolean) => void;
	manageAutomationsEnabled?: boolean;
	onManageAutomationsToggle?: (enabled: boolean) => void;
	isModelCatalogLoading: boolean;
	workspaceOptions?: ReadonlyArray<{
		id: string;
		name: string;
		owner_name: string;
		organization_id: string;
	}>;
	selectedWorkspaceId?: string | null;
	onWorkspaceChange?: (id: string | null) => void;
	chatOrganizationId?: string;
	isWorkspaceLoading?: boolean;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	selectedMCPServerIds?: readonly string[];
	onMCPSelectionChange?: (ids: string[]) => void;
	onMCPAuthComplete?: (serverId: string) => void;
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
	chatId?: string;
	sshCommand?: string;
	attachedWorkspace?: AttachedWorkspaceInfo;
	folder?: string;
};

/** Controlled model, workspace, and tool options arranged in the composer toolbar. */
export const AgentComposerOptions: React.FC<AgentComposerOptionsProps> = (
	props,
) => {
	const {
		isDisabled,
		planModeEnabled,
		onPlanModeToggle,
		workspaceOptions,
		selectedWorkspaceId,
		onWorkspaceChange,
		selectedMCPServerIds,
		onMCPSelectionChange,
		workspace,
		workspaceAgent,
		chatId,
		attachedWorkspace,
	} = props;
	const toggleMcp = (serverId: string, checked: boolean) => {
		if (!onMCPSelectionChange || !selectedMCPServerIds) return;
		onMCPSelectionChange(
			checked
				? [...selectedMCPServerIds, serverId]
				: selectedMCPServerIds.filter((id) => id !== serverId),
		);
	};
	const removeWorkspace = onWorkspaceChange
		? () => onWorkspaceChange(null)
		: undefined;
	const disablePlanMode = () => onPlanModeToggle(false);
	const enabledMcpServers =
		props.mcpServers?.filter((server) => server.enabled) ?? [];
	const activeMcpServers = enabledMcpServers.filter(
		(server) =>
			(server.availability === "force_on" ||
				selectedMCPServerIds?.includes(server.id)) &&
			!(server.auth_type === "oauth2" && !server.auth_connected),
	);
	const selectedWorkspace = workspaceOptions?.find(
		(item) => item.id === selectedWorkspaceId,
	);
	const linkedWorkspaceId = workspace?.id ?? attachedWorkspace?.id;
	const overflowPlanning = planModeEnabled && Boolean(props.hasContextUsage);

	const workspacePill =
		workspace && workspaceAgent && chatId
			? {
					badge: attachedWorkspace
						? ({
								kind: "attached-workspace",
								...attachedWorkspace,
							} satisfies ToolBadgeData)
						: ({
								kind: "workspace",
								name: workspace.name,
							} satisfies ToolBadgeData),
					props: {
						workspace,
						agent: workspaceAgent,
						chatId,
						sshCommand: props.sshCommand,
						folder: props.folder,
					},
				}
			: undefined;
	// Ordering controls which trailing badges move into the overflow menu.
	const badges: ToolBadgeData[] = [];
	if (overflowPlanning) badges.push({ kind: "planning" });
	if (workspacePill) {
		badges.push(workspacePill.badge);
	} else if (attachedWorkspace) {
		badges.push({ kind: "attached-workspace", ...attachedWorkspace });
	}
	if (selectedWorkspace && selectedWorkspace.id !== linkedWorkspaceId) {
		badges.push({ kind: "workspace", name: selectedWorkspace.name });
	}
	if (activeMcpServers.length >= 3) {
		badges.push({ kind: "mcp-group", servers: activeMcpServers });
	} else {
		for (const server of activeMcpServers) badges.push({ kind: "mcp", server });
	}

	return (
		// flex-1 routes free row space to the growing pills.
		<div className="flex min-w-0 flex-1 items-center gap-1">
			<AgentComposerOptionsMenu
				isDisabled={isDisabled}
				showAgentSetupNotice={props.showAgentSetupNotice ?? false}
				onAttachClick={props.onAttachClick}
				planModeEnabled={planModeEnabled}
				onPlanModeToggle={onPlanModeToggle}
				manageAutomationsEnabled={props.manageAutomationsEnabled ?? false}
				onManageAutomationsToggle={props.onManageAutomationsToggle}
				workspaceOptions={workspaceOptions}
				selectedWorkspaceId={selectedWorkspaceId}
				onWorkspaceChange={onWorkspaceChange}
				chatOrganizationId={props.chatOrganizationId}
				isWorkspaceLoading={props.isWorkspaceLoading}
				mcpServers={enabledMcpServers}
				selectedMCPServerIds={selectedMCPServerIds}
				onMCPToggle={toggleMcp}
				onMCPAuthComplete={props.onMCPAuthComplete}
			/>
			{props.isModelCatalogLoading ? (
				<Skeleton className="h-6 w-24 rounded" />
			) : (
				<ModelSelector
					value={props.selectedModel}
					onValueChange={props.onModelChange}
					options={props.modelOptions}
					disabled={isDisabled}
					placeholder={props.modelSelectorPlaceholder}
					className={cn(composerPillSizingClasses, "md:h-auto")}
					dropdownSide="top"
					dropdownAlign="start"
					enableMobileFullWidthDropdown
					reasoningEffort={props.reasoningEffort}
					onReasoningEffortChange={props.onReasoningEffortChange}
				/>
			)}
			{planModeEnabled && !overflowPlanning && (
				<AgentComposerPlanningBadge
					onRemove={disablePlanMode}
					isDisabled={isDisabled}
				/>
			)}
			<AgentComposerBadges
				badges={badges}
				workspacePill={workspacePill}
				onRemoveWorkspace={removeWorkspace}
				onRemoveMcp={(id) => toggleMcp(id, false)}
				onRemovePlanning={disablePlanMode}
				isDisabled={isDisabled}
			/>
		</div>
	);
};
