import { createContext } from "react";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import {
	isUserRightPanelTabAvailable,
	type NewFrameTab,
} from "../../../utils/rightPanelTabs";
import { asRecord } from "./utils";

type OpenPreviewTarget =
	| { kind: "preview"; slug: string }
	| { kind: "app"; slug: string };

type OpenPreviewResult = {
	title: string;
	workspaceId: string;
	agentName: string;
	target: OpenPreviewTarget;
	urlPath?: string;
};

const openPreviewKinds = ["port", "file", "app"] as const;
type OpenPreviewKind = (typeof openPreviewKinds)[number];

const isOpenPreviewKind = (value: unknown): value is OpenPreviewKind =>
	openPreviewKinds.some((kind) => kind === value);

// Control, format (including bidirectional overrides), and line or paragraph
// separators could disguise the title the user decides to open.
const isPreviewTitle = (value: unknown): value is string => {
	if (typeof value !== "string") {
		return false;
	}
	const runes = [...value].length;
	return (
		runes >= 1 && runes <= 80 && !/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(value)
	);
};

// Mirrors parseOpenPreviewArgs in coderd/x/chatd/chattool/openpreview.go,
// which limits the path to 2048 bytes.
const isPreviewURLPath = (value: unknown): value is string =>
	typeof value === "string" &&
	value.startsWith("/") &&
	!value.startsWith("//") &&
	new TextEncoder().encode(value).length <= 2048 &&
	!/[\\#\p{Cc}]/u.test(value);

const isPreviewSlug = (value: unknown): value is string =>
	typeof value === "string" &&
	/^preview--([0-9a-f]{8}|[1-9][0-9]{0,4})$/.test(value);

const parseTarget = (
	kind: OpenPreviewKind,
	rec: Record<string, unknown>,
): OpenPreviewTarget | undefined => {
	switch (kind) {
		case "port":
		case "file":
			return isPreviewSlug(rec.preview_slug)
				? { kind: "preview", slug: rec.preview_slug }
				: undefined;
		case "app":
			return typeof rec.app_slug === "string" && rec.app_slug !== ""
				? { kind: "app", slug: rec.app_slug }
				: undefined;
		default: {
			const _exhaustive: never = kind;
			return _exhaustive;
		}
	}
};

/**
 * Parses a stored `open_preview` tool result. Returns undefined for an error
 * result or one that fails validation.
 */
export const parseOpenPreviewResult = (
	result: unknown,
): OpenPreviewResult | undefined => {
	const rec = asRecord(result);
	if (
		!rec ||
		!isOpenPreviewKind(rec.kind) ||
		!isPreviewTitle(rec.title) ||
		typeof rec.workspace_id !== "string" ||
		typeof rec.agent_name !== "string" ||
		(rec.url_path !== undefined && !isPreviewURLPath(rec.url_path))
	) {
		return undefined;
	}
	const target = parseTarget(rec.kind, rec);
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

/**
 * Gives a row an open action only for a result from the chat's workspace and
 * agent whose tab the panel can show. Otherwise `getOpenAction` returns
 * undefined.
 */
export const getPreviewPanelContextValue = ({
	enabled,
	workspace,
	workspaceAgent,
	wildcardHostname,
	browserApp,
	openFrameTab,
	showBrowserTab,
	focusPanel,
}: {
	enabled: boolean;
	workspace: Workspace | undefined;
	workspaceAgent: WorkspaceAgent | undefined;
	wildcardHostname: string;
	browserApp: WorkspaceApp | undefined;
	openFrameTab: (tab: NewFrameTab) => void;
	showBrowserTab: () => void;
	focusPanel: () => void;
}): PreviewPanelContextValue => {
	if (!enabled) {
		return cannotOpen;
	}

	return {
		getOpenAction: (preview) => {
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
				return browserApp?.subdomain
					? () => {
							focusPanel();
							showBrowserTab();
						}
					: undefined;
			}

			const tab = getFrameTab(preview, workspaceAgent);
			return tab &&
				isUserRightPanelTabAvailable(tab, {
					workspace,
					workspaceAgent,
					wildcardHostname,
					openPreviewEnabled: enabled,
				})
				? () => {
						focusPanel();
						openFrameTab(tab);
					}
				: undefined;
		},
	};
};

const getFrameTab = (
	{ title, workspaceId, agentName, target, urlPath }: OpenPreviewResult,
	agent: WorkspaceAgent,
): NewFrameTab | undefined => {
	switch (target.kind) {
		case "preview":
			return {
				kind: "preview",
				label: title,
				workspaceId,
				agentName,
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
