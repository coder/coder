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
		!isPreviewTitle(rec.title) ||
		typeof rec.workspace_id !== "string" ||
		typeof rec.agent_name !== "string" ||
		// coderd checks the slug and URL path in full on every address and
		// sign-in request, so the parser checks only their shape.
		typeof rec.preview_slug !== "string" ||
		!rec.preview_slug.startsWith("preview--") ||
		(rec.url_path !== undefined &&
			(typeof rec.url_path !== "string" || !rec.url_path.startsWith("/")))
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
 * Rendering for `open_preview` tool calls. When the chat can show previews,
 * a successful result gets an action that opens its right panel tab, or the
 * reason this viewer cannot open it here.
 */
export const OpenPreviewTool: React.FC<{
	args: unknown;
	result: unknown;
	status: ToolStatus;
	isError: boolean;
	toolCallId?: string;
}> = ({ args, result, status, isError, toolCallId }) => {
	const { workspaceId, agentName, previewAccess } = useChatWorkspace();
	const failed = status !== "running" && (isError || status === "error");
	const preview = failed ? undefined : parseOpenPreviewResult(result);
	const access =
		preview &&
		previewAccess &&
		(preview.workspaceId !== workspaceId || preview.agentName !== agentName)
			? { unavailableReason: "This preview belongs to a different workspace." }
			: previewAccess;
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
				{preview &&
					access &&
					("open" in access ? (
						<ToolCall.HeaderActions>
							<Button
								size="xs"
								variant="outline"
								onClick={() => access.open({ ...preview, toolCallId })}
							>
								Open preview
							</Button>
						</ToolCall.HeaderActions>
					) : (
						<ToolCall.HeaderActions className="min-w-0 shrink">
							<span className="text-right text-xs text-content-secondary">
								{access.unavailableReason}
							</span>
						</ToolCall.HeaderActions>
					))}
			</ToolCall.HeaderLayout>
		</ToolCall.Root>
	);
};
