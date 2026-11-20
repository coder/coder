import type React from "react";
import { Button } from "#/components/Button/Button";
import { useChatWorkspace } from "../../../context/ChatWorkspaceContext";
import type { NewPreviewTab } from "../../../utils/rightPanelTabs";
import { ToolCall } from "./ToolCall";
import { asRecord, asString, parseArgs, type ToolStatus } from "./utils";

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
 * Parses a stored `open_preview` tool result into the tab that shows it.
 * Returns undefined for an error result or one that fails validation.
 */
export const parseOpenPreviewResult = (
	result: unknown,
): NewPreviewTab | undefined => {
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
		kind: "preview",
		label: rec.title,
		workspaceId: rec.workspace_id,
		agentName: rec.agent_name,
		slug: rec.preview_slug,
		urlPath: rec.url_path,
	};
};

/**
 * Rendering for `open_preview` tool calls, with an action that opens the
 * preview's right panel tab when it belongs to the chat's workspace agent.
 */
export const OpenPreviewTool: React.FC<{
	args: unknown;
	result: unknown;
	status: ToolStatus;
	isError: boolean;
}> = ({ args, result, status, isError }) => {
	const { workspaceId, agentName, openPreview } = useChatWorkspace();
	const failed = status !== "running" && (isError || status === "error");
	const preview = failed ? undefined : parseOpenPreviewResult(result);
	const open =
		preview &&
		openPreview &&
		preview.workspaceId === workspaceId &&
		preview.agentName === agentName
			? () => openPreview(preview)
			: undefined;
	const title =
		preview?.label || asString(parseArgs(args)?.title).trim() || "preview";

	const label =
		status === "running"
			? `Preparing ${title}…`
			: failed
				? `Failed to open ${title}`
				: title;

	return (
		<ToolCall.Root
			className="w-full"
			status={status}
			isError={isError}
			errorMessage={
				asString(asRecord(result)?.error) || "Failed to open preview"
			}
			hasContent={false}
		>
			<ToolCall.HeaderLayout>
				<ToolCall.Header iconName="open_preview" label={label} />
				{open && (
					<ToolCall.HeaderActions>
						<Button size="xs" variant="outline" onClick={open}>
							Open preview
						</Button>
					</ToolCall.HeaderActions>
				)}
			</ToolCall.HeaderLayout>
		</ToolCall.Root>
	);
};
