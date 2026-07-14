import {
	createContext,
	type FC,
	type PropsWithChildren,
	useCallback,
	useContext,
	useEffect,
	useState,
} from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useLocation } from "react-router";
import { isApiError } from "#/api/errors";
import {
	chat,
	chatMessagesForInfiniteScroll,
	chatModelConfigs,
	chatModels,
	createChat,
	createChatMessage,
	interruptChat,
	updateChatLabels,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	type ChatStore,
	selectChatStatus,
	selectHasStreamState,
	useChatSelector,
} from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import { useChatStore } from "#/pages/AgentsPage/components/ChatConversation/useChatStore";
import type { ModelSelectorOption } from "#/pages/AgentsPage/components/ChatElements";
import {
	getModelSelectorPlaceholder,
	resolveModelOptionId,
	resolveModelSelector,
} from "#/pages/AgentsPage/utils/modelOptions";
import type { ChatDetailError } from "#/pages/AgentsPage/utils/usageLimitMessage";
import { isCoderAssistantHidden, setCoderAssistantHidden } from "./visibility";

interface CoderAssistantContextValue {
	enabled: boolean;
	open: boolean;
	toggle: () => void;
	close: () => void;
	// Disables the assistant immediately and persists the choice.
	disable: () => void;
	chatId: string | null;
	chatTitle: string | undefined;
	store: ChatStore;
	persistedError: ChatDetailError | undefined;
	sendMessage: (text: string) => void;
	startNewChat: () => void;
	isThinking: boolean;
	// True while a create-chat or send-message request is in flight.
	isSendPending: boolean;
	// True while the assistant is streaming a response, mirroring the
	// derivation in ChatPageContent.
	isStreaming: boolean;
	interrupt: () => void;
	isInterruptPending: boolean;
	// Model selector state, mirroring the agents chat page wiring.
	modelOptions: readonly ModelSelectorOption[];
	selectedModel: string;
	setSelectedModel: (id: string) => void;
	hasModelOptions: boolean;
	modelSelectorPlaceholder: string;
	isModelCatalogLoading: boolean;
}

const CoderAssistantContext = createContext<CoderAssistantContextValue | null>(
	null,
);

const CHAT_ID_STORAGE_KEY = "coder_assistant_chat_id";

// Label key the server reads to learn which dashboard page the user is
// viewing. The value is the raw pathname; label values allow "/" and
// are capped at 256 bytes server-side.
const PAGE_LABEL_KEY = "coder-assistant-page";
const MAX_PAGE_LABEL_LENGTH = 256;

function pageLabelValue(pathname: string): string | undefined {
	if (!pathname || pathname.length > MAX_PAGE_LABEL_LENGTH) {
		return undefined;
	}
	return pathname;
}

// Same key the agents chat page uses, so the panel and the full
// page share the user's last model choice.
const LAST_MODEL_CONFIG_ID_STORAGE_KEY = "agents.last-model-config-id";

// Key used to store an error from a failed chat creation, before any
// chat ID exists.
const PENDING_CHAT_ERROR_KEY = "pending";

function readLocalStorage(key: string, fallback: string): string {
	try {
		return localStorage.getItem(key) ?? fallback;
	} catch {
		return fallback;
	}
}

function writeLocalStorage(key: string, value: string | null): void {
	try {
		if (value === null) {
			localStorage.removeItem(key);
		} else {
			localStorage.setItem(key, value);
		}
	} catch {
		// Storage may be unavailable in some contexts.
	}
}

export const CoderAssistantProvider: FC<
	PropsWithChildren<{ forceEnabled?: boolean }>
> = ({ children, forceEnabled }) => {
	const [enabled, setEnabled] = useState(
		() => forceEnabled || !isCoderAssistantHidden(),
	);
	const [open, setOpen] = useState(false);
	const [chatId, setChatIdState] = useState<string | null>(
		() => readLocalStorage(CHAT_ID_STORAGE_KEY, "") || null,
	);

	const queryClient = useQueryClient();
	const { user } = useAuthenticated();
	const organizationId = user.organization_ids[0];

	// When the assistant is disabled, act as if no chat exists so no
	// queries, mutations, or WebSocket connections fire.
	const effectiveChatId = enabled ? chatId : null;

	const setChatId = useCallback((id: string | null) => {
		setChatIdState(id);
		writeLocalStorage(CHAT_ID_STORAGE_KEY, id);
	}, []);

	// Error reasons keyed by chat ID, matching the callback contract
	// that useChatStore expects.
	const [errorReasons, setErrorReasons] = useState<
		Record<string, ChatDetailError>
	>({});
	const setChatErrorReason = useCallback(
		(chatID: string, reason: ChatDetailError) => {
			setErrorReasons((prev) => ({ ...prev, [chatID]: reason }));
		},
		[],
	);
	const clearChatErrorReason = useCallback((chatID: string) => {
		setErrorReasons((prev) => {
			if (!(chatID in prev)) {
				return prev;
			}
			const next = { ...prev };
			delete next[chatID];
			return next;
		});
	}, []);
	const persistedError =
		errorReasons[effectiveChatId ?? PENDING_CHAT_ERROR_KEY];

	const chatQuery = useQuery({
		...chat(effectiveChatId ?? ""),
		enabled: enabled && Boolean(effectiveChatId),
		retry: false,
	});
	const chatMessagesQuery = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(effectiveChatId ?? ""),
		enabled: enabled && Boolean(effectiveChatId),
	});

	// The stored chat may have been deleted since the last visit.
	// Drop the stale ID only when the server says the chat is gone
	// (404); transient errors are left for react-query to retry.
	const chatLoadError = chatQuery.error ?? chatMessagesQuery.error;
	useEffect(() => {
		if (
			chatId &&
			isApiError(chatLoadError) &&
			chatLoadError.response.status === 404
		) {
			setChatId(null);
		}
	}, [chatId, chatLoadError, setChatId]);

	// Flatten the infinite pages into a single chronological list,
	// deduplicated by ID. Mirrors the wiring in AgentChatPage.
	const chatMessagesList: TypesGen.ChatMessage[] | undefined = (() => {
		const pages = chatMessagesQuery.data?.pages;
		if (!pages) {
			return undefined;
		}
		const all = pages.flatMap((p) => p.messages);
		const byID = new Map(all.map((m) => [m.id, m]));
		const deduped = Array.from(byID.values());
		deduped.sort((a, b) => a.id - b.id);
		return deduped;
	})();

	// Queued messages are only in the first page (most recent).
	const chatQueuedMessages = chatMessagesQuery.data?.pages[0]?.queued_messages;

	// Synthetic ChatMessagesResponse for backward compat with
	// useChatStore, matching the shape built in AgentChatPage.
	const chatMessagesData: TypesGen.ChatMessagesResponse | undefined =
		chatMessagesList
			? {
					messages: chatMessagesList,
					queued_messages: chatQueuedMessages ?? [],
					has_more: chatMessagesQuery.data?.pages.at(-1)?.has_more ?? false,
				}
			: undefined;

	const { store } = useChatStore({
		chatID: effectiveChatId ?? undefined,
		chatMessages: chatMessagesList,
		chatRecord: chatQuery.data,
		chatMessagesData,
		chatQueuedMessages,
		setChatErrorReason,
		clearChatErrorReason,
	});

	const { isPending: isCreatePending, mutateAsync: createChatAsync } =
		useMutation(createChat(queryClient));
	const { isPending: isSendPending, mutateAsync: createMessageAsync } =
		useMutation(createChatMessage(queryClient, effectiveChatId ?? ""));
	const { isPending: isInterruptPending, mutateAsync: interruptAsync } =
		useMutation(interruptChat(queryClient, effectiveChatId ?? ""));
	const { mutate: updateLabels } = useMutation(updateChatLabels(queryClient));

	// Keep the chat's page label in sync with the current route so the
	// assistant knows where the user is in the dashboard. Debounced so
	// rapid navigation doesn't spam the PATCH endpoint. Labels are
	// replaced wholesale by the endpoint, so the existing set is merged.
	const { pathname } = useLocation();
	const chatLabels = chatQuery.data?.labels;
	useEffect(() => {
		if (!enabled || !effectiveChatId || !chatLabels) {
			return;
		}
		const page = pageLabelValue(pathname);
		if (!page || chatLabels[PAGE_LABEL_KEY] === page) {
			return;
		}
		const timeout = window.setTimeout(() => {
			updateLabels({
				chatId: effectiveChatId,
				labels: { ...chatLabels, [PAGE_LABEL_KEY]: page },
			});
		}, 1000);
		return () => window.clearTimeout(timeout);
	}, [enabled, effectiveChatId, chatLabels, pathname, updateLabels]);

	// Model selector, wired the same way as the agents chat page.
	const chatModelsQuery = useQuery({ ...chatModels(), enabled });
	const chatModelConfigsQuery = useQuery({ ...chatModelConfigs(), enabled });
	const userProviderConfigsQuery = useQuery({
		...userChatProviderConfigs(),
		enabled,
	});
	const {
		options: modelOptions,
		isModelCatalogLoading,
		modelCatalog,
		hasConfiguredModels,
	} = resolveModelSelector(
		chatModelConfigsQuery,
		chatModelsQuery,
		userProviderConfigsQuery,
	);
	const [selectedModel, setSelectedModel] = useState(() =>
		readLocalStorage(LAST_MODEL_CONFIG_ID_STORAGE_KEY, ""),
	);
	// Validate the user's choice against current options, falling back
	// to the chat's last model or the first available option.
	const effectiveSelectedModel = (() => {
		const resolvedSelectedModel = resolveModelOptionId(
			selectedModel,
			modelOptions,
		);
		if (resolvedSelectedModel) {
			return resolvedSelectedModel;
		}
		const resolvedChatModel = resolveModelOptionId(
			chatQuery.data?.last_model_config_id,
			modelOptions,
		);
		if (resolvedChatModel) {
			return resolvedChatModel;
		}
		return modelOptions[0]?.id ?? "";
	})();
	const hasModelOptions = modelOptions.length > 0;
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		isModelCatalogLoading,
		hasConfiguredModels,
		modelCatalog,
	);

	// The store's status is hydrated from REST and kept fresh by the
	// WebSocket, so it is the authoritative source for the thinking
	// indicator.
	const chatStatus = useChatSelector(store, selectChatStatus);
	const hasStreamState = useChatSelector(store, selectHasStreamState);
	const isThinking =
		isCreatePending ||
		isSendPending ||
		chatStatus === "running" ||
		chatStatus === "pending";
	// Same derivation ChatPageContent uses for the stop button.
	const isStreaming =
		hasStreamState || chatStatus === "running" || chatStatus === "pending";

	const toggle = useCallback(() => {
		setOpen((prev) => !prev);
	}, []);

	const close = useCallback(() => {
		setOpen(false);
	}, []);

	const disable = useCallback(() => {
		setCoderAssistantHidden(true);
		setEnabled(false);
		setOpen(false);
		// Clear the stored chat so re-enabling starts clean and no
		// orphaned queries fire against a stale chat ID.
		setChatId(null);
	}, [setChatId]);

	const interrupt = useCallback(() => {
		if (!effectiveChatId || isInterruptPending) {
			return;
		}
		void interruptAsync();
	}, [effectiveChatId, isInterruptPending, interruptAsync]);

	const sendMessage = useCallback(
		(text: string) => {
			// Ignore sends while a create or send is already in flight to
			// avoid duplicate chats or messages from rapid submissions.
			if (isCreatePending || isSendPending) {
				return;
			}
			const content: TypesGen.ChatInputPart[] = [{ type: "text", text }];
			const modelConfigId = effectiveSelectedModel || undefined;
			void (async () => {
				try {
					if (effectiveChatId) {
						await createMessageAsync({
							content,
							model_config_id: modelConfigId,
						});
					} else {
						const page = pageLabelValue(window.location.pathname);
						const created = await createChatAsync({
							organization_id: organizationId,
							content,
							model_config_id: modelConfigId,
							labels: {
								"coder-assistant": "true",
								...(page ? { [PAGE_LABEL_KEY]: page } : {}),
							},
							client_type: "ui",
						});
						clearChatErrorReason(PENDING_CHAT_ERROR_KEY);
						setChatId(created.id);
					}
					if (modelConfigId) {
						writeLocalStorage(LAST_MODEL_CONFIG_ID_STORAGE_KEY, modelConfigId);
					}
				} catch (error) {
					const target = effectiveChatId ?? PENDING_CHAT_ERROR_KEY;
					setChatErrorReason(target, {
						kind: "generic",
						message:
							error instanceof Error
								? error.message
								: "Failed to send message.",
					});
				}
			})();
		},
		[
			effectiveChatId,
			clearChatErrorReason,
			createChatAsync,
			createMessageAsync,
			effectiveSelectedModel,
			isCreatePending,
			isSendPending,
			organizationId,
			setChatErrorReason,
			setChatId,
		],
	);

	const startNewChat = useCallback(() => {
		// Stop any in-flight generation before abandoning the chat so
		// the server doesn't keep streaming into an orphaned chat.
		if (isStreaming) {
			interrupt();
		}
		clearChatErrorReason(PENDING_CHAT_ERROR_KEY);
		setChatId(null);
	}, [clearChatErrorReason, interrupt, isStreaming, setChatId]);

	return (
		<CoderAssistantContext.Provider
			value={{
				enabled,
				open,
				toggle,
				close,
				disable,
				chatId: effectiveChatId,
				chatTitle: chatQuery.data?.title,
				store,
				persistedError,
				sendMessage,
				startNewChat,
				isThinking,
				isSendPending: isCreatePending || isSendPending,
				isStreaming,
				interrupt,
				isInterruptPending,
				modelOptions,
				selectedModel: effectiveSelectedModel,
				setSelectedModel,
				hasModelOptions,
				modelSelectorPlaceholder,
				isModelCatalogLoading,
			}}
		>
			{children}
		</CoderAssistantContext.Provider>
	);
};

export function useCoderAssistantContext(): CoderAssistantContextValue {
	const ctx = useContext(CoderAssistantContext);
	if (!ctx) {
		throw new Error(
			"useCoderAssistantContext must be used within a CoderAssistantProvider",
		);
	}
	return ctx;
}
