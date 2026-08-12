import { type FC, useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { toast } from "sonner";
import { isApiError } from "#/api/errors";
import { permittedOrganizations } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import type { AgentChatSendShortcut } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFileAttachments } from "../hooks/useFileAttachments";
import { parseStoredDraft } from "../utils/draftStorage";
import {
	getModelSelectorPlaceholder,
	getProviderForModelOption,
	hasConfiguredModelsInCatalog,
	hasUserFixableProviders,
} from "../utils/modelOptions";
import {
	getReasoningEffortForModel,
	pickReasoningEffort,
	saveReasoningEffortForModel,
} from "../utils/reasoningEffort";
import { AgentChatInput } from "./AgentChatInput";
import { ChatAccessDeniedAlert } from "./ChatAccessDeniedAlert";
import {
	isChatHookDeniedResponse,
	isChatHookDispatchFailedResponse,
} from "./ChatConversation/chatError";
import { getErrorTitle } from "./ChatConversation/chatStatusHelpers";
import type { ModelSelectorOption } from "./ChatElements";
import { CompactOrgSelector } from "./ChatElements";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	saveMCPSelection,
} from "./MCPServerPicker";
import { getModelSelectorHelp } from "./ModelSelectorHelp";

/** @internal Exported for testing. */
export const emptyInputStorageKey = "agents.empty-input";
const selectedWorkspaceIdStorageKey = "agents.selected-workspace-id";
const lastModelConfigIDStorageKey = "agents.last-model-config-id";

type ChatModelOption = ModelSelectorOption;

export type CreateChatOptions = {
	message: string;
	fileIDs?: string[];
	workspaceId?: string;
	model?: string;
	reasoningEffort?: string;
	mcpServerIds?: string[];
	organizationId: string;
	planMode?: TypesGen.ChatPlanMode;
};

/**
 * Hook that manages draft persistence for the empty-state chat input.
 * Persists the current input to localStorage so the user's draft
 * survives page reloads.
 *
 * Once `submitDraft` is called, the stored draft is removed and further
 * content changes are no longer persisted for the lifetime of the hook.
 * Call `resetDraft` to re-enable persistence (e.g. on mutation failure).
 *
 * @internal Exported for testing.
 */
export function useEmptyStateDraft() {
	const [{ initialInputValue, initialEditorState }] = useState(() => {
		const draft = parseStoredDraft(localStorage.getItem(emptyInputStorageKey));
		return {
			initialInputValue: draft.text,
			initialEditorState: draft.editorState,
		};
	});
	const inputValueRef = useRef(initialInputValue);
	const sentRef = useRef(false);

	const handleContentChange = (
		content: string,
		serializedEditorState: string,
		hasFileReferences: boolean,
	) => {
		inputValueRef.current = content;
		if (!sentRef.current) {
			const shouldPersist = content.trim() || hasFileReferences;
			if (shouldPersist) {
				try {
					localStorage.setItem(emptyInputStorageKey, serializedEditorState);
				} catch {
					// QuotaExceededError, silently discard the draft.
				}
			} else {
				localStorage.removeItem(emptyInputStorageKey);
			}
		}
	};

	const submitDraft = () => {
		// Mark as sent so that editor change events firing during
		// the async gap cannot re-persist the draft.
		sentRef.current = true;
		localStorage.removeItem(emptyInputStorageKey);
	};

	const resetDraft = () => {
		sentRef.current = false;
	};

	const getCurrentContent = () => inputValueRef.current;

	return {
		initialInputValue,
		initialEditorState,
		getCurrentContent,
		handleContentChange,
		submitDraft,
		resetDraft,
	};
}

interface AgentCreateFormProps {
	onCreateChat: (options: CreateChatOptions) => Promise<void>;
	sendShortcut: AgentChatSendShortcut;
	isCreating: boolean;
	createError: unknown;
	canCreateChat: boolean;
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	modelOptions: readonly ChatModelOption[];
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	aiGatewayDisabled?: boolean;
	isModelCatalogLoading: boolean;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	isModelConfigsLoading: boolean;
	rootPersonalModelOverride?: TypesGen.ChatPersonalModelOverride;
	isPersonalModelOverridesLoading?: boolean;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	onMCPAuthComplete?: (serverId: string) => void;
	workspaceCount: number | undefined;
	workspaceOptions: readonly TypesGen.Workspace[];
	workspacesError: unknown;
	isWorkspacesLoading: boolean;
}

export const AgentCreateForm: FC<AgentCreateFormProps> = ({
	onCreateChat,
	sendShortcut,
	isCreating,
	createError,
	canCreateChat,
	modelCatalog,
	modelOptions,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames,
	aiGatewayDisabled,
	modelConfigs,
	isModelCatalogLoading,
	isModelConfigsLoading,
	rootPersonalModelOverride,
	isPersonalModelOverridesLoading = false,
	mcpServers,
	onMCPAuthComplete,
	workspaceCount: _workspaceCount,
	workspaceOptions,
	workspacesError,
	isWorkspacesLoading,
}) => {
	const { organizations, showOrganizations } = useDashboard();
	const {
		initialInputValue,
		initialEditorState,
		handleContentChange,
		submitDraft,
		resetDraft,
	} = useEmptyStateDraft();
	const [initialLastModelConfigID] = useState(() => {
		return localStorage.getItem(lastModelConfigIDStorageKey) ?? "";
	});
	/*
	 * Model precedence: user click > root override (specific model) > root
	 * override (chat_default, resolved) > last-used > default > first available.
	 */
	const lastUsedModelID =
		initialLastModelConfigID &&
		modelOptions.some((option) => option.id === initialLastModelConfigID)
			? initialLastModelConfigID
			: "";
	const defaultModelID = (() => {
		const defaultModelConfig = Array.isArray(modelConfigs)
			? modelConfigs.find((config) => config.is_default)
			: undefined;
		if (!defaultModelConfig) {
			return "";
		}
		return modelOptions.some((option) => option.id === defaultModelConfig.id)
			? defaultModelConfig.id
			: "";
	})();
	const isUsableRootPersonalOverride =
		rootPersonalModelOverride?.is_set === true &&
		!rootPersonalModelOverride.is_malformed;
	const rootOverrideModelID =
		isUsableRootPersonalOverride &&
		rootPersonalModelOverride.mode === "model" &&
		modelOptions.some(
			(option) => option.id === rootPersonalModelOverride.model_config_id,
		)
			? rootPersonalModelOverride.model_config_id
			: "";
	const isRootOverrideChatDefault =
		isUsableRootPersonalOverride &&
		rootPersonalModelOverride.mode === "chat_default";
	const rootOverrideDisplayModelID = isRootOverrideChatDefault
		? defaultModelID || (modelOptions[0]?.id ?? "")
		: rootOverrideModelID;
	const fallbackModelID =
		lastUsedModelID || defaultModelID || (modelOptions[0]?.id ?? "");
	const preferredModelID = rootOverrideDisplayModelID || fallbackModelID;
	const [userSelectedModel, setUserSelectedModel] = useState("");
	const [hasUserSelectedModel, setHasUserSelectedModel] = useState(false);
	const hasValidUserSelectedModel =
		hasUserSelectedModel &&
		modelOptions.some((modelOption) => modelOption.id === userSelectedModel);
	// Derive the effective model every render so we never reference
	// a stale model id and can honor fallback precedence.
	const selectedModel = hasValidUserSelectedModel
		? userSelectedModel
		: preferredModelID;
	const submittedModel = (() => {
		if (hasValidUserSelectedModel) {
			return userSelectedModel;
		}
		if (rootOverrideModelID) {
			return rootOverrideModelID;
		}
		return selectedModel || undefined;
	})();
	const [selectedReasoningEfforts, setSelectedReasoningEfforts] = useState<
		Record<string, string>
	>({});
	const selectedModelOption = modelOptions.find(
		(option) => option.id === selectedModel,
	);
	// Persisted per-model choice wins over a root override; a stale
	// stored value is ignored so the override still applies. The
	// override applies to its own model even after a manual re-select.
	const rootOverrideReasoningEffort =
		selectedModel === rootOverrideModelID
			? rootPersonalModelOverride?.reasoning_effort
			: undefined;
	const persistedReasoningEffort = (() => {
		const stored = getReasoningEffortForModel(selectedModel);
		const efforts = selectedModelOption?.reasoningEfforts;
		return stored && efforts?.includes(stored) ? stored : undefined;
	})();
	const effectiveReasoningEffort = selectedModelOption
		? pickReasoningEffort(
				selectedReasoningEfforts[selectedModel] ??
					persistedReasoningEffort ??
					rootOverrideReasoningEffort,
				selectedModelOption.reasoningEfforts ?? [],
				selectedModelOption.reasoningEffortDefault,
			)
		: undefined;
	const initialOrg =
		organizations.find((o) => o.is_default) ?? organizations[0];
	// effectiveWorkspaceId nulls a stored selection outside the effective org's
	// filtered workspace list without deleting it. Preserve the stored value
	// because the permitted-organizations query may resolve after mount and
	// change the effective org.
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => localStorage.getItem(selectedWorkspaceIdStorageKey),
	);
	const [selectedOrg, setSelectedOrg] = useState<TypesGen.Organization | null>(
		null,
	);
	const [pendingOrgChange, setPendingOrgChange] =
		useState<TypesGen.Organization | null>(null);
	const permittedOrgsQuery = useQuery({
		...permittedOrganizations({
			// agents-access grants chat:create only at member scope. "me" is
			// replaced with the caller ID so that permission can match.
			object: { resource_type: "chat", owner_id: "me" },
			action: "create",
		}),
		enabled: showOrganizations,
	});
	const permittedOrgs = permittedOrgsQuery.data ?? organizations;
	const effectiveOrg =
		selectedOrg && permittedOrgs.some((org) => org.id === selectedOrg.id)
			? selectedOrg
			: (permittedOrgs.find((org) => org.is_default) ??
				permittedOrgs[0] ??
				initialOrg ??
				null);
	const organizationId = effectiveOrg?.id ?? "";
	const [planModeEnabled, setPlanModeEnabled] = useState(false);
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(modelCatalog);
	const hasUserFixableModelProviders = hasUserFixableProviders(modelCatalog);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelCatalogLoading,
		hasConfiguredModels,
		modelCatalog,
	);
	const modelSelectorHelp = getModelSelectorHelp({
		isModelCatalogLoading,
		hasModelOptions,
		hasConfiguredModels,
		hasUserFixableModelProviders,
	});
	useEffect(() => {
		if (!initialLastModelConfigID) {
			return;
		}
		if (isModelCatalogLoading || isModelConfigsLoading) {
			return;
		}
		if (lastUsedModelID) {
			return;
		}
		localStorage.removeItem(lastModelConfigIDStorageKey);
	}, [
		initialLastModelConfigID,
		isModelCatalogLoading,
		isModelConfigsLoading,
		lastUsedModelID,
	]);

	const [userMCPServerIds, setUserMCPServerIds] = useState<string[] | null>(
		null,
	);
	const effectiveMCPServerIds = (() => {
		if (userMCPServerIds !== null) {
			return userMCPServerIds;
		}
		const saved = getSavedMCPSelection(mcpServers ?? []);
		if (saved !== null) {
			return saved;
		}
		return getDefaultMCPSelection(mcpServers ?? []);
	})();
	const handleWorkspaceChange = (value: string | null) => {
		if (value === null) {
			setSelectedWorkspaceId(null);
			localStorage.removeItem(selectedWorkspaceIdStorageKey);
			return;
		}
		setSelectedWorkspaceId(value);
		localStorage.setItem(selectedWorkspaceIdStorageKey, value);
	};

	const handleModelChange = (value: string) => {
		setHasUserSelectedModel(true);
		setUserSelectedModel(value);
	};

	const handleReasoningEffortChange = (value: string) => {
		setSelectedReasoningEfforts((current) => ({
			...current,
			[selectedModel]: value,
		}));
		saveReasoningEffortForModel(selectedModel, value);
	};

	const isForbidden = !canCreateChat;

	// Filter workspaces by the selected organization. We use
	// client-side filtering of the full "owner:me" fetch rather
	// than re-querying with an org filter because it avoids
	// extra loading/error states on org change. The full list is
	// already small (user's own workspaces) and limit: 0
	// guarantees completeness. If workspace counts grow large
	// enough to warrant pagination, this should switch to a
	// server-side organization:<name> query filter.
	const filteredWorkspaces =
		showOrganizations && effectiveOrg
			? workspaceOptions.filter((ws) => ws.organization_id === effectiveOrg.id)
			: workspaceOptions;

	const effectiveWorkspaceId =
		selectedWorkspaceId !== null &&
		(isWorkspacesLoading ||
			filteredWorkspaces.some((ws) => ws.id === selectedWorkspaceId))
			? selectedWorkspaceId
			: null;
	// While the list loads, effectiveWorkspaceId is display-only: a
	// stored workspace may belong to an org the user cannot chat in,
	// so only a workspace confirmed in the effective org is submitted.
	const submittableWorkspaceId = isWorkspacesLoading
		? null
		: effectiveWorkspaceId;

	const handleSend = async (message: string, fileIDs?: string[]) => {
		submitDraft();
		await onCreateChat({
			message,
			fileIDs,
			workspaceId: submittableWorkspaceId ?? undefined,
			model: submittedModel,
			reasoningEffort: effectiveReasoningEffort,
			organizationId,
			mcpServerIds:
				effectiveMCPServerIds.length > 0
					? [...effectiveMCPServerIds]
					: undefined,
			planMode: planModeEnabled ? "plan" : undefined,
		}).catch((err) => {
			resetDraft();
			throw err;
		});
	};

	const {
		attachments,
		textContents,
		uploadStates,
		previewUrls,
		handleAttach,
		handleRemoveAttachment,
		resetAttachments,
	} = useFileAttachments(organizationId || undefined, {
		persist: true,
		provider: getProviderForModelOption(modelOptions, selectedModel),
	});

	const handleSendWithAttachments = async (message: string) => {
		const fileIds: string[] = [];
		let skippedErrors = 0;
		for (const file of attachments) {
			const state = uploadStates.get(file);
			if (state?.status === "error") {
				skippedErrors++;
				continue;
			}
			if (state?.status === "uploaded" && state.fileId) {
				fileIds.push(state.fileId);
			}
		}
		if (skippedErrors > 0) {
			toast.warning(
				`${skippedErrors} attachment${skippedErrors > 1 ? "s" : ""} could not be sent (upload failed)`,
			);
		}
		const fileArg = fileIds.length > 0 ? fileIds : undefined;
		try {
			await handleSend(message, fileArg);
			resetAttachments();
		} catch {
			// Attachments preserved for retry on failure.
		}
	};

	return (
		<>
			<div className="order-last flex min-h-0 flex-none items-end justify-center overflow-auto px-4 pb-4 sm:order-none sm:h-full sm:flex-1 sm:items-center">
				<div className="mx-auto flex w-full max-w-3xl flex-col gap-2">
					{isForbidden ? (
						<ChatAccessDeniedAlert />
					) : createError ? (
						isApiError(createError) &&
						createError.response.status === 502 &&
						isChatHookDispatchFailedResponse(createError.response.data) ? (
							<Alert severity="error">
								<AlertTitle>
									{getErrorTitle("hook_dispatch_failed", "error")}
								</AlertTitle>
								<AlertDescription>
									<span>{createError.response.data.message}</span>
									{createError.response.data.detail && (
										<span className="mt-1 block text-content-secondary">
											{createError.response.data.detail}
										</span>
									)}
								</AlertDescription>
							</Alert>
						) : isApiError(createError) &&
							createError.response.status === 403 &&
							isChatHookDeniedResponse(createError.response.data) ? (
							<Alert severity="info">
								<AlertDescription>
									{createError.response.data.message}
								</AlertDescription>
							</Alert>
						) : (
							<ErrorAlert error={createError} />
						)
					) : null}
					{workspacesError != null && <ErrorAlert error={workspacesError} />}
					{permittedOrgsQuery.error != null && (
						<ErrorAlert error={permittedOrgsQuery.error} />
					)}
					{showOrganizations && permittedOrgs.length > 1 && (
						<CompactOrgSelector
							value={effectiveOrg}
							options={permittedOrgs}
							onChange={(newOrg) => {
								const orgChanged = newOrg.id !== effectiveOrg?.id;
								if (orgChanged && attachments.length > 0) {
									setPendingOrgChange(newOrg);
									return;
								}
								if (orgChanged) {
									handleWorkspaceChange(null);
								}
								setSelectedOrg(newOrg);
							}}
						/>
					)}
					<AgentChatInput
						onSend={handleSendWithAttachments}
						sendShortcut={sendShortcut}
						placeholder="Ask Coder to build, fix bugs, or explore your project..."
						isDisabled={
							isCreating ||
							isForbidden ||
							(showOrganizations && permittedOrgsQuery.isLoading) ||
							isPersonalModelOverridesLoading ||
							!hasModelOptions ||
							Boolean(aiGatewayDisabled)
						}
						isLoading={isCreating}
						initialValue={initialInputValue}
						initialEditorState={initialEditorState}
						onContentChange={handleContentChange}
						selectedModel={selectedModel}
						onModelChange={handleModelChange}
						modelOptions={modelOptions}
						modelSelectorPlaceholder={modelSelectorPlaceholder}
						reasoningEffort={effectiveReasoningEffort}
						onReasoningEffortChange={handleReasoningEffortChange}
						isModelCatalogLoading={isModelCatalogLoading}
						hasModelOptions={hasModelOptions}
						planModeEnabled={planModeEnabled}
						onPlanModeToggle={setPlanModeEnabled}
						attachments={attachments}
						onAttach={handleAttach}
						onRemoveAttachment={handleRemoveAttachment}
						uploadStates={uploadStates}
						previewUrls={previewUrls}
						textContents={textContents}
						mcpServers={mcpServers}
						selectedMCPServerIds={effectiveMCPServerIds}
						onMCPSelectionChange={(ids) => {
							setUserMCPServerIds(ids);
							saveMCPSelection(ids);
						}}
						onMCPAuthComplete={onMCPAuthComplete}
						workspaceOptions={filteredWorkspaces}
						selectedWorkspaceId={effectiveWorkspaceId}
						onWorkspaceChange={handleWorkspaceChange}
						isWorkspaceLoading={isWorkspacesLoading}
						canConfigureAgentSetup={canConfigureAgentSetup}
						providerCount={providerCount}
						modelCount={modelCount}
						unsupportedProviderNames={unsupportedProviderNames}
						aiGatewayDisabled={aiGatewayDisabled}
					/>
					{modelSelectorHelp ? (
						<div className="px-3 pt-1 text-2xs text-content-secondary">
							{modelSelectorHelp}
						</div>
					) : null}
				</div>
			</div>
			<ConfirmDialog
				open={pendingOrgChange !== null}
				title="Change organization?"
				description="Changing organization will remove your current attachments."
				type="info"
				hideCancel={false}
				confirmText="Continue"
				onConfirm={() => {
					if (!pendingOrgChange) {
						return;
					}
					resetAttachments();
					handleWorkspaceChange(null);
					setSelectedOrg(pendingOrgChange);
					setPendingOrgChange(null);
				}}
				onClose={() => setPendingOrgChange(null)}
			/>
		</>
	);
};
