import { type FC, useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Outlet, useNavigate, useParams } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import type { ChatModelsResponse } from "api/api";
import { getErrorMessage } from "api/errors";
import { chatModels, chats, createChat, deleteChat } from "api/queries/chats";
import { workspaces } from "api/queries/workspaces";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import {
	ModelSelector,
	type ModelSelectorOption,
} from "components/ai-elements";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { useAuthenticated } from "hooks";
import {
	ChevronDownIcon,
	Loader2Icon,
	SendIcon,
	Settings2Icon,
	SlidersHorizontalIcon,
} from "lucide-react";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentsSidebar } from "./AgentsSidebar";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel";
import {
	formatProviderLabel,
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";

const emptyInputStorageKey = "agents.empty-input";

type ChatModelOption = ModelSelectorOption;

type CreateChatOptions = {
	message: string;
	workspaceId?: string;
	model?: string;
	systemPrompt?: string;
};

export interface AgentsOutletContext {
	chatErrorReasons: Record<string, string>;
	setChatErrorReason: (chatId: string, reason: string) => void;
	clearChatErrorReason: (chatId: string) => void;
}

export const AgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions, user } = useAuthenticated();

	const chatsQuery = useQuery(chats());
	const chatModelsQuery = useQuery(chatModels());
	const createMutation = useMutation(createChat(queryClient));
	const archiveMutation = useMutation(deleteChat(queryClient));
	const [archiveTargetChatId, setArchiveTargetChatId] = useState<string | null>(
		null,
	);
	const [chatErrorReasons, setChatErrorReasons] = useState<
		Record<string, string>
	>({});
	const catalogModelOptions = useMemo(
		() => getModelOptionsFromCatalog(chatModelsQuery.data),
		[chatModelsQuery.data],
	);
	const setChatErrorReason = useCallback((chatId: string, reason: string) => {
		const trimmedReason = reason.trim();
		if (!chatId || !trimmedReason) {
			return;
		}
		setChatErrorReasons((current) => {
			if (current[chatId] === trimmedReason) {
				return current;
			}
			return {
				...current,
				[chatId]: trimmedReason,
			};
		});
	}, []);
	const clearChatErrorReason = useCallback((chatId: string) => {
		if (!chatId) {
			return;
		}
		setChatErrorReasons((current) => {
			if (!(chatId in current)) {
				return current;
			}
			const next = { ...current };
			delete next[chatId];
			return next;
		});
	}, []);
	const outletContext: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
	};
	const isAgentsAdmin =
		permissions.editDeploymentConfig ||
		user.roles.some((role) => role.name === "owner" || role.name === "admin");
	const canSetSystemPrompt = isAgentsAdmin;

	const handleCreateChat = async (options: CreateChatOptions) => {
		const { message, workspaceId, model, systemPrompt } = options;
		const createdChat = await createMutation.mutateAsync({
			message,
			input: {
				parts: [{ type: "text", text: message }],
			},
			workspace_id: workspaceId,
			model,
			system_prompt: systemPrompt,
		});

		if (typeof window !== "undefined") {
			localStorage.removeItem(emptyInputStorageKey);
		}

		navigate(`/agents/${createdChat.id}`);
	};

	const handleNewAgent = () => {
		if (typeof window !== "undefined") {
			localStorage.setItem(emptyInputStorageKey, "");
		}
		navigate("/agents");
	};

	useEffect(() => {
		document.title = pageTitle("Agents");
	}, []);

	if (chatsQuery.isLoading) {
		return <Loader />;
	}

	if (chatsQuery.isError) {
		return (
			<div className="flex h-full items-center justify-center p-6">
				<div className="w-full max-w-xl space-y-3">
					<ErrorAlert error={chatsQuery.error} />
					<Button
						size="sm"
						variant="outline"
						onClick={() => void chatsQuery.refetch()}
					>
						Retry
					</Button>
				</div>
			</div>
		);
	}

	const chatList = chatsQuery.data ?? [];
	const archiveTargetChat = archiveTargetChatId
		? chatList.find((chat) => chat.id === archiveTargetChatId)
		: undefined;

	const handleArchiveAgent = (chatId: string) => {
		setArchiveTargetChatId(chatId);
	};

	const handleCloseArchiveDialog = () => {
		if (archiveMutation.isPending) {
			return;
		}
		setArchiveTargetChatId(null);
	};

	const handleConfirmArchive = async () => {
		if (!archiveTargetChatId || archiveMutation.isPending) {
			return;
		}

		const archivedChatId = archiveTargetChatId;
		const nextChatId = chatList.find((chat) => chat.id !== archivedChatId)?.id;

		try {
			await archiveMutation.mutateAsync(archivedChatId);
			setArchiveTargetChatId(null);
			clearChatErrorReason(archivedChatId);
			displaySuccess("Agent archived.");

			if (archivedChatId === agentId) {
				navigate(nextChatId ? `/agents/${nextChatId}` : "/agents", {
					replace: true,
				});
			}
		} catch (error) {
			displayError(getErrorMessage(error, "Failed to archive agent."));
		}
	};

	return (
		<div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary md:flex-row">
			<div
				className={cn(
					"shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default md:h-full md:w-[320px] md:min-h-0 md:border-b-0",
					agentId && "hidden md:block",
				)}
			>
				<AgentsSidebar
					chats={chatList}
					chatErrorReasons={chatErrorReasons}
					modelOptions={catalogModelOptions}
					selectedChatId={agentId}
					onSelect={(id) => navigate(`/agents/${id}`)}
					onArchiveAgent={handleArchiveAgent}
					onNewAgent={handleNewAgent}
					isCreating={createMutation.isPending}
					isArchiving={archiveMutation.isPending}
					archivingChatId={archiveTargetChatId}
				/>
			</div>

			<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
				{agentId ? (
					<Outlet context={outletContext} />
				) : (
					<AgentsEmptyState
						onCreateChat={handleCreateChat}
						isCreating={createMutation.isPending}
						createError={createMutation.error}
						modelCatalog={chatModelsQuery.data}
						modelOptions={catalogModelOptions}
						isModelCatalogLoading={chatModelsQuery.isLoading}
						modelCatalogError={chatModelsQuery.error}
						canSetSystemPrompt={canSetSystemPrompt}
						canManageChatModelConfigs={isAgentsAdmin}
					/>
				)}
			</div>

			<Dialog
				open={archiveTargetChatId !== null}
				onOpenChange={(nextOpen) => {
					if (!nextOpen) {
						handleCloseArchiveDialog();
					}
				}}
			>
				<DialogContent className="max-w-md p-6">
					<DialogHeader className="space-y-2">
						<DialogTitle>Archive agent</DialogTitle>
						<DialogDescription>
							{archiveTargetChat
								? `Archive "${archiveTargetChat.title}"? This permanently deletes its chat history.`
								: "Archive this agent? This permanently deletes its chat history."}
						</DialogDescription>
					</DialogHeader>
					<DialogFooter className="gap-2">
						<Button
							size="sm"
							variant="outline"
							onClick={handleCloseArchiveDialog}
							disabled={archiveMutation.isPending}
						>
							Cancel
						</Button>
						<Button
							size="sm"
							variant="destructive"
							onClick={() => void handleConfirmArchive()}
							disabled={archiveMutation.isPending}
						>
							{archiveMutation.isPending && (
								<Loader2Icon className="h-4 w-4 animate-spin" />
							)}
							Archive
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
};

interface AgentsEmptyStateProps {
	onCreateChat: (options: CreateChatOptions) => Promise<void>;
	isCreating: boolean;
	createError: unknown;
	modelCatalog: ChatModelsResponse | null | undefined;
	modelOptions: readonly ChatModelOption[];
	isModelCatalogLoading: boolean;
	modelCatalogError: unknown;
	canSetSystemPrompt: boolean;
	canManageChatModelConfigs: boolean;
}

const AgentsEmptyState: FC<AgentsEmptyStateProps> = ({
	onCreateChat,
	isCreating,
	createError,
	modelCatalog,
	modelOptions,
	isModelCatalogLoading,
	modelCatalogError,
	canSetSystemPrompt,
	canManageChatModelConfigs,
}) => {
	const [input, setInput] = useState(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	});
	const [selectedModel, setSelectedModel] = useState(modelOptions[0]?.id ?? "");
	const [systemPrompt, setSystemPrompt] = useState("");
	const [isSystemPromptDialogOpen, setSystemPromptDialogOpen] = useState(false);
	const [isModelConfigDialogOpen, setModelConfigDialogOpen] = useState(false);
	const workspacesQuery = useQuery(workspaces({ limit: 50 }));
	const [selectedWorkspaceId, setSelectedWorkspaceId] =
		useState<string | null>(null);
	const workspaceOptions = workspacesQuery.data?.workspaces ?? [];
	const autoCreateWorkspaceValue = "__auto_create_workspace__";
	const hasAdminControls = canSetSystemPrompt || canManageChatModelConfigs;
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
	const isCreatingWorkspace = isCreating && selectedWorkspaceId === null;
	const submitStatusText = isCreatingWorkspace
		? "Starting agent. Workspace can be auto-created by AI tools."
		: isCreating
			? "Starting agent..."
			: hasModelOptions
				? "Press Enter to send"
				: hasConfiguredModels
					? "Models are configured but unavailable. Ask an admin."
					: "No models configured. Ask an admin.";

	useEffect(() => {
		if (typeof window === "undefined") {
			return;
		}
		localStorage.setItem(emptyInputStorageKey, input);
	}, [input]);

	useEffect(() => {
		setSelectedModel((current) => {
			if (current && modelOptions.some((model) => model.id === current)) {
				return current;
			}
			return modelOptions[0]?.id ?? "";
		});
	}, [modelOptions]);

	const handleWorkspaceChange = (value: string) => {
		if (value === autoCreateWorkspaceValue) {
			setSelectedWorkspaceId(null);
			return;
		}
		setSelectedWorkspaceId(value);
	};

	const handleSubmit = async () => {
		const message = input.trim();
		if (!message) {
			return;
		}
		if (isCreating) {
			return;
		}
		if (!hasModelOptions) {
			return;
		}
		const trimmedSystemPrompt = systemPrompt.trim();

		try {
			await onCreateChat({
				message,
				workspaceId: selectedWorkspaceId ?? undefined,
				model: selectedModel || undefined,
				systemPrompt:
					canSetSystemPrompt && trimmedSystemPrompt
						? trimmedSystemPrompt
						: undefined,
			});
			setInput("");
		} catch {
			// Error state is surfaced through createError.
		}
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			void handleSubmit();
		}
	};

	return (
		<div className="flex h-full min-h-0 flex-1 overflow-auto p-4 sm:p-6 lg:p-8">
			<div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
				{createError ? <ErrorAlert error={createError} /> : null}
				{workspacesQuery.isError && <ErrorAlert error={workspacesQuery.error} />}

				<div className="flex items-start justify-between gap-3">
					<div>
						<div className="text-sm font-medium text-content-primary">
							Start a new agent
						</div>
						<div className="text-xs text-content-secondary">
							Choose settings, then send your first message.
						</div>
					</div>
					{hasAdminControls && (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									size="sm"
									variant="outline"
									disabled={isCreating}
									className="shrink-0 gap-1.5"
								>
									<Settings2Icon className="h-3.5 w-3.5" />
									Admin
									<ChevronDownIcon className="h-3.5 w-3.5 text-content-secondary" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								{canSetSystemPrompt && (
									<DropdownMenuItem
										onSelect={(event) => {
											event.preventDefault();
											setSystemPromptDialogOpen(true);
										}}
									>
										<SlidersHorizontalIcon className="h-3.5 w-3.5" />
										System prompt
									</DropdownMenuItem>
								)}
								{canManageChatModelConfigs && (
									<DropdownMenuItem
										onSelect={(event) => {
											event.preventDefault();
											setModelConfigDialogOpen(true);
										}}
									>
										<Settings2Icon className="h-3.5 w-3.5" />
										Model config
									</DropdownMenuItem>
								)}
							</DropdownMenuContent>
						</DropdownMenu>
					)}
				</div>

				<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-4 shadow-sm">
					<div className="mb-3 flex flex-wrap items-center justify-between gap-2">
						<span className="text-xs font-medium text-content-primary">
							Session settings
						</span>
					</div>

					<div className="grid gap-3">
						<label className="flex flex-col gap-1 text-xs text-content-secondary">
							<span className="font-medium text-content-primary">Workspace</span>
							<Select
								value={selectedWorkspaceId ?? autoCreateWorkspaceValue}
								onValueChange={handleWorkspaceChange}
								disabled={isCreating || workspacesQuery.isLoading}
							>
								<SelectTrigger className="h-9 rounded-lg text-xs">
									<SelectValue
										placeholder={
											workspacesQuery.isLoading
												? "Loading workspaces..."
												: "Auto-create Workspace"
										}
									/>
								</SelectTrigger>
								<SelectContent>
									<SelectItem value={autoCreateWorkspaceValue}>
										Auto-create Workspace
									</SelectItem>
									{workspaceOptions.map((workspace) => (
										<SelectItem key={workspace.id} value={workspace.id}>
											{workspace.name}
										</SelectItem>
									))}
									{workspaceOptions.length === 0 &&
										!workspacesQuery.isLoading && (
											<SelectItem value="no-workspaces" disabled>
												No workspaces found
											</SelectItem>
										)}
								</SelectContent>
							</Select>
						</label>
					</div>

					{modelCatalogStatusMessage && (
						<div className="mt-2 text-2xs text-content-secondary">
							{modelCatalogStatusMessage}
						</div>
					)}
				</div>

				<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-2.5 shadow-sm focus-within:ring-2 focus-within:ring-content-link/40">
					<TextareaAutosize
						className="min-h-[170px] w-full resize-none rounded-xl border border-border-default/70 bg-surface-primary px-3 py-2 font-sans text-[15px] leading-6 text-content-primary shadow-sm placeholder:text-content-secondary focus:outline-none disabled:cursor-not-allowed disabled:opacity-70"
						placeholder="Ask Coder to build, fix bugs, or explore your project..."
						value={input}
						onChange={(e) => setInput(e.target.value)}
						onKeyDown={handleKeyDown}
						disabled={isCreating}
						minRows={5}
					/>
					<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
						<div className="flex min-w-0 items-center gap-2">
							<ModelSelector
								value={selectedModel}
								onValueChange={setSelectedModel}
								options={modelOptions}
								disabled={isCreating}
								placeholder={modelSelectorPlaceholder}
								formatProviderLabel={formatProviderLabel}
								dropdownSide="top"
								dropdownAlign="start"
								className="h-8 w-[220px] max-w-[65vw] justify-start rounded-lg bg-surface-secondary/60 text-xs"
							/>
							<span className="hidden text-xs text-content-secondary sm:inline">
								{submitStatusText}
							</span>
						</div>
						<Button
							size="icon"
							variant="default"
							onClick={() => void handleSubmit()}
							disabled={isCreating || !hasModelOptions || !input.trim()}
							className="shadow-sm"
						>
							{isCreating ? (
								<Loader2Icon className="animate-spin" />
							) : (
								<SendIcon />
							)}
							<span className="sr-only">Send</span>
						</Button>
					</div>
					<div className="px-2.5 pb-1 text-xs text-content-secondary sm:hidden">
						{submitStatusText}
					</div>
				</div>
			</div>

			{canSetSystemPrompt && (
				<Dialog
					open={isSystemPromptDialogOpen}
					onOpenChange={setSystemPromptDialogOpen}
				>
					<DialogContent className="max-w-2xl p-6">
						<DialogHeader className="space-y-2">
							<DialogTitle>System prompt</DialogTitle>
							<DialogDescription>
								Admin-only instruction applied when this chat is created.
							</DialogDescription>
						</DialogHeader>
						<TextareaAutosize
							className="min-h-[180px] w-full resize-y rounded-md border border-border bg-surface-primary px-3 py-2 font-sans text-sm text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30"
							placeholder="Optional. Set deployment-level instructions for this chat."
							value={systemPrompt}
							onChange={(e) => setSystemPrompt(e.target.value)}
							disabled={isCreating}
							minRows={6}
						/>
						<DialogFooter className="gap-2">
							<Button
								size="sm"
								variant="outline"
								onClick={() => setSystemPrompt("")}
								disabled={isCreating || !systemPrompt}
							>
								Clear
							</Button>
							<Button
								size="sm"
								variant="default"
								onClick={() => setSystemPromptDialogOpen(false)}
							>
								Done
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			)}

			{canManageChatModelConfigs && (
				<Dialog
					open={isModelConfigDialogOpen}
					onOpenChange={setModelConfigDialogOpen}
				>
					<DialogContent className="max-h-[85dvh] max-w-2xl overflow-y-auto p-5">
						<DialogTitle className="sr-only">
							Chat model configuration
						</DialogTitle>
						<DialogDescription className="sr-only">
							Admin-only controls for chat model providers and model configs.
						</DialogDescription>
						<ChatModelAdminPanel />
					</DialogContent>
				</Dialog>
			)}
		</div>
	);
};

export default AgentsPage;
