import { isApiError } from "api/errors";
import { workspaces } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChevronDownIcon } from "components/AnimatedIcons/ChevronDown";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { Check, MonitorIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { useFileAttachments } from "../hooks/useFileAttachments";
import {
	getModelCatalogStatusMessage,
	getModelSelectorPlaceholder,
	getNormalizedModelRef,
	hasConfiguredModelsInCatalog,
} from "../utils/modelOptions";
import {
	formatUsageLimitMessage,
	isUsageLimitData,
} from "../utils/usageLimitMessage";
import { AgentChatInput } from "./AgentChatInput";

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
	const [initialInputValue] = useState(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	});
	const inputValueRef = useRef(initialInputValue);
	const sentRef = useRef(false);

	const handleContentChange = (content: string) => {
		inputValueRef.current = content;
		if (typeof window !== "undefined" && !sentRef.current) {
			if (content) {
				localStorage.setItem(emptyInputStorageKey, content);
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
	modelCatalog: TypesGen.ChatModelsResponse | null | undefined;
	modelOptions: readonly ChatModelOption[];
	isModelCatalogLoading: boolean;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	isModelConfigsLoading: boolean;
	modelCatalogError: unknown;
}

export const AgentCreateForm: FC<AgentCreateFormProps> = ({
	onCreateChat,
	isCreating,
	createError,
	modelCatalog,
	modelOptions,
	modelConfigs,
	isModelCatalogLoading,
	isModelConfigsLoading,
	modelCatalogError,
}) => {
	const { organizations } = useDashboard();
	const { initialInputValue, handleContentChange, submitDraft, resetDraft } =
		useEmptyStateDraft();
	const [initialLastModelConfigID] = useState(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(lastModelConfigIDStorageKey) ?? "";
	});
	const modelIDByConfigID = (() => {
		const optionIDByRef = new Map<string, string>();
		for (const option of modelOptions) {
			const provider = option.provider.trim().toLowerCase();
			const model = option.model.trim();
			if (!provider || !model) {
				continue;
			}
			const key = `${provider}:${model}`;
			if (!optionIDByRef.has(key)) {
				optionIDByRef.set(key, option.id);
			}
		}

		const byConfigID = new Map<string, string>();
		for (const config of modelConfigs) {
			const { provider, model } = getNormalizedModelRef(config);
			if (!provider || !model) {
				continue;
			}
			const modelID = optionIDByRef.get(`${provider}:${model}`);
			if (!modelID || byConfigID.has(config.id)) {
				continue;
			}
			byConfigID.set(config.id, modelID);
		}
		return byConfigID;
	})();
	const lastUsedModelID = initialLastModelConfigID
		? (modelIDByConfigID.get(initialLastModelConfigID) ?? "")
		: "";
	const defaultModelID = (() => {
		const defaultModelConfig = modelConfigs.find((config) => config.is_default);
		if (!defaultModelConfig) {
			return "";
		}
		return modelIDByConfigID.get(defaultModelConfig.id) ?? "";
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
	const [workspacePopoverOpen, setWorkspacePopoverOpen] = useState(false);
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => {
			if (typeof window === "undefined") return null;
			return localStorage.getItem(selectedWorkspaceIdStorageKey) || null;
		},
	);
	const workspaceOptions = workspacesQuery.data?.workspaces ?? [];
	const autoCreateWorkspaceValue = "__auto_create_workspace__";
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(modelCatalog);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelCatalogLoading,
		hasConfiguredModels,
	);
	const modelCatalogStatusMessage = getModelCatalogStatusMessage(
		modelCatalog,
		modelOptions,
		isModelCatalogLoading,
		Boolean(modelCatalogError),
	);
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";

	useEffect(() => {
		if (typeof window === "undefined") {
			return;
		}
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
	useEffect(() => {
		selectedWorkspaceIdRef.current = selectedWorkspaceId;
		selectedModelRef.current = selectedModel;
	});

	const handleWorkspaceChange = (value: string) => {
		if (value === autoCreateWorkspaceValue) {
			setSelectedWorkspaceId(null);
			if (typeof window !== "undefined") {
				localStorage.removeItem(selectedWorkspaceIdStorageKey);
			}
			return;
		}
		setSelectedWorkspaceId(value);
		if (typeof window !== "undefined") {
			localStorage.setItem(selectedWorkspaceIdStorageKey, value);
		}
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
		}).catch(() => {
			// Re-enable draft persistence so the user can edit
			// and retry after a failed send attempt.
			resetDraft();
		});
	};

	const selectedWorkspace = selectedWorkspaceId
		? workspaceOptions.find((ws) => ws.id === selectedWorkspaceId)
		: undefined;
	const selectedWorkspaceLabel = selectedWorkspace
		? `${selectedWorkspace.owner_name}/${selectedWorkspace.name}`
		: undefined;

	const {
		attachments,
		uploadStates,
		previewUrls,
		handleAttach,
		handleRemoveAttachment,
		resetAttachments,
	} = useFileAttachments(organizations[0]?.id);

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
		<div className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-4 pt-12 md:h-full md:items-center md:pt-4">
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
				{createError ? (
					isApiError(createError) &&
					createError.response?.status === 409 &&
					isUsageLimitData(createError.response.data) ? (
						<Alert
							severity="info"
							className="py-2"
							actions={
								<Button asChild variant="subtle" size="sm">
									<Link to="/agents/analytics">View Usage</Link>
								</Button>
							}
						>
							{formatUsageLimitMessage(createError.response.data)}
						</Alert>
					) : (
						<ErrorAlert error={createError} />
					)
				) : null}
				{workspacesQuery.isError && (
					<ErrorAlert error={workspacesQuery.error} />
				)}

				<AgentChatInput
					onSend={handleSendWithAttachments}
					placeholder="Ask Coder to build, fix bugs, or explore your project..."
					isDisabled={isCreating}
					isLoading={isCreating}
					initialValue={initialInputValue}
					onContentChange={handleContentChange}
					selectedModel={selectedModel}
					onModelChange={handleModelChange}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					inputStatusText={inputStatusText}
					modelCatalogStatusMessage={modelCatalogStatusMessage}
					attachments={attachments}
					onAttach={handleAttach}
					onRemoveAttachment={handleRemoveAttachment}
					uploadStates={uploadStates}
					previewUrls={previewUrls}
					leftActions={
						<Popover
							open={workspacePopoverOpen}
							onOpenChange={setWorkspacePopoverOpen}
						>
							{/* pointer-events-auto overrides the pointer-events:none
									   that Radix Select's DismissableLayer sets on
									   document.body when the Model Selector is open.
									   Without it the first click only dismisses the
									   Select and a second click is needed to open
									   the popover. */}
							<PopoverTrigger asChild>
								<button
									type="button"
									disabled={isCreating || workspacesQuery.isLoading}
									className="pointer-events-auto group flex h-8 items-center gap-1.5 rounded-md border-none bg-transparent px-1 text-xs text-content-secondary shadow-none ring-offset-background transition-colors hover:bg-transparent hover:text-content-primary focus:outline-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
								>
									<MonitorIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary transition-colors group-hover:text-content-primary" />
									<span>{selectedWorkspaceLabel ?? "Workspace"}</span>
									<ChevronDownIcon className="size-icon-sm text-content-secondary transition-colors group-hover:text-content-primary" />
								</button>
							</PopoverTrigger>
							<PopoverContent side="top" align="start" className="w-72 p-0">
								<Command loop>
									<CommandInput placeholder="Search workspaces..." />
									<CommandList>
										<CommandEmpty>No workspaces found</CommandEmpty>
										<CommandGroup>
											<CommandItem
												value="Auto-create Workspace"
												onSelect={() => {
													handleWorkspaceChange(autoCreateWorkspaceValue);
													setWorkspacePopoverOpen(false);
												}}
											>
												Auto-create Workspace
												{selectedWorkspaceId == null && (
													<Check className="ml-auto size-icon-sm shrink-0" />
												)}
											</CommandItem>
											{workspaceOptions.map((workspace) => (
												<CommandItem
													key={workspace.id}
													value={`${workspace.owner_name}/${workspace.name}`}
													onSelect={() => {
														handleWorkspaceChange(workspace.id);
														setWorkspacePopoverOpen(false);
													}}
												>
													{workspace.owner_name}/{workspace.name}
													{selectedWorkspaceId === workspace.id && (
														<Check className="ml-auto size-icon-sm shrink-0" />
													)}
												</CommandItem>
											))}
										</CommandGroup>
									</CommandList>
								</Command>
							</PopoverContent>
						</Popover>
					}
				/>
				<p className="mt-1 text-center text-xs text-content-secondary/50">
					Coder Agents is available via{" "}
					<a
						href="https://coder.com/docs/ai-coder/agents/early-access"
						target="_blank"
						rel="noreferrer"
						className="text-content-secondary/50 underline hover:text-content-secondary"
					>
						Early Access
					</a>
				</p>
			</div>
		</div>
	);
};
