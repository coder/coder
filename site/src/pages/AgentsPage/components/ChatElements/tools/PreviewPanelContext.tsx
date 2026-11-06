import {
	createContext,
	useContext,
	useEffect,
	useEffectEvent,
	useRef,
} from "react";
import type { ChatMessage, Workspace } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { savePersistedSelectedPreview } from "../../../utils/rightPanelTabStorage";
import {
	type ChatStore,
	selectMessagesByID,
	useChatSelector,
} from "../../ChatConversation/chatStore";
import { parseMessageContent } from "../../ChatConversation/messageParsing";
import {
	type PreviewIdentity,
	previewKey,
} from "../../RightPanel/PreviewFrame";
import { asRecord, asString } from "../runtimeTypeUtils";

/** The preview a successful `open_preview` tool result points at. */
type OpenPreviewTarget = {
	workspaceId: string;
	preview: PreviewIdentity;
};

/** Returns undefined when result doesn't name a workspace and agent. */
export const parseOpenPreviewTarget = (
	result: unknown,
): OpenPreviewTarget | undefined => {
	const rec = asRecord(result);
	if (
		typeof rec?.workspace_id !== "string" ||
		typeof rec.agent_name !== "string"
	) {
		return undefined;
	}

	return {
		workspaceId: rec.workspace_id,
		preview: {
			agent_name: rec.agent_name,
			port: typeof rec.port === "number" ? rec.port : undefined,
			path: typeof rec.path === "string" ? rec.path : undefined,
			url_path: typeof rec.url_path === "string" ? rec.url_path : undefined,
		},
	};
};

type PreviewPanelContextValue = {
	/** Selects the target in the Preview tab and shows the tab. */
	onOpenPreview?: (target: OpenPreviewTarget) => void;
};

export const PreviewPanelContext = createContext<PreviewPanelContextValue>({});

export const usePreviewPanel = () => useContext(PreviewPanelContext);

/**
 * Calls onNewPreview with the newest preview the agent publishes while the
 * chat is open. Messages loaded with the chat and older pages are skipped.
 */
export const useNewPreviews = ({
	store,
	initialMessages,
	onNewPreview,
}: {
	store: ChatStore;
	initialMessages: readonly ChatMessage[];
	onNewPreview?: (target: OpenPreviewTarget, title: string) => void;
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);

	// Messages can arrive out of ID order, so only a result newer than this ID
	// is new.
	const seenIDRef = useRef(initialMessages.at(-1)?.id ?? -1);
	const notify = useEffectEvent((target: OpenPreviewTarget, title: string) => {
		onNewPreview?.(target, title);
	});
	const canNotify = onNewPreview !== undefined;

	useEffect(() => {
		// Results that arrive before the chat can open previews, such as while
		// its workspace loads, are reported once it can.
		if (!canNotify) {
			return;
		}

		let latest:
			| { messageID: number; target: OpenPreviewTarget; title: string }
			| undefined;
		for (const message of messagesByID.values()) {
			if (message.id <= seenIDRef.current) {
				continue;
			}

			for (const result of parseMessageContent(message.content).toolResults) {
				const target =
					result.name === "open_preview"
						? parseOpenPreviewTarget(result.result)
						: undefined;
				if (target && (!latest || message.id >= latest.messageID)) {
					latest = {
						messageID: message.id,
						target,
						title: asString(asRecord(result.result)?.title),
					};
				}
			}
		}

		if (latest) {
			seenIDRef.current = latest.messageID;
			notify(latest.target, latest.title);
		}
	}, [messagesByID, canNotify]);
};

/**
 * Opens `open_preview` targets for the chat page from tool actions, and offers
 * new results in a toast. Returns the `PreviewPanelContext` value. Opening a
 * preview updates the key the page gives the Preview panel.
 */
export const useChatPreviewPanel = ({
	chatId,
	store,
	initialMessages,
	workspace,
	isPreviewTabSupported,
	isArchived,
	showSingletonTab,
	setPreviewPanelKey,
}: {
	chatId: string;
	store: ChatStore;
	initialMessages: readonly ChatMessage[];
	workspace: Workspace | undefined;
	isPreviewTabSupported: boolean;
	isArchived: boolean;
	showSingletonTab: (tabId: "preview") => void;
	setPreviewPanelKey: (update: (key: number) => number) => void;
}) => {
	const openPreview = (target: OpenPreviewTarget) => {
		// The panel only lists previews of the chat's current workspace.
		if (target.workspaceId !== workspace?.id) {
			return;
		}

		savePersistedSelectedPreview(chatId, previewKey(target.preview));
		setPreviewPanelKey((key) => key + 1);
		showSingletonTab("preview");
	};

	// Archived chats do not save the preview selection, which opening relies on.
	const onOpenPreview =
		isPreviewTabSupported && !isArchived ? openPreview : undefined;
	// The toast's action belongs to this chat and workspace, so it must not
	// outlive them.
	const toastId = `new-preview-${chatId}-${workspace?.id}`;
	useEffect(() => {
		return () => {
			toast.dismiss(toastId);
		};
	}, [toastId]);
	useNewPreviews({
		store,
		initialMessages,
		onNewPreview: onOpenPreview
			? (target, title) => {
					if (target.workspaceId === workspace?.id) {
						toast.info("Opened a preview", {
							id: toastId,
							description: title,
							action: {
								label: "Open preview",
								onClick: () => onOpenPreview(target),
							},
						});
					}
				}
			: undefined,
	});

	return { onOpenPreview };
};
