import { createContext } from "react";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import type { NewFrameTab } from "../../../utils/rightPanelTabs";
import { asRecord } from "./utils";

type OpenPreviewResult = {
	title: string;
	workspaceId: string;
	agentName: string;
	slug: string;
	urlPath?: string;
};

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
		(rec.kind !== "port" && rec.kind !== "file") ||
		!isPreviewTitle(rec.title) ||
		typeof rec.workspace_id !== "string" ||
		typeof rec.agent_name !== "string" ||
		!isPreviewSlug(rec.preview_slug) ||
		(rec.url_path !== undefined && !isPreviewURLPath(rec.url_path))
	) {
		return undefined;
	}
	return {
		title: rec.title,
		workspaceId: rec.workspace_id,
		agentName: rec.agent_name,
		slug: rec.preview_slug,
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
 * agent. Otherwise `getOpenAction` returns undefined.
 */
export const getPreviewPanelContextValue = ({
	enabled,
	workspace,
	workspaceAgent,
	openFrameTab,
	focusPanel,
}: {
	enabled: boolean;
	workspace: Workspace | undefined;
	workspaceAgent: WorkspaceAgent | undefined;
	openFrameTab: (tab: NewFrameTab) => void;
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

			return () => {
				focusPanel();
				openFrameTab({
					kind: "preview",
					label: preview.title,
					workspaceId: preview.workspaceId,
					agentName: preview.agentName,
					slug: preview.slug,
					urlPath: preview.urlPath,
				});
			};
		},
	};
};
