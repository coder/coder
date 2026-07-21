import { type FC, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { isApiError } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import type { AgentChatSendShortcut } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { docs } from "#/utils/docs";
import { useFileAttachments } from "../hooks/useFileAttachments";
import { parseStoredDraft } from "../utils/draftStorage";
import {
	getModelSelectorPlaceholder,
	getProviderForModelOption,
	hasConfiguredModelsInCatalog,
	hasUserFixableProviders,
} from "../utils/modelOptions";
import { pickReasoningEffort } from "../utils/reasoningEffort";
import {
	formatUsageLimitMessage,
	isChatUsageLimitExceededResponse,
} from "../utils/usageLimitMessage";
import { AgentChatInput } from "./AgentChatInput";
import { ChatAccessDeniedAlert } from "./ChatAccessDeniedAlert";
import type { ModelSelectorOption } from "./ChatElements";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	saveMCPSelection,
} from "./MCPServerPicker";
import { getModelSelectorHelp } from "./ModelSelectorHelp";

/** @internal Exported for testing. */
export const emptyInputStorageKey = "agents.empty-input";
const selectedWorkspaceIdStorageKey = "agents.selected-workspace-id";
const lastModelConfigIDStorageKey = (organization: string) =>
	`agents.last-model-config-id.${organization}`;

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
	organization: TypesGen.Organization;
	registerOrganizationChangeGuard?: (
		guard: (
			nextOrganization: TypesGen.Organization,
		) => boolean | Promise<boolean>,
	) => () => void;
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
	organization,
	registerOrganizationChangeGuard,
}) => {
	const {
		initialInputValue,
		initialEditorState,
		handleContentChange,
		submitDraft,
		resetDraft,
	} = useEmptyStateDraft();
	const [initialLastModelConfigID] = useState(() => {
		return (
			localStorage.getItem(lastModelConfigIDStorageKey(organization.name)) ?? ""
		);
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
	const [selectedReasoningEffort, setSelectedReasoningEffort] = useState("");
	const selectedModelOption = modelOptions.find(
		(option) => option.id === selectedModel,
	);
	const effectiveReasoningEffort = selectedModelOption
		? pickReasoningEffort(
				selectedReasoningEffort,
				selectedModelOption.reasoningEfforts ?? [],
				selectedModelOption.reasoningEffortDefault,
			)
		: undefined;
	const organizationId = organization.id;
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => localStorage.getItem(selectedWorkspaceIdStorageKey),
	);
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
		localStorage.removeItem(lastModelConfigIDStorageKey(organization.name));
	}, [
		initialLastModelConfigID,
		isModelCatalogLoading,
		isModelConfigsLoading,
		lastUsedModelID,
		organization.name,
	]);

	const [userMCPServerIds, setUserMCPServerIds] = useState<string[] | null>(
		null,
	);
	const effectiveMCPServerIds = (() => {
		if (userMCPServerIds !== null) {
			return userMCPServerIds;
		}
		const saved = getSavedMCPSelection(mcpServers ?? [], organization.name);
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

	const isForbidden = !canCreateChat;

	// Filter workspaces by the selected organization. We use
	// client-side filtering of the full "owner:me" fetch rather
	// than re-querying with an org filter because it avoids
	// extra loading/error states on org change. The full list is
	// already small (user's own workspaces) and limit: 0
	// guarantees completeness. If workspace counts grow large
	// enough to warrant pagination, this should switch to a
	// server-side organization:<name> query filter.
	const filteredWorkspaces = workspaceOptions.filter(
		(workspace) => workspace.organization_id === organization.id,
	);

	const effectiveWorkspaceId =
		selectedWorkspaceId !== null &&
		(isWorkspacesLoading ||
			filteredWorkspaces.some((ws) => ws.id === selectedWorkspaceId))
			? selectedWorkspaceId
			: null;

	const handleSend = async (message: string, fileIDs?: string[]) => {
		submitDraft();
		await onCreateChat({
			message,
			fileIDs,
			workspaceId: effectiveWorkspaceId ?? undefined,
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
	const attachmentsRef = useRef(attachments);
	useEffect(() => {
		attachmentsRef.current = attachments;
	}, [attachments]);
	const pendingOrganizationChangeRef = useRef<
		| {
				promise: Promise<boolean>;
				resolve: (confirmed: boolean) => void;
		  }
		| undefined
	>(undefined);
	const [organizationChangePending, setOrganizationChangePending] =
		useState(false);

	useEffect(() => {
		if (!registerOrganizationChangeGuard) {
			return;
		}
		const unregister = registerOrganizationChangeGuard(() => {
			if (attachmentsRef.current.length === 0) {
				return true;
			}
			if (pendingOrganizationChangeRef.current) {
				return pendingOrganizationChangeRef.current.promise;
			}
			setOrganizationChangePending(true);
			let resolve: (confirmed: boolean) => void = () => undefined;
			const promise = new Promise<boolean>((promiseResolve) => {
				resolve = promiseResolve;
			});
			pendingOrganizationChangeRef.current = { promise, resolve };
			return promise;
		});
		return () => {
			unregister();
			pendingOrganizationChangeRef.current?.resolve(false);
			pendingOrganizationChangeRef.current = undefined;
		};
	}, [registerOrganizationChangeGuard]);

	const resolveOrganizationChange = (confirmed: boolean) => {
		if (confirmed) {
			resetAttachments();
			handleWorkspaceChange(null);
		}
		pendingOrganizationChangeRef.current?.resolve(confirmed);
		pendingOrganizationChangeRef.current = undefined;
		setOrganizationChangePending(false);
	};

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
						createError.response?.status === 409 &&
						isChatUsageLimitExceededResponse(createError.response.data) ? (
							<Alert
								severity="info"
								actions={
									<Button asChild size="sm">
										<Link to="/agents/analytics">View usage</Link>
									</Button>
								}
							>
								<AlertDescription>
									{formatUsageLimitMessage(createError.response.data)}
								</AlertDescription>
							</Alert>
						) : (
							<ErrorAlert error={createError} />
						)
					) : null}
					{workspacesError != null && <ErrorAlert error={workspacesError} />}
					<AgentChatInput
						onSend={handleSendWithAttachments}
						sendShortcut={sendShortcut}
						placeholder="Ask Coder to build, fix bugs, or explore your project..."
						isDisabled={
							isCreating ||
							isForbidden ||
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
						onReasoningEffortChange={setSelectedReasoningEffort}
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
							saveMCPSelection(ids, organization.name);
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
					<p className="text-center text-xs text-content-secondary/50">
						<a
							href={docs("/ai-coder/agents")}
							target="_blank"
							rel="noreferrer"
							className="text-content-secondary/50 underline hover:text-content-secondary"
						>
							Introductory access
						</a>{" "}
						to Coder Agents through September 2026
					</p>
				</div>
			</div>
			<ConfirmDialog
				open={organizationChangePending}
				title="Change organization?"
				description="Changing organization will remove your current attachments."
				type="info"
				hideCancel={false}
				confirmText="Continue"
				onConfirm={() => resolveOrganizationChange(true)}
				onClose={() => resolveOrganizationChange(false)}
			/>
		</>
	);
};
