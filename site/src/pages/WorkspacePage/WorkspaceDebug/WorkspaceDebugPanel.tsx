import { MessageScroller } from "@shadcn/react/message-scroller";
import { SendHorizonalIcon, SparklesIcon } from "lucide-react";
import { type FC, type KeyboardEvent, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { Link as RouterLink } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	chatMessagesForInfiniteScroll,
	chatModels,
	createChatMessage,
	openChat,
	workspaceDebugChat,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import { Loader } from "#/components/Loader/Loader";
import { Textarea } from "#/components/Textarea/Textarea";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import type { ChatDetailError } from "#/pages/AgentsPage/components/ChatConversation/chatError";
import { getPersistedDetailError } from "#/pages/AgentsPage/components/ChatConversation/chatError";
import {
	selectChatStatus,
	useChatSelector,
	useChatStore,
} from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import { ChatPageTimeline } from "#/pages/AgentsPage/components/ChatPageContent";
import type { WorkspaceFailure } from "./workspaceFailure";

type WorkspaceDebugPanelProps = {
	workspace: TypesGen.Workspace;
	failure: WorkspaceFailure;
};

/**
 * WorkspaceDebugPanel is the right-hand column shown when a workspace build
 * or agent fails. It opens (or reuses) a Coder Agents chat seeded server-side
 * with the failure context, then embeds the standard chat timeline and a
 * minimal composer with no model or workspace selection.
 */
export const WorkspaceDebugPanel: FC<WorkspaceDebugPanelProps> = ({
	workspace,
	failure,
}) => {
	const queryClient = useQueryClient();
	const aiGatewayEnabled = useAIGatewayEnabled();
	const debugChatQuery = useQuery({
		...workspaceDebugChat(queryClient, failure.buildId),
		enabled: aiGatewayEnabled,
	});

	return (
		<aside
			aria-label="AI workspace debugging"
			className="flex flex-col w-full min-w-0 min-h-0 h-full bg-surface-primary"
		>
			<header className="flex items-center gap-2 px-4 py-3 border-0 border-b border-solid border-border">
				<SparklesIcon className="size-icon-sm text-content-link" />
				<h2 className="m-0 text-sm font-medium">AI workspace debugging</h2>
				<Badge variant="warning" size="xs" className="ml-auto">
					Prototype
				</Badge>
			</header>

			<div className="px-4 py-2 text-xs text-content-secondary border-0 border-b border-solid border-border">
				{failure.kind === "build" ? "Build failed" : "Agent failed to start"}:{" "}
				<span className="text-content-primary break-words">
					{failure.error}
				</span>
			</div>

			{!aiGatewayEnabled && (
				<div className="p-4">
					<Alert severity="info">
						<AlertTitle>Coder Agents is disabled</AlertTitle>
						<AlertDescription>
							Enable the AI Gateway on this deployment to debug failed
							workspaces with an agent.
						</AlertDescription>
					</Alert>
				</div>
			)}
			{aiGatewayEnabled && debugChatQuery.isPending && (
				<div className="flex flex-col items-center gap-2 p-8 text-sm text-content-secondary">
					<Loader size="sm" />
					Collecting build logs and template context...
				</div>
			)}
			{aiGatewayEnabled && debugChatQuery.isError && (
				<div className="p-4 flex flex-col gap-3">
					<ErrorAlert error={debugChatQuery.error} showDebugDetail={false} />
					<Button
						variant="outline"
						size="sm"
						onClick={() => void debugChatQuery.refetch()}
					>
						Retry
					</Button>
				</div>
			)}
			{aiGatewayEnabled && debugChatQuery.data && (
				<MessageScroller.Provider
					key={debugChatQuery.data.chat.id}
					autoScroll
					defaultScrollPosition="end"
				>
					<WorkspaceDebugChat
						chatId={debugChatQuery.data.chat.id}
						organizationId={workspace.organization_id}
					/>
				</MessageScroller.Provider>
			)}
		</aside>
	);
};

type WorkspaceDebugChatProps = {
	chatId: string;
	organizationId: string;
};

const WorkspaceDebugChat: FC<WorkspaceDebugChatProps> = ({
	chatId,
	organizationId,
}) => {
	const queryClient = useQueryClient();
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const chatQuery = useQuery(openChat(chatId));
	const chatMessagesQuery = useInfiniteQuery(
		chatMessagesForInfiniteScroll(chatId),
	);
	const modelsQuery = useQuery(chatModels(organizationId));
	const [cachedError, setCachedError] = useState<ChatDetailError>();

	const chat = chatQuery.data;
	const pages = chatMessagesQuery.data?.pages;
	const chatMessagesList = (() => {
		if (!pages || pages.length === 0) return undefined;
		const byID = new Map(
			pages.flatMap((p) => p.messages).map((m) => [m.id, m] as const),
		);
		return Array.from(byID.values()).sort((a, b) => a.id - b.id);
	})();
	const chatQueuedMessages = pages?.[0]?.queued_messages;
	const chatMessagesData: TypesGen.ChatMessagesResponse | undefined =
		chatMessagesList
			? {
					messages: chatMessagesList,
					queued_messages: chatQueuedMessages ?? [],
					has_more: Boolean(pages?.at(-1)?.has_more),
				}
			: undefined;

	const { store, isHydratingMessages, upsertCacheMessages } = useChatStore({
		chatID: chatId,
		chatMessages: chatMessagesList,
		chatRecord: chat,
		chatRecordUpdatedAt: chatQuery.dataUpdatedAt,
		chatMessagesData,
		chatQueuedMessages,
		setChatErrorReason: (_id, reason) => setCachedError(reason),
		clearChatErrorReason: () => setCachedError(undefined),
		aiGatewayDisabled,
	});
	const liveChatStatus =
		useChatSelector(store, selectChatStatus) ?? chat?.status ?? null;
	const persistedError = getPersistedDetailError({
		chatStatus: liveChatStatus,
		chatRecord: chat,
		cachedError,
	});

	const { isPending: isSendPending, mutateAsync: sendMessage } = useMutation(
		createChatMessage(queryClient, chatId),
	);
	const [sendError, setSendError] = useState<string>();

	const handleSend = async (text: string) => {
		setSendError(undefined);
		try {
			const response = await sendMessage({
				content: [{ type: "text", text }],
			});
			const inserted =
				response.messages ?? (response.message ? [response.message] : []);
			if (inserted.length > 0) {
				upsertCacheMessages(inserted);
				store.upsertDurableMessages(inserted);
			}
			if (!response.queued) {
				store.clearStreamState();
				store.setChatStatus("running");
			}
		} catch (error) {
			setSendError(getErrorMessage(error, "Failed to send message."));
		}
	};

	const modelName = modelsQuery.data?.models.find(
		(m) => m.id === chat?.last_model_config_id,
	);
	const isBusy =
		liveChatStatus === "running" || liveChatStatus === "interrupting";

	return (
		<>
			<div className="flex flex-col flex-1 min-h-0 min-w-0 text-sm">
				<ChatPageTimeline
					organizationId={organizationId}
					store={store}
					chatFiles={chat?.files}
					persistedError={persistedError}
					hasMoreMessages={Boolean(chatMessagesQuery.hasNextPage)}
					isFetchingMoreMessages={chatMessagesQuery.isFetchingNextPage}
					isHydratingMessages={isHydratingMessages}
					hasFetchMoreError={chatMessagesQuery.isFetchNextPageError}
					onFetchMoreMessages={() => chatMessagesQuery.fetchNextPage()}
				/>
			</div>
			<footer className="flex flex-col gap-2 p-3 border-0 border-t border-solid border-border">
				{sendError && (
					<Alert severity="error">
						<AlertDescription>{sendError}</AlertDescription>
					</Alert>
				)}
				<DebugComposer
					disabled={isSendPending}
					onSend={handleSend}
					placeholder={
						isBusy
							? "Investigating... (your message will be queued)"
							: "Ask a follow-up question"
					}
				/>
				<div className="flex items-center justify-between text-xs text-content-secondary">
					<span>
						{modelName
							? `Model: ${modelName.display_name || modelName.model}`
							: "Default model"}
					</span>
					<Link asChild size="sm">
						<RouterLink to={`/agents/${chatId}`}>Open in Agents</RouterLink>
					</Link>
				</div>
			</footer>
		</>
	);
};

type DebugComposerProps = {
	disabled: boolean;
	placeholder: string;
	onSend: (text: string) => Promise<void>;
};

const DebugComposer: FC<DebugComposerProps> = ({
	disabled,
	placeholder,
	onSend,
}) => {
	const [value, setValue] = useState("");
	const trimmed = value.trim();

	const submit = async () => {
		if (!trimmed || disabled) {
			return;
		}
		setValue("");
		await onSend(trimmed);
	};

	const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
		if (event.key === "Enter" && !event.shiftKey) {
			event.preventDefault();
			void submit();
		}
	};

	return (
		<form
			className="flex items-end gap-2"
			onSubmit={(event) => {
				event.preventDefault();
				void submit();
			}}
		>
			<Textarea
				aria-label="Message"
				className="min-h-10 max-h-40 resize-none"
				rows={2}
				value={value}
				placeholder={placeholder}
				disabled={disabled}
				onChange={(event) => setValue(event.target.value)}
				onKeyDown={handleKeyDown}
			/>
			<Button
				type="submit"
				size="icon"
				aria-label="Send message"
				disabled={disabled || trimmed.length === 0}
			>
				<SendHorizonalIcon />
			</Button>
		</form>
	);
};
