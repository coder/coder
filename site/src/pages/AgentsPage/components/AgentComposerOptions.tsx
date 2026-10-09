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
} from "./AgentComposerBadges";
import {
	type AgentComposerOptionsData,
	AgentComposerOptionsMenu,
	OptionsContext,
	useAgentComposerOptions,
} from "./AgentComposerOptionsMenu";

export type { AttachedWorkspaceInfo } from "./AgentComposerBadges";

/** Tool settings shared by the menu and badges. */
type AgentComposerOptionsProviderProps = AgentComposerOptionsData & {
	children: React.ReactNode;
};

const AgentComposerOptionsProvider = ({
	children,
	...data
}: AgentComposerOptionsProviderProps) => {
	const { planning, mcp, workspaceSelection, linkedWorkspace } = data;
	const selectedServerIds = mcp?.selectedServerIds;
	const { options, selectedId, onChange } = workspaceSelection ?? {};
	const { workspace, agent, chatId, attachedWorkspace } = linkedWorkspace ?? {};

	const toggleMcp = (serverId: string, checked: boolean) => {
		if (!mcp) {
			return;
		}

		mcp.onSelectionChange(
			checked
				? [...mcp.selectedServerIds, serverId]
				: mcp.selectedServerIds.filter((id) => id !== serverId),
		);
	};

	const enabledMcpServers =
		mcp?.servers.filter((server) => server.enabled) ?? [];
	const activeMcpServers = enabledMcpServers.filter(
		(server) =>
			(server.availability === "force_on" ||
				selectedServerIds?.includes(server.id)) &&
			!(server.auth_type === "oauth2" && !server.auth_connected),
	);
	const selectedWorkspace = options?.find((item) => item.id === selectedId);
	const linkedWorkspaceId = workspace?.id ?? attachedWorkspace?.id;

	// Ordering controls which trailing badges move into the overflow menu.
	const badges: ToolBadgeData[] = [];
	if (workspace && agent && chatId) {
		badges.push({
			kind: "linked-workspace",
			props: {
				workspace,
				agent,
				chatId,
				sshCommand: linkedWorkspace?.sshCommand,
				folder: linkedWorkspace?.folder,
			},
		});
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
				state: {
					organizationId: data.organizationId,
					planning,
					automations: data.automations,
					mcp: mcp ? { ...mcp, servers: enabledMcpServers } : undefined,
					workspaceSelection,
				},
				actions: {
					toggleMcp,
					removeWorkspace: onChange ? () => onChange(null) : undefined,
					disablePlanMode: () => planning.onChange(false),
				},
				meta: { badges },
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

	if (!state.planning.enabled) {
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
			onRemoveWorkspace={actions.removeWorkspace}
			onRemoveMcp={(id) => actions.toggleMcp(id, false)}
			onRemovePlanning={actions.disablePlanMode}
			isDisabled={state.isDisabled}
		/>
	);
};

/** Composer controls that callers can arrange or omit independently. */
export const AgentComposerOptions = {
	Provider: AgentComposerOptionsProvider,
	Frame: AgentComposerOptionsFrame,
	Model: AgentComposerModel,
	Menu: AgentComposerOptionsMenu,
	PlanningBadge: AgentComposerOptionsPlanningBadge,
	Badges: AgentComposerOptionsBadges,
};
