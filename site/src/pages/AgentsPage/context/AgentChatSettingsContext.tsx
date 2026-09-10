import {
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useState,
} from "react";
import { useQuery } from "react-query";
import { chatProviderConfigs } from "#/api/queries/aiProviders";
import { chatModels, mcpServerConfigs } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	resolveMCPSelection,
	saveMCPSelection,
} from "../components/MCPServerPicker";
import { getModelSelectorHelp } from "../components/ModelSelectorHelp";
import {
	countConfiguredProviderConfigs,
	getModelSelectorPlaceholder,
	getUnavailableModelNotice,
	getUnsupportedProviderNames,
	getUsableDefaultModelIDForOrganization,
	hasUserFixableProviders,
	resolveModelOptionId,
	resolveModelSelector,
} from "../utils/modelOptions";
import { pickReasoningEffort } from "../utils/reasoningEffort";

/**
 * Model and MCP settings that decide what a chat submits. Sends, edits, plan
 * implementation, and ask-user responses read the same resolved values as the
 * composer, so changing a selector applies to the very next submission.
 */
export interface AgentChatSettings {
	/** Selectable models for the chat organization. */
	readonly modelOptions: readonly ModelSelectorOption[];
	/** Catalog entries behind the options, used for compaction thresholds. */
	readonly models: readonly TypesGen.ChatModel[] | undefined;
	readonly hasModelOptions: boolean;
	/**
	 * True while the catalog is unresolved, including before the chat record
	 * names an organization. Keeps the selector from flashing "No Models".
	 */
	readonly isModelCatalogLoading: boolean;
	readonly modelCatalogError: unknown;
	readonly modelSelectorPlaceholder: string;
	readonly modelSelectorHelp: ReactNode | undefined;
	readonly unavailableModelNotice: string | undefined;
	readonly canConfigureAgentSetup: boolean;
	readonly providerCount: number | undefined;
	readonly modelCount: number | undefined;
	readonly unsupportedProviderNames: readonly string[];
	/** The model every submission uses, validated against the options. */
	readonly selectedModel: string;
	readonly onModelChange: (modelID: string) => void;
	readonly reasoningEffort: string | undefined;
	/** Records a deliberate effort pick, which an in-progress edit sends. */
	readonly onReasoningEffortChange: (reasoningEffort: string) => void;
	/**
	 * Whether the user picked an effort since the last reset. An edit sends
	 * `reasoning_effort` only when the user picked one for that edit, so the
	 * backend keeps the original effort otherwise.
	 */
	readonly hasPickedReasoningEffort: boolean;
	/** Clears {@link hasPickedReasoningEffort}. Call when an edit begins. */
	readonly resetPickedReasoningEffort: () => void;
	readonly mcpServers: readonly TypesGen.MCPServerConfig[];
	/** The MCP servers every submission uses. */
	readonly selectedMCPServerIds: readonly string[];
	readonly onMCPSelectionChange: (ids: string[]) => void;
	readonly onMCPAuthComplete: (serverId: string) => void;
}

const AgentChatSettingsContext = createContext<AgentChatSettings | undefined>(
	undefined,
);

/**
 * Returns the canonical submission settings for the current chat. Must be used
 * within an `AgentChatSettingsProvider`.
 */
export const useAgentChatSettings = (): AgentChatSettings => {
	const settings = useContext(AgentChatSettingsContext);
	if (settings === undefined) {
		throw new Error(
			"useAgentChatSettings must be used within an AgentChatSettingsProvider",
		);
	}
	return settings;
};

interface AgentChatSettingsProviderProps {
	/** The open chat, or undefined while it loads. */
	chat: TypesGen.Chat | undefined;
	children: ReactNode;
}

interface AgentChatSettingsValueProviderProps {
	settings: AgentChatSettings;
	children: ReactNode;
}

/**
 * Provides settings from an explicit value instead of the queries.
 *
 * @internal Intended for tests and stories that render one isolated settings
 * state, including states the query cache cannot represent, such as a cached
 * catalog whose refetch failed. It exists so the context itself stays private:
 * a narrow seam that only accepts a complete {@link AgentChatSettings} is safer
 * than exporting the raw context. Production code uses
 * {@link AgentChatSettingsProvider}.
 */
export const AgentChatSettingsValueProvider: FC<
	AgentChatSettingsValueProviderProps
> = ({ settings, children }) => (
	<AgentChatSettingsContext value={settings}>
		{children}
	</AgentChatSettingsContext>
);

export const AgentChatSettingsProvider: FC<AgentChatSettingsProviderProps> = ({
	chat,
	children,
}) => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const organizationId = chat?.organization_id ?? "";

	const [userSelectedModel, setUserSelectedModel] = useState("");
	const [userSelectedReasoningEffort, setUserSelectedReasoningEffort] =
		useState("");
	const [hasPickedReasoningEffort, setHasPickedReasoningEffort] =
		useState(false);
	const [userSelectedMCPServerIds, setUserSelectedMCPServerIds] = useState<
		string[] | null
	>(null);

	const modelsQuery = useQuery(chatModels(organizationId));
	const chatProviderConfigsQuery = useQuery({
		...chatProviderConfigs(),
		enabled: permissions.editDeploymentConfig,
	});
	const mcpServersQuery = useQuery({
		...mcpServerConfigs(organizationId),
		enabled: Boolean(organizationId),
	});

	const models = modelsQuery.data?.models;
	const {
		options: modelOptions,
		isModelCatalogLoading,
		modelCatalog,
		hasConfiguredModels,
	} = resolveModelSelector(organizationId, modelsQuery);
	const isModelDataPending = organizationId === "" || isModelCatalogLoading;
	const hasModelOptions = modelOptions.length > 0;
	const providerCount =
		permissions.editDeploymentConfig &&
		chatProviderConfigsQuery.data &&
		modelsQuery.data
			? countConfiguredProviderConfigs(
					chatProviderConfigsQuery.data,
					modelsQuery.data,
				)
			: undefined;

	// Validate explicit and historical choices against organization options.
	// Prefer the usable organization default before another organization model.
	const selectedModel = (() => {
		const resolvedUserSelection = resolveModelOptionId(
			userSelectedModel,
			modelOptions,
		);
		if (resolvedUserSelection) {
			return resolvedUserSelection;
		}

		const resolvedChatModel = resolveModelOptionId(
			chat?.last_model_config_id,
			modelOptions,
		);
		if (resolvedChatModel) {
			return resolvedChatModel;
		}

		return (
			getUsableDefaultModelIDForOrganization(
				models,
				modelOptions,
				organizationId,
			) ||
			modelOptions[0]?.id ||
			""
		);
	})();

	const selectedModelOption = modelOptions.find(
		(option) => option.id === selectedModel,
	);
	const reasoningEffort = selectedModelOption
		? pickReasoningEffort(
				userSelectedReasoningEffort || chat?.last_reasoning_effort,
				selectedModelOption.reasoningEfforts ?? [],
				selectedModelOption.reasoningEffortDefault,
			)
		: undefined;

	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelDataPending,
		hasConfiguredModels,
		modelCatalog,
	);
	const modelSelectorHelp = getModelSelectorHelp({
		isModelCatalogLoading: isModelDataPending,
		hasModelOptions,
		hasConfiguredModels,
		hasUserFixableModelProviders: hasUserFixableProviders(modelCatalog),
	});
	const unavailableModelNotice = getUnavailableModelNotice({
		hasResolvedModelData:
			!isModelDataPending && Boolean(modelsQuery.data) && !modelsQuery.error,
		storedModelRef: chat?.last_model_config_id,
		modelOptions,
		catalog: modelCatalog,
	});

	const mcpServers = mcpServersQuery.data ?? [];
	const isDefaultChatOrganization = organizations.some(
		(organization) =>
			organization.id === organizationId && organization.is_default,
	);
	const selectedMCPServerIds = resolveMCPSelection({
		userSelection: userSelectedMCPServerIds,
		chatSelection: chat?.mcp_server_ids,
		organizationId,
		servers: mcpServers,
		isDefaultOrganization: isDefaultChatOrganization,
	});

	const settings: AgentChatSettings = {
		modelOptions,
		models,
		hasModelOptions,
		isModelCatalogLoading: isModelDataPending,
		modelCatalogError: modelsQuery.error,
		modelSelectorPlaceholder,
		modelSelectorHelp,
		unavailableModelNotice,
		canConfigureAgentSetup: permissions.editDeploymentConfig,
		providerCount,
		modelCount: modelsQuery.data ? modelOptions.length : undefined,
		unsupportedProviderNames: getUnsupportedProviderNames(modelsQuery.data),
		selectedModel,
		onModelChange: setUserSelectedModel,
		reasoningEffort,
		onReasoningEffortChange: (nextReasoningEffort: string) => {
			setUserSelectedReasoningEffort(nextReasoningEffort);
			setHasPickedReasoningEffort(true);
		},
		hasPickedReasoningEffort,
		resetPickedReasoningEffort: () => setHasPickedReasoningEffort(false),
		mcpServers,
		selectedMCPServerIds,
		onMCPSelectionChange: (ids: string[]) => {
			setUserSelectedMCPServerIds(ids);
			if (organizationId) {
				saveMCPSelection(organizationId, ids);
			}
		},
		onMCPAuthComplete: () => {
			void mcpServersQuery.refetch();
		},
	};

	return (
		<AgentChatSettingsContext value={settings}>
			{children}
		</AgentChatSettingsContext>
	);
};
