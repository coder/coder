import { createContext, useEffect, useEffectEvent, useRef } from "react";
import type {
	ChatMessage,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { isFilePreviewID, isValidPort } from "#/utils/portForward";
import {
	isUserRightPanelTabAvailable,
	type NewFrameTab,
} from "../../../utils/rightPanelTabs";
import type {
	ChatStore,
	ChatStoreState,
} from "../../ChatConversation/chatStore";
import { asRecord } from "./utils";

/** What an `open_preview` result shows: exactly one of its target fields. */
type OpenPreviewTarget =
	| { kind: "port"; port: number }
	| { kind: "file"; previewId: string }
	| { kind: "app"; slug: string };

/** An `open_preview` tool result that `parseOpenPreviewResult` validated. */
type OpenPreviewResult = {
	title: string;
	workspaceId: string;
	agentName: string;
	target: OpenPreviewTarget;
	/** Path and query to open at. Undefined opens the root. */
	urlPath?: string;
};

const parseTarget = (
	rec: Record<string, unknown>,
): OpenPreviewTarget | undefined => {
	const targets: OpenPreviewTarget[] = [];
	if (rec.port !== undefined) {
		if (!isValidPort(rec.port)) {
			return undefined;
		}
		targets.push({ kind: "port", port: rec.port });
	}
	if (rec.preview_id !== undefined) {
		if (!isFilePreviewID(rec.preview_id)) {
			return undefined;
		}
		targets.push({ kind: "file", previewId: rec.preview_id });
	}
	if (rec.app_slug !== undefined) {
		if (typeof rec.app_slug !== "string" || rec.app_slug === "") {
			return undefined;
		}
		targets.push({ kind: "app", slug: rec.app_slug });
	}
	return targets.length === 1 ? targets[0] : undefined;
};

/**
 * Parses a stored `open_preview` tool result. Returns undefined for an error
 * result or one that does not name exactly one target.
 */
export const parseOpenPreviewResult = (
	result: unknown,
): OpenPreviewResult | undefined => {
	const rec = asRecord(result);
	if (
		!rec ||
		typeof rec.title !== "string" ||
		typeof rec.workspace_id !== "string" ||
		typeof rec.agent_name !== "string" ||
		(rec.url_path !== undefined && typeof rec.url_path !== "string")
	) {
		return undefined;
	}
	const target = parseTarget(rec);
	if (!target) {
		return undefined;
	}
	return {
		title: rec.title,
		workspaceId: rec.workspace_id,
		agentName: rec.agent_name,
		target,
		// "/" opens the root, so it shares the tab of a result without a path.
		urlPath:
			rec.url_path === undefined || rec.url_path === "/"
				? undefined
				: rec.url_path,
	};
};

type PreviewPanelContextValue = {
	/**
	 * Returns an action that opens the preview's right panel tab, or
	 * undefined when the chat cannot show it, such as outside a chat or
	 * without the open-preview experiment.
	 */
	getOpenAction: (preview: OpenPreviewResult) => (() => void) | undefined;
};

const cannotOpen: PreviewPanelContextValue = {
	getOpenAction: () => undefined,
};

/** Lets `open_preview` rows open their preview in the chat's right panel. */
export const PreviewPanelContext =
	createContext<PreviewPanelContextValue>(cannotOpen);

const findNewestPreview = (
	messages: Iterable<ChatMessage>,
	afterID: number,
): { messageID: number; preview: OpenPreviewResult } | undefined => {
	let newest: { messageID: number; preview: OpenPreviewResult } | undefined;
	for (const message of messages) {
		if (message.id <= afterID) {
			continue;
		}
		for (const part of message.content ?? []) {
			const preview =
				part.type === "tool-result" &&
				part.tool_name === "open_preview" &&
				!part.is_error
					? parseOpenPreviewResult(part.result)
					: undefined;
			if (preview && (!newest || message.id >= newest.messageID)) {
				newest = { messageID: message.id, preview };
			}
		}
	}
	return newest;
};

/**
 * Opens `open_preview` results in the chat's right panel, from a row's
 * action or on their own when they arrive while the chat is open.
 */
export const useChatPreviewPanel = ({
	enabled,
	store,
	workspace,
	workspaceAgent,
	wildcardHostname,
	browserApp,
	openFrameTab,
	showBrowserTab,
	focusPanel,
}: {
	enabled: boolean;
	store: ChatStore;
	workspace: Workspace | undefined;
	workspaceAgent: WorkspaceAgent | undefined;
	wildcardHostname: string;
	browserApp: WorkspaceApp | undefined;
	openFrameTab: (tab: NewFrameTab) => void;
	showBrowserTab: () => void;
	focusPanel: () => void;
}): PreviewPanelContextValue => {
	const getOpenWithoutFocus = (
		preview: OpenPreviewResult,
	): (() => void) | undefined => {
		// Agent IDs change when the workspace rebuilds; the name does not.
		if (
			!workspace ||
			!workspaceAgent ||
			preview.workspaceId !== workspace.id ||
			preview.agentName !== workspaceAgent.name
		) {
			return undefined;
		}

		const { target } = preview;
		if (target.kind === "app" && target.slug === AGENT_BROWSER_APP_SLUG) {
			return browserApp?.subdomain ? showBrowserTab : undefined;
		}

		const tab = getFrameTab(preview, workspaceAgent);
		return tab &&
			isUserRightPanelTabAvailable(tab, {
				workspace,
				workspaceAgent,
				wildcardHostname,
			})
			? () => openFrameTab(tab)
			: undefined;
	};

	// Every preview needs the wildcard host, which is empty until the proxies
	// load. A result that arrives before then opens once it can.
	const canAutoOpen =
		enabled &&
		workspace !== undefined &&
		workspaceAgent !== undefined &&
		wildcardHostname.trim() !== "";
	// Results in the store at mount never open on their own. The store's own
	// IDs leave out pending sends, whose IDs sort above every message ID.
	const seenIDRef = useRef(store.getSnapshot().orderedMessageIDs.at(-1) ?? -1);
	const autoOpen = useEffectEvent((preview: OpenPreviewResult) => {
		// Below lg the panel would replace the chat, so the user opens the
		// preview from its row instead.
		if (!window.matchMedia(belowLgViewportMediaQuery).matches) {
			getOpenWithoutFocus(preview)?.();
		}
	});

	useEffect(() => {
		if (!canAutoOpen) {
			return;
		}

		let checkedMessages: ChatStoreState["messagesByID"] | undefined;
		const check = () => {
			const { messagesByID } = store.getSnapshot();
			if (messagesByID === checkedMessages) {
				return;
			}
			checkedMessages = messagesByID;

			const newest = findNewestPreview(
				messagesByID.values(),
				seenIDRef.current,
			);
			if (newest) {
				seenIDRef.current = newest.messageID;
				autoOpen(newest.preview);
			}
		};

		check();
		return store.subscribe(check);
	}, [store, canAutoOpen]);

	if (!enabled) {
		return cannotOpen;
	}
	return {
		getOpenAction: (preview) => {
			const open = getOpenWithoutFocus(preview);
			// Below lg the panel hides the chat and the row's button with it,
			// so the panel takes focus.
			return (
				open &&
				(() => {
					focusPanel();
					open();
				})
			);
		},
	};
};

const getFrameTab = (
	{ title, target, urlPath }: OpenPreviewResult,
	agent: WorkspaceAgent,
): NewFrameTab | undefined => {
	switch (target.kind) {
		case "port":
			// The tool only accepts plain HTTP servers.
			return {
				kind: "port",
				label: title,
				agentId: agent.id,
				port: target.port,
				protocol: "http",
				urlPath,
			};
		case "file":
			return {
				kind: "file_preview",
				label: title,
				agentId: agent.id,
				previewId: target.previewId,
				urlPath,
			};
		case "app": {
			const app = agent.apps.find((app) => app.slug === target.slug);
			// A path-based app shares the dashboard's origin, so the tool only
			// opens subdomain apps. The app can change after the tool ran, such
			// as on a rebuild.
			return app?.subdomain
				? {
						kind: "workspace_app",
						label: app.display_name ?? app.slug,
						agentId: agent.id,
						appId: app.id,
						urlPath,
					}
				: undefined;
		}
		default: {
			const _exhaustive: never = target;
			return _exhaustive;
		}
	}
};
