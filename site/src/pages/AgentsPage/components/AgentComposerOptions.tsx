import { cn } from "cn";
import type React from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "#/modules/aiModels/ModelSelector";
import { useAgentComposer } from "./AgentComposer";
import {
	AgentComposerBadges,
	AgentComposerPlanningBadge,
	composerPillSizingClasses,
	type ToolBadgeData,
	type WorkspacePillBadge,
} from "./AgentComposerBadges";
import {
	type AgentComposerOptionsData,
	AgentComposerOptionsMenu,
	OptionsContext,
	useAgentComposerOptions,
} from "./AgentComposerOptionsMenu";

export type { AttachedWorkspaceInfo } from "./AgentComposerBadges";

/** Controlled tool state shared by the menu and independently composed badges. */
type AgentComposerOptionsProviderProps = AgentComposerOptionsData & {
	children: React.ReactNode;
};

const AgentComposerOptionsProvider = ({
	children,
	...data
}: AgentComposerOptionsProviderProps) => {
	const {
		workspaceOptions,
		selectedWorkspaceId,
		onWorkspaceChange,
		selectedMCPServerIds,
		onMCPSelectionChange,
		workspace,
		workspaceAgent,
		chatId,
		attachedWorkspace,
	} = data;

	const toggleMcp = (serverId: string, checked: boolean) => {
		if (!onMCPSelectionChange || !selectedMCPServerIds) {
			return;
		}

		onMCPSelectionChange(
			checked
				? [...selectedMCPServerIds, serverId]
				: selectedMCPServerIds.filter((id) => id !== serverId),
		);
	};

	const enabledMcpServers =
		data.mcpServers?.filter((server) => server.enabled) ?? [];
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
	const workspacePill: WorkspacePillBadge | undefined =
		workspace && workspaceAgent && chatId
			? {
					badge: attachedWorkspace
						? { kind: "attached-workspace", ...attachedWorkspace }
						: { kind: "workspace", name: workspace.name },
					props: {
						workspace,
						agent: workspaceAgent,
						chatId,
						sshCommand: data.sshCommand,
						folder: data.folder,
					},
				}
			: undefined;

	// Ordering controls which trailing badges move into the overflow menu.
	const badges: ToolBadgeData[] = [];
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
		for (const server of activeMcpServers) {
			badges.push({ kind: "mcp", server });
		}
	}

	return (
		<OptionsContext
			value={{
				state: { ...data, mcpServers: enabledMcpServers },
				actions: {
					toggleMcp,
					removeWorkspace: onWorkspaceChange
						? () => onWorkspaceChange(null)
						: undefined,
					disablePlanMode: () => data.onPlanModeToggle(false),
				},
				meta: { badges, workspacePill },
			}}
		>
			{children}
		</OptionsContext>
	);
};

const AgentComposerOptionsFrame = ({
	children,
}: {
	children: React.ReactNode;
}) => <div className="flex min-w-0 flex-1 items-center gap-1">{children}</div>;

/** Model catalog selection and reasoning controls, independent of tool state. */
type AgentComposerModelProps = {
	selectedModel: string;
	onModelChange: (value: string) => void;
	modelOptions: readonly ModelSelectorOption[];
	modelSelectorPlaceholder: string;
	reasoningEffort?: string;
	onReasoningEffortChange?: (value: string) => void;
	isModelCatalogLoading: boolean;
};

const AgentComposerModel = (props: AgentComposerModelProps) => {
	const { state } = useAgentComposer();

	if (props.isModelCatalogLoading) {
		return <Skeleton className="h-6 w-24 rounded" />;
	}

	return (
		<ModelSelector
			value={props.selectedModel}
			onValueChange={props.onModelChange}
			options={props.modelOptions}
			disabled={state.isDisabled}
			placeholder={props.modelSelectorPlaceholder}
			className={cn(composerPillSizingClasses, "md:h-auto")}
			dropdownSide="top"
			dropdownAlign="start"
			enableMobileFullWidthDropdown
			reasoningEffort={props.reasoningEffort}
			onReasoningEffortChange={props.onReasoningEffortChange}
		/>
	);
};

const AgentComposerOptionsPlanningBadge = () => {
	const { state, actions } = useAgentComposerOptions();
	const composer = useAgentComposer();

	if (!state.planModeEnabled) {
		return null;
	}

	return (
		<AgentComposerPlanningBadge
			onRemove={actions.disablePlanMode}
			isDisabled={composer.state.isDisabled}
		/>
	);
};

const AgentComposerOptionsBadges = ({
	leadingBadges = [],
}: {
	leadingBadges?: readonly Extract<ToolBadgeData, { kind: "planning" }>[];
}) => {
	const { actions, meta } = useAgentComposerOptions();
	const { state } = useAgentComposer();

	return (
		<AgentComposerBadges
			badges={[...leadingBadges, ...meta.badges]}
			workspacePill={meta.workspacePill}
			onRemoveWorkspace={actions.removeWorkspace}
			onRemoveMcp={(id) => actions.toggleMcp(id, false)}
			onRemovePlanning={actions.disablePlanMode}
			isDisabled={state.isDisabled}
		/>
	);
};

/** Compose model selection and shared tool controls without a fixed toolbar assembly. */
export const AgentComposerOptions = {
	Provider: AgentComposerOptionsProvider,
	Frame: AgentComposerOptionsFrame,
	Model: AgentComposerModel,
	Menu: AgentComposerOptionsMenu,
	PlanningBadge: AgentComposerOptionsPlanningBadge,
	Badges: AgentComposerOptionsBadges,
};
