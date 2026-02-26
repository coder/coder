import type { ChatDiffStatusResponse } from "api/api";
import { API } from "api/api";
import {
	chat,
	chatDiffStatus,
	chatModelConfigs,
	chatModels,
	chats,
	createChatMessage,
	deleteChatQueuedMessage,
	interruptChat,
	promoteChatQueuedMessage,
} from "api/queries/chats";
import { workspaceById } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Skeleton } from "components/Skeleton/Skeleton";
import { useAuthenticated } from "hooks";
import {
	ArchiveIcon,
	ChevronRightIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	MonitorIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
} from "lucide-react";
import { getVSCodeHref, SESSION_TOKEN_PLACEHOLDER } from "modules/apps/apps";
import { UserDropdown } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import { toast } from "sonner";
import { cn } from "utils/cn";
import { AgentChatInput } from "./AgentChatInput";
import { ConversationTimeline } from "./AgentDetail/ConversationTimeline";
import {
	getLatestContextUsage,
	getParentChatID,
	getWorkspaceAgent,
} from "./AgentDetail/chatHelpers";
import {
	buildParsedMessageSections,
	buildSubagentTitles,
	parseMessagesWithMergedTools,
} from "./AgentDetail/messageParsing";
import { buildStreamTools } from "./AgentDetail/streamState";
import { useChatStream } from "./AgentDetail/useChatStream";
import { useMessageWindow } from "./AgentDetail/useMessageWindow";
import type { AgentsOutletContext } from "./AgentsPage";
import { FilesChangedPanel } from "./FilesChangedPanel";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";
import { QueuedMessagesList } from "./QueuedMessagesList";

interface DiffStatsBadgeProps {
	status: ChatDiffStatusResponse;
	isOpen: boolean;
	onToggle: () => void;
}

const DiffStatsBadge: FC<DiffStatsBadgeProps> = ({
	status,
	isOpen,
	onToggle,
}) => {
	const additions = status.additions ?? 0;
	const deletions = status.deletions ?? 0;

	return (
		<Button
			variant="subtle"
			onClick={onToggle}
			className="h-auto min-w-0 gap-3 px-2 py-1 shadow-none hover:bg-transparent"
		>
			<span className="font-mono text-sm font-semibold text-content-success">
				+{additions}
			</span>
			<span className="font-mono text-sm font-semibold text-content-destructive">
				−{deletions}
			</span>
			{isOpen ? <PanelRightCloseIcon /> : <PanelRightOpenIcon />}
		</Button>
	);
};

const noopSetChatErrorReason: AgentsOutletContext["setChatErrorReason"] =
	() => {};
const noopClearChatErrorReason: AgentsOutletContext["clearChatErrorReason"] =
	() => {};
const noopRequestArchiveAgent: AgentsOutletContext["requestArchiveAgent"] =
	() => {};
const lastModelConfigIDStorageKey = "agents.last-model-config-id";

/**
 * Renders the top bar shared by all AgentDetail states (loading,
 * error, and loaded). The UserDropdown is always visible so the
 * page never looks broken while data is loading.
 */
const AgentDetailTopBar: FC<{ children?: React.ReactNode }> = ({
	children,
}) => {
	const { user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();

	return (
		<div className="flex shrink-0 items-center gap-2 px-4 py-0.5">
			{children ?? <div className="flex-1" />}
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
	);
};

export const AgentDetail: FC = () => {
	const navigate = useNavigate();
	const { agentId } = useParams<{ agentId: string }>();
	const outletContext = useOutletContext<AgentsOutletContext | undefined>();
	const queryClient = useQueryClient();
	const [selectedModel, setSelectedModel] = useState("");
	const [showDiffPanel, setShowDiffPanel] = useState(false);
	const chatErrorReasons = outletContext?.chatErrorReasons ?? {};
	const setChatErrorReason =
		outletContext?.setChatErrorReason ?? noopSetChatErrorReason;
	const clearChatErrorReason =
		outletContext?.clearChatErrorReason ?? noopClearChatErrorReason;
	const requestArchiveAgent =
		outletContext?.requestArchiveAgent ?? noopRequestArchiveAgent;
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatsQuery = useQuery(chats());
	const workspaceId = chatQuery.data?.chat?.workspace_id;
	const workspaceAgentId = chatQuery.data?.chat?.workspace_agent_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const diffStatusQuery = useQuery({
		...chatDiffStatus(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const hasDiffStatus = Boolean(diffStatusQuery.data?.url);
	const workspace = workspaceQuery.data;
	const workspaceAgent = getWorkspaceAgent(workspace, workspaceAgentId);
	const chatData = chatQuery.data;
	const chatRecord = chatData?.chat;
	const chatMessages = chatData?.messages;
	const chatQueuedMessages = chatData?.queued_messages;
	const chatLastModelConfigID = chatRecord?.last_model_config_id;

	// Auto-open the diff panel when diff status first appears.
	// See: https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes
	const [prevHasDiffStatus, setPrevHasDiffStatus] = useState(false);
	if (hasDiffStatus !== prevHasDiffStatus) {
		setPrevHasDiffStatus(hasDiffStatus);
		if (hasDiffStatus) {
			setShowDiffPanel(true);
		}
	}

	const modelOptions = useMemo(
		() =>
			getModelOptionsFromCatalog(
				chatModelsQuery.data,
				chatModelConfigsQuery.data,
			),
		[chatModelsQuery.data, chatModelConfigsQuery.data],
	);
	const modelConfigIDByModelID = useMemo(() => {
		const byModelID = new Map<string, string>();
		for (const config of chatModelConfigsQuery.data ?? []) {
			const provider = config.provider.trim().toLowerCase();
			const model = config.model.trim();
			if (!provider || !model) {
				continue;
			}
			const colonRef = `${provider}:${model}`;
			if (!byModelID.has(colonRef)) {
				byModelID.set(colonRef, config.id);
			}
			const slashRef = `${provider}/${model}`;
			if (!byModelID.has(slashRef)) {
				byModelID.set(slashRef, config.id);
			}
		}
		return byModelID;
	}, [chatModelConfigsQuery.data]);
	const modelIDByConfigID = useMemo(() => {
		const byConfigID = new Map<string, string>();
		for (const [modelID, configID] of modelConfigIDByModelID.entries()) {
			if (!byConfigID.has(configID)) {
				byConfigID.set(configID, modelID);
			}
		}
		return byConfigID;
	}, [modelConfigIDByModelID]);

	const sendMutation = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const interruptMutation = useMutation(
		interruptChat(queryClient, agentId ?? ""),
	);
	const deleteQueuedMutation = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const promoteQueuedMutation = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
	);

	const {
		messagesById,
		streamState,
		chatStatus,
		streamError,
		queuedMessages,
		subagentStatusOverrides,
		clearStreamError,
	} = useChatStream({
		chatId: agentId,
		chatMessages,
		chatRecord,
		chatData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	});

	useEffect(() => {
		setSelectedModel((current) => {
			if (current && modelOptions.some((model) => model.id === current)) {
				return current;
			}
			if (chatLastModelConfigID) {
				const fromChat = modelIDByConfigID.get(chatLastModelConfigID);
				if (fromChat && modelOptions.some((model) => model.id === fromChat)) {
					return fromChat;
				}
			}
			return modelOptions[0]?.id ?? "";
		});
	}, [chatLastModelConfigID, modelIDByConfigID, modelOptions]);

	const messages = useMemo(() => {
		const list = Array.from(messagesById.values());
		list.sort(
			(a, b) =>
				new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
		);
		return list;
	}, [messagesById]);
	const compressionThreshold = useMemo(() => {
		if (!chatLastModelConfigID) {
			return undefined;
		}
		const config = chatModelConfigsQuery.data?.find(
			(c) => c.id === chatLastModelConfigID,
		);
		return config?.compression_threshold;
	}, [chatLastModelConfigID, chatModelConfigsQuery.data]);
	const latestContextUsage = useMemo(() => {
		const usage = getLatestContextUsage(messages);
		if (!usage) {
			return usage;
		}
		return { ...usage, compressionThreshold };
	}, [messages, compressionThreshold]);
	const isStreaming =
		Boolean(streamState) ||
		chatStatus === "running" ||
		chatStatus === "pending";
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(
		chatModelsQuery.data,
	);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		chatModelsQuery.isLoading,
		hasConfiguredModels,
	);
	const modelCatalogStatusMessage = getModelCatalogStatusMessage(
		chatModelsQuery.data,
		modelOptions,
		chatModelsQuery.isLoading,
		Boolean(chatModelsQuery.error),
	);
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";
	const isSubmissionPending =
		sendMutation.isPending || interruptMutation.isPending;
	const isInputDisabled = !hasModelOptions;

	const handleSend = async (message: string) => {
		if (
			!message.trim() ||
			isSubmissionPending ||
			!agentId ||
			!hasModelOptions
		) {
			return;
		}
		const selectedModelConfigID =
			(selectedModel && modelConfigIDByModelID.get(selectedModel)) || undefined;
		const request: TypesGen.CreateChatMessageRequest = {
			content: [{ type: "text", text: message }],
			model_config_id: selectedModelConfigID,
		};
		clearChatErrorReason(agentId);
		clearStreamError();
		if (scrollContainerRef.current) {
			scrollContainerRef.current.scrollTop = 0;
		}
		await sendMutation.mutateAsync(request);
		if (typeof window !== "undefined") {
			if (selectedModelConfigID) {
				localStorage.setItem(
					lastModelConfigIDStorageKey,
					selectedModelConfigID,
				);
			} else {
				localStorage.removeItem(lastModelConfigIDStorageKey);
			}
		}
	};

	const handleInterrupt = () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		void interruptMutation.mutateAsync();
	};

	const streamTools = useMemo(
		() => buildStreamTools(streamState),
		[streamState],
	);
	const { hasMoreMessages, windowedMessages, loadMoreSentinelRef } =
		useMessageWindow({
			messages,
			resetKey: agentId,
		});

	const parsedMessages = useMemo(
		() => parseMessagesWithMergedTools(windowedMessages),
		[windowedMessages],
	);
	const subagentTitles = useMemo(
		() => buildSubagentTitles(parsedMessages),
		[parsedMessages],
	);
	const parsedSections = useMemo(
		() => buildParsedMessageSections(parsedMessages),
		[parsedMessages],
	);
	const persistedErrorReason = agentId ? chatErrorReasons[agentId] : undefined;
	const detailErrorMessage =
		(chatStatus === "error" ? persistedErrorReason : undefined) || streamError;
	const isAwaitingFirstStreamChunk =
		!streamState && (chatStatus === "running" || chatStatus === "pending");
	const hasStreamOutput = Boolean(streamState) || isAwaitingFirstStreamChunk;

	const chatTitle = chatQuery.data?.chat?.title;
	const parentChatID = getParentChatID(chatQuery.data?.chat);
	const parentChat = parentChatID
		? chatsQuery.data?.find((chat) => chat.id === parentChatID)
		: undefined;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const shouldShowDiffPanel = hasDiffStatus && showDiffPanel;

	const handleOpenInEditor = async (editor: "cursor" | "vscode") => {
		if (!workspace || !workspaceAgent) {
			return;
		}

		try {
			const { key } = await API.getApiKey();
			const vscodeHref = getVSCodeHref("vscode", {
				owner: workspace.owner_name,
				workspace: workspace.name,
				token: key,
				agent: workspaceAgent.name,
				folder: workspaceAgent.expanded_directory,
			});

			if (editor === "cursor") {
				const cursorApp = workspaceAgent.apps.find((app) => {
					const name = (app.display_name ?? app.slug).toLowerCase();
					return app.slug.toLowerCase() === "cursor" || name === "cursor";
				});
				if (cursorApp?.external && cursorApp.url) {
					const href = cursorApp.url.includes(SESSION_TOKEN_PLACEHOLDER)
						? cursorApp.url.replaceAll(SESSION_TOKEN_PLACEHOLDER, key)
						: cursorApp.url;
					window.location.assign(href);
					return;
				}
				window.location.assign(vscodeHref.replace(/^vscode:/, "cursor:"));
				return;
			}

			window.location.assign(vscodeHref);
		} catch {
			toast.error(
				editor === "cursor"
					? "Failed to open in Cursor."
					: "Failed to open in VS Code.",
			);
		}
	};

	const handleViewWorkspace = () => {
		if (!workspaceRoute) {
			return;
		}
		navigate(workspaceRoute);
	};

	const handleArchiveAgentAction = () => {
		if (!agentId) {
			return;
		}
		requestArchiveAgent(agentId);
	};

	if (chatQuery.isLoading) {
		return (
			<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
				<AgentDetailTopBar />
				<div className="mx-auto w-full max-w-3xl space-y-6 py-6">
					<div className="flex justify-end">
						<Skeleton className="h-10 w-2/3 rounded-xl" />
					</div>
					<div className="space-y-3">
						<Skeleton className="h-4 w-full" />
						<Skeleton className="h-4 w-5/6" />
						<Skeleton className="h-4 w-4/6" />
						<Skeleton className="h-4 w-full" />
						<Skeleton className="h-4 w-3/5" />
					</div>
				</div>
			</div>
		);
	}

	if (!chatQuery.data || !agentId) {
		return (
			<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
				<AgentDetailTopBar />
				<div className="flex flex-1 items-center justify-center text-content-secondary">
					Chat not found
				</div>
			</div>
		);
	}

	return (
		<div
			className={cn(
				"flex min-h-0 min-w-0 flex-1 bg-surface-primary",
				shouldShowDiffPanel && "flex-col xl:flex-row",
			)}
		>
			<div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface-primary">
				<AgentDetailTopBar>
					<div className="flex min-w-0 flex-1 items-center gap-1.5">
						{chatTitle && (
							<>
								{parentChat && (
									<>
										<Button
											size="sm"
											variant="subtle"
											className="h-auto max-w-[16rem] rounded-sm px-1 py-0.5 text-xs text-content-secondary shadow-none hover:bg-transparent hover:text-content-primary"
											onClick={() => navigate(`/agents/${parentChat.id}`)}
										>
											<span className="truncate">{parentChat.title}</span>
										</Button>
										<ChevronRightIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary/70" />
									</>
								)}
								<span className="truncate text-sm text-content-primary">
									{chatTitle}
								</span>
							</>
						)}
					</div>
					<div className="flex items-center gap-2">
						{hasDiffStatus && diffStatusQuery.data && (
							<DiffStatsBadge
								status={diffStatusQuery.data}
								isOpen={showDiffPanel}
								onToggle={() => setShowDiffPanel((prev) => !prev)}
							/>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									size="icon"
									variant="subtle"
									className="h-7 w-7 text-content-secondary hover:text-content-primary"
									aria-label="Open agent actions"
								>
									<EllipsisIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									disabled={!canOpenEditors}
									onSelect={() => {
										void handleOpenInEditor("cursor");
									}}
								>
									<ExternalLinkIcon className="h-3.5 w-3.5" />
									Open in Cursor
								</DropdownMenuItem>
								<DropdownMenuItem
									disabled={!canOpenEditors}
									onSelect={() => {
										void handleOpenInEditor("vscode");
									}}
								>
									<ExternalLinkIcon className="h-3.5 w-3.5" />
									Open in VS Code
								</DropdownMenuItem>
								<DropdownMenuItem
									disabled={!canOpenWorkspace}
									onSelect={handleViewWorkspace}
								>
									<MonitorIcon className="h-3.5 w-3.5" />
									View Workspace
								</DropdownMenuItem>
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onSelect={handleArchiveAgentAction}
								>
									<ArchiveIcon className="h-3.5 w-3.5" />
									Archive Agent
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				</AgentDetailTopBar>

				<div
					ref={scrollContainerRef}
					className="flex h-full flex-col-reverse overflow-y-auto [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
				>
					<div>
						<ConversationTimeline
							isEmpty={messages.length === 0}
							hasMoreMessages={hasMoreMessages}
							loadMoreSentinelRef={loadMoreSentinelRef}
							parsedSections={parsedSections}
							hasStreamOutput={hasStreamOutput}
							streamState={streamState}
							streamTools={streamTools}
							subagentTitles={subagentTitles}
							subagentStatusOverrides={subagentStatusOverrides}
							isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
							detailErrorMessage={detailErrorMessage}
						/>

						{queuedMessages.length > 0 && (
							<QueuedMessagesList
								messages={queuedMessages}
								onDelete={(id) => deleteQueuedMutation.mutate(id)}
								onPromote={(id) => promoteQueuedMutation.mutate(id)}
							/>
						)}
						<AgentChatInput
							onSend={handleSend}
							isDisabled={isInputDisabled}
							isLoading={sendMutation.isPending}
							isStreaming={isStreaming}
							onInterrupt={handleInterrupt}
							isInterruptPending={interruptMutation.isPending}
							contextUsage={latestContextUsage}
							hasModelOptions={hasModelOptions}
							selectedModel={selectedModel}
							onModelChange={setSelectedModel}
							modelOptions={modelOptions}
							modelSelectorPlaceholder={modelSelectorPlaceholder}
							inputStatusText={inputStatusText}
							modelCatalogStatusMessage={modelCatalogStatusMessage}
							sticky
						/>
					</div>
				</div>
			</div>

			{shouldShowDiffPanel && (
				<div
					data-testid="agents-detail-right-panel"
					className="min-h-0 min-w-0 border-t border-border-default bg-surface-primary h-[42dvh] min-h-[260px] max-h-[56dvh] xl:h-auto xl:max-h-none xl:w-[40%] xl:min-w-[360px] xl:max-w-[720px] xl:border-l xl:border-t-0"
				>
					<FilesChangedPanel chatId={agentId} />
				</div>
			)}
		</div>
	);
};

export default AgentDetail;
