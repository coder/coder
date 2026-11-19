import { createContext, useEffect, useEffectEvent, useRef } from "react";
import type {
	ChatMessage,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { isHostnameLabel, isValidPort } from "#/utils/portForward";
import {
	isUserRightPanelTabAvailable,
	type NewFrameTab,
} from "../../../utils/rightPanelTabs";
import type {
	ChatStore,
	ChatStoreState,
} from "../../ChatConversation/chatStore";
import { asRecord } from "./utils";

type OpenPreviewTarget =
	| { kind: "port"; port: number }
	| { kind: "file"; slug: string }
	| { kind: "app"; slug: string };

type OpenPreviewResult = {
	title: string;
	workspaceId: string;
	agentName: string;
	target: OpenPreviewTarget;
	urlPath?: string;
};

const parseTarget = (
	rec: Record<string, unknown>,
): OpenPreviewTarget | undefined => {
	switch (rec.kind) {
		case "port":
			return isValidPort(rec.port)
				? { kind: "port", port: rec.port }
				: undefined;
		case "file":
			return isHostnameLabel(rec.preview_slug)
				? { kind: "file", slug: rec.preview_slug }
				: undefined;
		case "app":
			return typeof rec.app_slug === "string" && rec.app_slug !== ""
				? { kind: "app", slug: rec.app_slug }
				: undefined;
		default:
			return undefined;
	}
};

/**
 * Parses a stored `open_preview` tool result. Returns undefined for an error
 * result or one whose target is missing or of an unknown kind.
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
		urlPath: rec.url_path,
	};
};

type PreviewPanelContextValue = {
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

	const canAutoOpen =
		enabled &&
		workspace !== undefined &&
		workspaceAgent !== undefined &&
		wildcardHostname.trim() !== "";
	const seenIDRef = useRef(store.getSnapshot().orderedMessageIDs.at(-1) ?? -1);
	const autoOpen = useEffectEvent((preview: OpenPreviewResult) => {
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
				slug: target.slug,
				urlPath,
			};
		case "app": {
			const app = agent.apps.find((app) => app.slug === target.slug);
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
