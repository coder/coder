import type { ChatModelsResponse } from "api/api";
import { getErrorMessage } from "api/errors";
import { chatModels, chats, createChat, deleteChat } from "api/queries/chats";
import { deploymentConfig } from "api/queries/deployment";
import { workspaces } from "api/queries/workspaces";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import type { ModelSelectorOption } from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "components/Dialog/Dialog";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { useAuthenticated } from "hooks";
import type { LucideIcon } from "lucide-react";
import { BoxesIcon, KeyRoundIcon, Loader2Icon, MonitorIcon, UserIcon, XIcon } from "lucide-react";
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
const selectedWorkspaceIdStorageKey = "agents.selected-workspace-id";
const selectedWorkspaceModeStorageKey = "agents.selected-workspace-mode";
const defaultContextCompressionThreshold = "70";

const contextCompressionThresholdStorageKey = (modelID: string) =>
	`agents.context-compression-threshold.${modelID || "default"}`;

const parseContextCompressionThreshold = (
	value: string,
): number | undefined => {
	const parsedValue = Number.parseInt(value.trim(), 10);
	if (!Number.isFinite(parsedValue)) {
		return undefined;
	}
	if (parsedValue < 0 || parsedValue > 100) {
		return undefined;
	}
	return parsedValue;
};

type ChatModelOption = ModelSelectorOption;

type CreateChatOptions = {
	message: string;
	workspaceId?: string;
	workspaceMode?: "local";
	model?: string;
	systemPrompt?: string;
	contextCompressionThreshold?: number;
};

type ConfigureAgentsSection = "providers" | "system-prompt" | "models";

type ConfigureAgentsSectionOption = {
	id: ConfigureAgentsSection;
	label: string;
	icon: LucideIcon;
};

export interface AgentsOutletContext {
	chatErrorReasons: Record<string, string>;
	setChatErrorReason: (chatId: string, reason: string) => void;
	clearChatErrorReason: (chatId: string) => void;
	topBarTitleRef: React.RefObject<HTMLDivElement | null>;
	topBarActionsRef: React.RefObject<HTMLDivElement | null>;
	rightPanelRef: React.RefObject<HTMLDivElement | null>;
	setRightPanelOpen: (isOpen: boolean) => void;
	requestArchiveAgent: (chatId: string) => void;
}

export const AgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions, user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();
	const isAgentsAdmin =
		permissions.editDeploymentConfig ||
		user.roles.some((role) => role.name === "owner" || role.name === "admin");
	const canSetSystemPrompt = isAgentsAdmin;

	const chatsQuery = useQuery(chats());
	const chatModelsQuery = useQuery(chatModels());
	const deploymentConfigQuery = useQuery({
		...deploymentConfig(),
		enabled: isAgentsAdmin,
	});
	const createMutation = useMutation(createChat(queryClient));
	const archiveMutation = useMutation(deleteChat(queryClient));
	const [archiveTargetChatId, setArchiveTargetChatId] = useState<string | null>(
		null,
	);
	const [isRightPanelOpen, setIsRightPanelOpen] = useState(false);
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
	const rightPanelRef = useRef<HTMLDivElement>(null);
	const requestArchiveAgent = useCallback((chatId: string) => {
		setArchiveTargetChatId(chatId);
	}, []);
	const outletContext: AgentsOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		topBarTitleRef,
		topBarActionsRef,
		rightPanelRef,
		setRightPanelOpen: setIsRightPanelOpen,
		requestArchiveAgent,
	};
	const canUseLocalWorkspaceMode =
		isAgentsAdmin &&
		Boolean(deploymentConfigQuery.data?.config.ai?.chat?.local_workspace);

	const handleCreateChat = async (options: CreateChatOptions) => {
		const {
			message,
			workspaceId,
			workspaceMode,
			model,
			systemPrompt,
			contextCompressionThreshold,
		} = options;
		const createdChat = await createMutation.mutateAsync({
			message,
			input: {
				parts: [{ type: "text", text: message }],
			},
			workspace_id: workspaceId,
			workspace_mode: workspaceMode,
			model,
			system_prompt: systemPrompt,
			context_compression_threshold: contextCompressionThreshold,
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

	useEffect(() => {
		if (!agentId) {
			setIsRightPanelOpen(false);
		}
	}, [agentId]);

	const chatList = chatsQuery.data ?? [];
	const archiveTargetChat = archiveTargetChatId
		? chatList.find((chat) => chat.id === archiveTargetChatId)
		: undefined;

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
					onArchiveAgent={requestArchiveAgent}
					onNewAgent={handleNewAgent}
					isCreating={createMutation.isPending}
					isArchiving={archiveMutation.isPending}
					archivingChatId={archiveTargetChatId}
					isLoading={chatsQuery.isLoading}
					loadError={chatsQuery.isError ? chatsQuery.error : undefined}
					onRetryLoad={() => void chatsQuery.refetch()}
				/>
			</div>

			<div
				className={cn(
					"flex min-h-0 min-w-0 flex-1 bg-surface-primary",
					isRightPanelOpen && "flex-col xl:flex-row",
				)}
			>
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
							canUseLocalWorkspaceMode={canUseLocalWorkspaceMode}
							topBarActionsRef={topBarActionsRef}
						/>
					)}
				</div>
				{agentId && isRightPanelOpen && (
					<div
						ref={rightPanelRef}
						data-testid="agents-detail-right-panel"
						className="min-h-0 min-w-0 border-t border-border-default bg-surface-primary h-[42dvh] min-h-[260px] max-h-[56dvh] xl:h-auto xl:max-h-none xl:w-[40%] xl:min-w-[360px] xl:max-w-[720px] xl:border-l xl:border-t-0"
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
	canUseLocalWorkspaceMode: boolean;
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
	canUseLocalWorkspaceMode,
	topBarActionsRef,
}) => {
	const initialInput = useMemo(() => {
		if (typeof window === "undefined") {
			return "";
		}
		return localStorage.getItem(emptyInputStorageKey) ?? "";
	}, []);
	const [selectedModel, setSelectedModel] = useState(modelOptions[0]?.id ?? "");
	const [contextCompressionThreshold, setContextCompressionThreshold] =
		useState(defaultContextCompressionThreshold);
	const [systemPrompt, setSystemPrompt] = useState("");
	const [isConfigureAgentsDialogOpen, setConfigureAgentsDialogOpen] =
		useState(false);
	const [activeConfigureSection, setActiveConfigureSection] =
		useState<ConfigureAgentsSection>("providers");
	const workspacesQuery = useQuery(workspaces({ limit: 50 }));
	const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | null>(
		() => {
			if (typeof window === "undefined") return null;
			return localStorage.getItem(selectedWorkspaceIdStorageKey) || null;
		},
	);
	const [selectedWorkspaceMode, setSelectedWorkspaceMode] = useState<
		"workspace" | "local"
	>(() => {
		if (typeof window === "undefined") return "workspace";
		const stored = localStorage.getItem(selectedWorkspaceModeStorageKey);
		if (stored === "local") return "local";
		return "workspace";
	});
	const workspaceOptions = workspacesQuery.data?.workspaces ?? [];
	const autoCreateWorkspaceValue = "__auto_create_workspace__";
	const localWorkspaceValue = "__local_workspace__";
	const hasAdminControls = canSetSystemPrompt || canManageChatModelConfigs;
	const configureSectionOptions = useMemo<
		readonly ConfigureAgentsSectionOption[]
	>(() => {
		const options: ConfigureAgentsSectionOption[] = [];
		if (canManageChatModelConfigs) {
			options.push({
				id: "providers",
				label: "Providers",
				icon: KeyRoundIcon,
			});
			options.push({
				id: "models",
				label: "Models",
				icon: BoxesIcon,
			});
		}
		if (canSetSystemPrompt) {
			options.push({
				id: "system-prompt",
				label: "Behavior",
				icon: UserIcon,
			});
		}
		return options;
	}, [canManageChatModelConfigs, canSetSystemPrompt]);
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
	const selectedWorkspaceModeRef = useRef(selectedWorkspaceMode);
	selectedWorkspaceModeRef.current = selectedWorkspaceMode;
	const selectedModelRef = useRef(selectedModel);
	selectedModelRef.current = selectedModel;
	const contextCompressionThresholdRef = useRef(contextCompressionThreshold);
	contextCompressionThresholdRef.current = contextCompressionThreshold;
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

	useEffect(() => {
		if (typeof window === "undefined") {
			return;
		}
		const storedThreshold = localStorage.getItem(
			contextCompressionThresholdStorageKey(selectedModel),
		);
		setContextCompressionThreshold(
			storedThreshold ?? defaultContextCompressionThreshold,
		);
	}, [selectedModel]);

	useEffect(() => {
		if (
			configureSectionOptions.some((section) => section.id === activeConfigureSection)
		) {
			return;
		}
		setActiveConfigureSection(configureSectionOptions[0]?.id ?? "providers");
	}, [activeConfigureSection, configureSectionOptions]);

	const handleWorkspaceChange = (value: string) => {
		if (value === autoCreateWorkspaceValue) {
			setSelectedWorkspaceMode("workspace");
			setSelectedWorkspaceId(null);
			if (typeof window !== "undefined") {
				localStorage.setItem(selectedWorkspaceModeStorageKey, "workspace");
				localStorage.removeItem(selectedWorkspaceIdStorageKey);
			}
			return;
		}
		if (value === localWorkspaceValue && canUseLocalWorkspaceMode) {
			setSelectedWorkspaceMode("local");
			setSelectedWorkspaceId(null);
			if (typeof window !== "undefined") {
				localStorage.setItem(selectedWorkspaceModeStorageKey, "local");
				localStorage.removeItem(selectedWorkspaceIdStorageKey);
			}
			return;
		}
		setSelectedWorkspaceMode("workspace");
		setSelectedWorkspaceId(value);
		if (typeof window !== "undefined") {
			localStorage.setItem(selectedWorkspaceModeStorageKey, "workspace");
			localStorage.setItem(selectedWorkspaceIdStorageKey, value);
		}
	};

	const handleOpenConfigureAgentsDialog = () => {
		const initialSection =
			configureSectionOptions.find((section) => section.id === "providers")
				?.id ??
			configureSectionOptions[0]?.id ??
			"providers";
		setActiveConfigureSection(initialSection);
		setConfigureAgentsDialogOpen(true);
	};

	const handleInputChange = useCallback((value: string) => {
		if (typeof window !== "undefined") {
			localStorage.setItem(emptyInputStorageKey, value);
		}
	}, []);

	const handleContextCompressionThresholdChange = useCallback(
		(value: string) => {
			setContextCompressionThreshold(value);
			if (typeof window !== "undefined") {
				localStorage.setItem(
					contextCompressionThresholdStorageKey(selectedModelRef.current),
					value,
				);
			}
		},
		[],
	);

	const handleSend = useCallback(
		async (message: string) => {
			const trimmedSystemPrompt = systemPromptRef.current.trim();
			const localWorkspaceMode = selectedWorkspaceModeRef.current === "local";
			const parsedCompressionThreshold = parseContextCompressionThreshold(
				contextCompressionThresholdRef.current,
			);
			await onCreateChat({
				message,
				workspaceId: localWorkspaceMode
					? undefined
					: selectedWorkspaceIdRef.current ?? undefined,
				workspaceMode: localWorkspaceMode ? "local" : undefined,
				model: selectedModelRef.current || undefined,
				systemPrompt:
					canSetSystemPrompt && trimmedSystemPrompt
						? trimmedSystemPrompt
						: undefined,
				contextCompressionThreshold: parsedCompressionThreshold,
			});
		},
		[onCreateChat, canSetSystemPrompt],
	);

	useEffect(() => {
		if (!canUseLocalWorkspaceMode && selectedWorkspaceMode === "local") {
			setSelectedWorkspaceMode("workspace");
		}
	}, [canUseLocalWorkspaceMode, selectedWorkspaceMode]);

	const selectedWorkspaceName =
		selectedWorkspaceMode === "local"
			? "Local Workspace"
			: selectedWorkspaceId
				? workspaceOptions.find((ws) => ws.id === selectedWorkspaceId)?.name
				: null;

	return (
		<div className="flex h-full min-h-0 flex-1 items-center justify-center overflow-auto p-4 sm:p-6 lg:p-8">
			{hasAdminControls &&
				topBarActionsRef.current &&
				createPortal(
					<Button
						variant="subtle"
						disabled={isCreating}
						className="h-8 gap-1.5 border-none bg-transparent px-1 text-[13px] shadow-none hover:bg-transparent"
						onClick={handleOpenConfigureAgentsDialog}
					>
						Admin
					</Button>,
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
					contextCompressionThreshold={contextCompressionThreshold}
					onContextCompressionThresholdChange={
						handleContextCompressionThresholdChange
					}
					leftActions={
						<Select
							value={
								selectedWorkspaceMode === "local"
									? localWorkspaceValue
									: selectedWorkspaceId ?? autoCreateWorkspaceValue
							}
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
								{canUseLocalWorkspaceMode && (
									<SelectItem value={localWorkspaceValue}>
										Local Workspace
									</SelectItem>
								)}
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

			{hasAdminControls && (
				<Dialog
					open={isConfigureAgentsDialogOpen}
					onOpenChange={setConfigureAgentsDialogOpen}
				>
					<DialogContent className="grid h-[min(88dvh,720px)] max-w-4xl grid-cols-1 gap-0 overflow-hidden p-0 md:grid-cols-[200px_minmax(0,1fr)]">
						{/* Visually hidden for accessibility */}
						<DialogHeader className="sr-only">
							<DialogTitle>Configure Agents</DialogTitle>
							<DialogDescription>
								Manage providers, system prompt, and available models.
							</DialogDescription>
						</DialogHeader>

						{/* Sidebar */}
						<nav className="flex flex-row gap-0.5 overflow-x-auto border-b border-border p-2 md:flex-col md:overflow-x-visible md:border-b-0 md:border-r md:p-3">
							<DialogClose asChild>
								<Button
									variant="subtle"
									size="icon"
									className="mb-2 h-8 w-8 shrink-0 border-none bg-transparent shadow-none hover:bg-surface-tertiary/30"
								>
									<XIcon className="h-[18px] w-[18px] text-content-secondary" />
									<span className="sr-only">Close</span>
								</Button>
							</DialogClose>
							{configureSectionOptions.map((section) => {
								const isActive = section.id === activeConfigureSection;
								const SectionIcon = section.icon;
								return (
									<Button
										key={section.id}
										variant="subtle"
										className={cn(
											"h-auto justify-start gap-2.5 rounded-lg border-none px-3 py-2 text-left shadow-none",
											isActive
												? "bg-surface-tertiary/50 text-content-primary hover:bg-surface-tertiary/50"
												: "bg-transparent text-content-secondary hover:bg-surface-tertiary/30 hover:text-content-primary",
										)}
										onClick={() => setActiveConfigureSection(section.id)}
									>
										<SectionIcon className="h-[18px] w-[18px] shrink-0" />
										<span className="text-[13px] font-medium">{section.label}</span>
									</Button>
								);
							})}
						</nav>

						{/* Content */}
						<div className="flex min-h-0 flex-col pt-5">
							<h2 className="m-0 px-6 text-xl font-semibold text-content-primary">
								{configureSectionOptions.find(
									(s) => s.id === activeConfigureSection,
								)?.label ?? "Settings"}
							</h2>

							<ScrollArea className="min-h-0 flex-1" viewportClassName="px-6 pb-6">
								{activeConfigureSection === "providers" &&
									canManageChatModelConfigs && (
										<ChatModelAdminPanel section="providers" />
									)}
								{activeConfigureSection === "system-prompt" && canSetSystemPrompt && (
									<div className="space-y-4">
										<p className="m-0 text-[13px] leading-relaxed text-content-secondary">
											Configure how the AI agent behaves across this
											deployment.
										</p>
										<div className="space-y-2">
											<h3 className="m-0 text-[13px] font-semibold text-content-primary">
												System Prompt
											</h3>
											<p className="m-0 text-xs text-content-secondary">
												Admin-only instruction applied to all new chats.
											</p>
											<TextareaAutosize
												className="min-h-[220px] w-full resize-y rounded-lg border border-border bg-surface-primary px-4 py-3 font-sans text-[13px] leading-relaxed text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30"
												placeholder="Optional. Set deployment-wide instructions for all new chats."
												value={systemPrompt}
												onChange={(event) => setSystemPrompt(event.target.value)}
												disabled={isCreating}
												minRows={7}
											/>
											<div className="flex justify-end">
												<Button
													size="sm"
													variant="outline"
													onClick={() => setSystemPrompt("")}
													disabled={isCreating || !systemPrompt}
												>
													Clear
												</Button>
											</div>
										</div>
									</div>
								)}
								{activeConfigureSection === "models" &&
									canManageChatModelConfigs && (
										<ChatModelAdminPanel section="models" />
									)}
							</ScrollArea>
						</div>
					</DialogContent>
				</Dialog>
			)}
		</div>
	);
};

export default AgentsPage;
