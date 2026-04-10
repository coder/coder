import { type FC, useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { isApiError } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { docs } from "#/utils/docs";
import { useFileAttachments } from "../hooks/useFileAttachments";
import { parseStoredDraft } from "../utils/draftStorage";
import {
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
	hasUserFixableProviders,
} from "../utils/modelOptions";
import {
	formatUsageLimitMessage,
	isUsageLimitData,
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
const lastModelConfigIDStorageKey = "agents.last-model-config-id";

type ChatModelOption = ModelSelectorOption;

export type CreateChatOptions = {
	message: string;
	fileIDs?: string[];
	workspaceId?: string;
	model?: string;
	mcpServerIds?: string[];
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
					// QuotaExceededError — silently discard the draft.
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
	isCreating: boolean;
	createError: unknown;
	canCreateChat: boolean;
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	modelOptions: readonly ChatModelOption[];
	isModelCatalogLoading: boolean;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	isModelConfigsLoading: boolean;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	onMCPAuthComplete?: (serverId: string) => void;
	workspaceCount: number | undefined;
	workspaceOptions: readonly TypesGen.Workspace[];
	workspacesError: unknown;
	isWorkspacesLoading: boolean;
}

export const AgentCreateForm: FC<AgentCreateFormProps> = ({
	onCreateChat,
	isCreating,
	createError,
	canCreateChat,
	modelCatalog,
	modelOptions,
	modelConfigs,
	isModelCatalogLoading,
	isModelConfigsLoading,
	mcpServers,
	onMCPAuthComplete,
	workspaceCount: _workspaceCount,
	workspaceOptions,
	workspacesError,
	isWorkspacesLoading,
}) => {
	const { organizations } = useDashboard();
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
	const preferredModelID =
		lastUsedModelID || defaultModelID || (modelOptions[0]?.id ?? "");
	const [userSelectedModel, setUserSelectedModel] = useState("");
	const [hasUserSelectedModel, setHasUserSelectedModel] = useState(false);
	// Derive the effective model every render so we never reference
	// a stale model id and can honor fallback precedence.
	const selectedModel =
		hasUserSelectedModel &&
		modelOptions.some((modelOption) => modelOption.id === userSelectedModel)
			? userSelectedModel
			: preferredModelID;
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => {
			return localStorage.getItem(selectedWorkspaceIdStorageKey) || null;
		},
	);
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

	// Keep a mutable ref to selectedWorkspaceId and selectedModel so
	// that the onSend callback always sees the latest values without
	// the shared input component re-rendering on every change.
	const selectedWorkspaceIdRef = useRef(selectedWorkspaceId);
	const selectedModelRef = useRef(selectedModel);
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
	const selectedMCPServerIdsRef = useRef(effectiveMCPServerIds);
	useEffect(() => {
		selectedWorkspaceIdRef.current = selectedWorkspaceId;
		selectedModelRef.current = selectedModel;
		selectedMCPServerIdsRef.current = effectiveMCPServerIds;
	});
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

	const handleSend = async (message: string, fileIDs?: string[]) => {
		submitDraft();
		await onCreateChat({
			message,
			fileIDs,
			workspaceId: selectedWorkspaceIdRef.current ?? undefined,
			model: selectedModelRef.current || undefined,
			mcpServerIds:
				selectedMCPServerIdsRef.current.length > 0
					? [...selectedMCPServerIdsRef.current]
					: undefined,
		}).catch((err) => {
			// Re-enable draft persistence so the user can edit
			// and retry after a failed send attempt, then rethrow
			// so callers (handleSendWithAttachments) can preserve
			// attachments on failure.
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
	} = useFileAttachments(organizations[0]?.id, { persist: true });

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

	const isForbidden = !canCreateChat;

	return (
		<div className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-4 pt-12 md:h-full md:items-center md:pt-4">
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
				{isForbidden ? (
					<ChatAccessDeniedAlert />
				) : createError ? (
					isApiError(createError) &&
					createError.response?.status === 409 &&
					isUsageLimitData(createError.response.data) ? (
						<Alert
							severity="info"
							actions={
								<Button asChild size="sm">
									<Link to="/agents/analytics">View Usage</Link>
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
					placeholder="Ask Coder to build, fix bugs, or explore your project..."
					isDisabled={isCreating || isForbidden}
					isLoading={isCreating}
					initialValue={initialInputValue}
					initialEditorState={initialEditorState}
					onContentChange={handleContentChange}
					selectedModel={selectedModel}
					onModelChange={handleModelChange}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					isModelCatalogLoading={isModelCatalogLoading}
					hasModelOptions={hasModelOptions}
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
					workspaceOptions={workspaceOptions}
					selectedWorkspaceId={selectedWorkspaceId}
					onWorkspaceChange={handleWorkspaceChange}
					isWorkspaceLoading={isWorkspacesLoading}
				/>
				{modelSelectorHelp ? (
					<div className="px-3 pt-1 text-2xs text-content-secondary">
						{modelSelectorHelp}
					</div>
				) : null}
				<p className="mt-1 text-center text-xs text-content-secondary/50">
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
	);
};
