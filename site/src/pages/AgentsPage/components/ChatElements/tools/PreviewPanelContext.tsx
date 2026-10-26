import {
	createContext,
	useContext,
	useEffect,
	useEffectEvent,
	useRef,
} from "react";
import type { ChatMessage, ChatPreview } from "#/api/typesGenerated";
import {
	type ChatStore,
	selectMessagesByID,
	useChatSelector,
} from "../../ChatConversation/chatStore";
import { parseMessageContent } from "../../ChatConversation/messageParsing";
import { asRecord } from "../runtimeTypeUtils";

type PreviewIdentity = Pick<
	ChatPreview,
	"agent_name" | "port" | "preview_id" | "url_path"
>;

/** The preview a successful `open_preview` result points at. */
export type OpenPreviewTarget = {
	workspaceId: string;
	preview: PreviewIdentity;
};

/**
 * Key for a preview's Select value and saved selection. Port previews have no
 * ID, so the port and URL path stand in.
 */
export const previewKey = (preview: PreviewIdentity): string =>
	`${preview.agent_name}:${preview.preview_id || preview.port}${preview.url_path ?? ""}`;

/**
 * Reads the target of a built-in `open_preview` result. An MCP tool can share
 * the name but not the result contract.
 */
export const parseOpenPreviewTarget = ({
	result,
	isError,
	mcpServerConfigId,
}: {
	result?: unknown;
	isError: boolean;
	mcpServerConfigId?: string;
}): OpenPreviewTarget | undefined => {
	if (isError || mcpServerConfigId) {
		return undefined;
	}

	const rec = asRecord(result);
	if (
		rec?.ok !== true ||
		typeof rec.workspace_id !== "string" ||
		typeof rec.agent_name !== "string"
	) {
		return undefined;
	}

	const port = typeof rec.port === "number" ? rec.port : undefined;
	const previewID =
		typeof rec.preview_id === "string" ? rec.preview_id : undefined;
	if (port === undefined && previewID === undefined) {
		return undefined;
	}

	const urlPath = typeof rec.url_path === "string" ? rec.url_path : undefined;

	return {
		workspaceId: rec.workspace_id,
		preview: {
			agent_name: rec.agent_name,
			port,
			preview_id: previewID,
			url_path: urlPath,
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
 * Opens the latest preview the agent publishes while the chat is open.
 * Messages that were already loaded, older pages and repeated deliveries are
 * skipped.
 */
export const useOpenNewPreviews = ({
	store,
	chatID,
	initialMessages,
	onOpenPreview,
}: {
	store: ChatStore;
	chatID: string;
	initialMessages: readonly ChatMessage[];
	onOpenPreview?: (target: OpenPreviewTarget) => void;
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);

	// Messages loaded with the chat are history, and older pages load below
	// them. Newer messages can arrive out of ID order, so only a preview
	// newer than the last one opened is opened.
	const openedIDRef = useRef(initialMessages.at(-1)?.id ?? -1);
	const openPreview = useEffectEvent((target: OpenPreviewTarget) => {
		onOpenPreview?.(target);
	});

	useEffect(() => {
		// Each open selects its tab, so only the newest target is opened.
		let latest: { messageID: number; target: OpenPreviewTarget } | undefined;
		for (const message of messagesByID.values()) {
			// The store can briefly hold the previous chat's messages.
			if (message.chat_id !== chatID || message.id <= openedIDRef.current) {
				continue;
			}

			for (const result of parseMessageContent(message.content).toolResults) {
				const target =
					result.name === "open_preview"
						? parseOpenPreviewTarget(result)
						: undefined;
				if (target && (!latest || message.id >= latest.messageID)) {
					latest = { messageID: message.id, target };
				}
			}
		}

		if (latest) {
			openedIDRef.current = latest.messageID;
			openPreview(latest.target);
		}
	}, [chatID, messagesByID]);
};
