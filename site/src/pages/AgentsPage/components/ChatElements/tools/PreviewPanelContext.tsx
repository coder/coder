import {
	createContext,
	useContext,
	useEffect,
	useEffectEvent,
	useRef,
} from "react";
import type {
	ChatMessage,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import {
	AGENT_BROWSER_APP_SLUG,
	getAgentBrowserApp,
	isWorkspaceAppEmbeddable,
} from "#/modules/apps/apps";
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

type OpenPreviewTarget =
	| {
			workspaceId: string;
			preview: PreviewIdentity;
	  }
	| {
			workspaceId: string;
			app: { agent_name: string; slug: string };
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

	if (typeof rec.app_slug === "string") {
		return {
			workspaceId: rec.workspace_id,
			app: { agent_name: rec.agent_name, slug: rec.app_slug },
		};
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
	/**
	 * Returns an action that shows the target in the Preview tab or in its
	 * app's tab, or undefined when the chat cannot show it.
	 */
	getOpenAction?: (target: OpenPreviewTarget) => (() => void) | undefined;
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
				// App results get no toast: an app can serve code the agent controls,
				// outside the restrictions on preview pages.
				if (
					target &&
					!("app" in target) &&
					(!latest || message.id >= latest.messageID)
				) {
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
 * new port and file previews in a toast. Returns the `PreviewPanelContext`
 * value. Opening a preview updates the key the page gives the Preview panel.
 */
export const useChatPreviewPanel = ({
	chatId,
	store,
	initialMessages,
	workspace,
	workspaceAgent,
	isPreviewTabSupported,
	isArchived,
	showSingletonTab,
	openWorkspaceApp,
	setPreviewPanelKey,
}: {
	chatId: string;
	store: ChatStore;
	initialMessages: readonly ChatMessage[];
	workspace: Workspace | undefined;
	workspaceAgent: WorkspaceAgent | undefined;
	isPreviewTabSupported: boolean;
	isArchived: boolean;
	showSingletonTab: (tabId: "browser" | "preview") => void;
	openWorkspaceApp: (app: WorkspaceApp) => void;
	setPreviewPanelKey: (update: (key: number) => number) => void;
}) => {
	const getOpenAction = (
		target: OpenPreviewTarget,
	): (() => void) | undefined => {
		// Results from a workspace the chat no longer uses cannot be shown.
		if (target.workspaceId !== workspace?.id) {
			return undefined;
		}

		if (!("app" in target)) {
			return () => {
				savePersistedSelectedPreview(chatId, previewKey(target.preview));
				setPreviewPanelKey((key) => key + 1);
				showSingletonTab("preview");
			};
		}

		// A stopped workspace has no agents.
		if (workspaceAgent?.name !== target.app.agent_name) {
			return undefined;
		}

		// agent-browser has the built-in Browser tab instead of an app tab.
		if (target.app.slug === AGENT_BROWSER_APP_SLUG) {
			return getAgentBrowserApp(workspaceAgent)?.subdomain
				? () => showSingletonTab("browser")
				: undefined;
		}

		const app = workspaceAgent.apps.find((app) => app.slug === target.app.slug);
		// Recheck because the app can change after the tool ran, such as on a
		// rebuild. Path-based apps run on the dashboard's origin.
		return app?.subdomain && isWorkspaceAppEmbeddable(app)
			? () => openWorkspaceApp(app)
			: undefined;
	};

	// Archived chats are read-only and do not save the preview selection that
	// opening a preview relies on.
	const canOpenResults = isPreviewTabSupported && !isArchived;
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
		onNewPreview: canOpenResults
			? (target, title) => {
					const open = getOpenAction(target);
					if (open) {
						toast.info("Opened a preview", {
							id: toastId,
							description: title,
							action: { label: "Open preview", onClick: open },
						});
					}
				}
			: undefined,
	});

	return { getOpenAction: canOpenResults ? getOpenAction : undefined };
};
