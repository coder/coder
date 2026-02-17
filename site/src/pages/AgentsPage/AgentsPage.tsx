import type { ChatModelsResponse } from "api/api";
import { getErrorMessage } from "api/errors";
import { chatModels, chats, createChat, deleteChat } from "api/queries/chats";
import { workspaces } from "api/queries/workspaces";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
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
	MonitorIcon,
	Settings2Icon,
	SlidersHorizontalIcon,
} from "lucide-react";
import { UserDropdown } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Outlet, useNavigate, useParams } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { AgentChatInput } from "./AgentChatInput";
import { AgentsSidebar } from "./AgentsSidebar";
import { ChatModelAdminPanel } from "./components/ChatModelAdminPanel";
import {
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
	topBarTitleRef: React.RefObject<HTMLDivElement | null>;
	topBarActionsRef: React.RefObject<HTMLDivElement | null>;
}

export const AgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions, user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();

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
	const topBarTitleRef = useRef<HTMLDivElement>(null);
	const topBarActionsRef = useRef<HTMLDivElement>(null);
	const outletContext: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		topBarTitleRef,
		topBarActionsRef,
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
					logoUrl={appearance.logo_url}
					onArchiveAgent={handleArchiveAgent}
					onNewAgent={handleNewAgent}
					isCreating={createMutation.isPending}
					isArchiving={archiveMutation.isPending}
					archivingChatId={archiveTargetChatId}
					isLoading={chatsQuery.isLoading}
					loadError={chatsQuery.isError ? chatsQuery.error : undefined}
					onRetryLoad={() => void chatsQuery.refetch()}
				/>
			</div>

			<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
				<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
					<div
						ref={topBarTitleRef}
						className="flex min-w-0 flex-1 items-center"
					/>
					<div ref={topBarActionsRef} className="flex items-center gap-2" />
					<div className="flex items-center [&_span]:!rounded-full [&_span]:!size-8 [&_span]:!text-xs">
						<UserDropdown
							user={user}
							buildInfo={buildInfo}
							supportLinks={
								appearance.support_links?.filter(
									(link) => link.location !== "navbar",
								) ?? []
							}
							onSignOut={signOut}
						/>
					</div>
				</div>
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
						topBarActionsRef={topBarActionsRef}
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
	topBarActionsRef: React.RefObject<HTMLDivElement | null>;
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
	topBarActionsRef,
}) => {
	const initialInput = useMemo(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	}, []);
	const [selectedModel, setSelectedModel] = useState(modelOptions[0]?.id ?? "");
	const [systemPrompt, setSystemPrompt] = useState("");
	const [isSystemPromptDialogOpen, setSystemPromptDialogOpen] = useState(false);
	const [isModelConfigDialogOpen, setModelConfigDialogOpen] = useState(false);
	const workspacesQuery = useQuery(workspaces({ limit: 50 }));
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		null,
	);
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
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";

	// Keep a mutable ref to selectedWorkspaceId and selectedModel so
	// that the onSend callback always sees the latest values without
	// the shared input component re-rendering on every change.
	const selectedWorkspaceIdRef = useRef(selectedWorkspaceId);
	selectedWorkspaceIdRef.current = selectedWorkspaceId;
	const selectedModelRef = useRef(selectedModel);
	selectedModelRef.current = selectedModel;
	const systemPromptRef = useRef(systemPrompt);
	systemPromptRef.current = systemPrompt;

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

	const handleInputChange = useCallback((value: string) => {
		if (typeof window !== "undefined") {
			localStorage.setItem(emptyInputStorageKey, value);
		}
	}, []);

	const handleSend = useCallback(
		async (message: string) => {
			const trimmedSystemPrompt = systemPromptRef.current.trim();
			await onCreateChat({
				message,
				workspaceId: selectedWorkspaceIdRef.current ?? undefined,
				model: selectedModelRef.current || undefined,
				systemPrompt:
					canSetSystemPrompt && trimmedSystemPrompt
						? trimmedSystemPrompt
						: undefined,
			});
		},
		[onCreateChat, canSetSystemPrompt],
	);

	const selectedWorkspaceName = selectedWorkspaceId
		? workspaceOptions.find((ws) => ws.id === selectedWorkspaceId)?.name
		: null;

	return (
		<div className="flex h-full min-h-0 flex-1 items-center justify-center overflow-auto p-4 sm:p-6 lg:p-8">
			{hasAdminControls &&
				topBarActionsRef.current &&
				createPortal(
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="subtle"
								disabled={isCreating}
								className="h-8 gap-1.5 border-none bg-transparent px-1 text-[13px] shadow-none hover:bg-transparent"
							>
								Admin
								<ChevronDownIcon className="h-2.5 w-2.5 text-content-secondary" />
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
					</DropdownMenu>,
					topBarActionsRef.current,
				)}

			<div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
				{createError ? <ErrorAlert error={createError} /> : null}
				{workspacesQuery.isError && (
					<ErrorAlert error={workspacesQuery.error} />
				)}

				<AgentChatInput
					onSend={handleSend}
					placeholder="Ask Coder to build, fix bugs, or explore your project..."
					isDisabled={isCreating}
					isLoading={isCreating}
					initialValue={initialInput}
					onInputChange={handleInputChange}
					selectedModel={selectedModel}
					onModelChange={setSelectedModel}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					inputStatusText={inputStatusText}
					modelCatalogStatusMessage={modelCatalogStatusMessage}
					leftActions={
						<Select
							value={selectedWorkspaceId ?? autoCreateWorkspaceValue}
							onValueChange={handleWorkspaceChange}
							disabled={isCreating || workspacesQuery.isLoading}
						>
							<SelectTrigger className="h-8 w-auto gap-1.5 border-none bg-transparent px-1 text-xs shadow-none hover:bg-transparent">
								<MonitorIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
								<SelectValue>
									{selectedWorkspaceName ?? "Workspace"}
								</SelectValue>
							</SelectTrigger>
							<SelectContent side="top">
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
					}
				/>
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
