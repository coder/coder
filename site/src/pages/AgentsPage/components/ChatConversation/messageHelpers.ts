import type * as TypesGen from "#/api/typesGenerated";
import { shouldRenderTool } from "../ChatElements/tools/toolVisibility";
import type { ParsedMessageContent, RenderBlock } from "./types";

export type UserInlineRenderBlock =
	| Extract<RenderBlock, { type: "response" }>
	| Extract<RenderBlock, { type: "file-reference" }>;

type FileRenderBlock = Extract<RenderBlock, { type: "file" }>;
type WorkspaceFileReferenceBlock = Extract<
	RenderBlock,
	{ type: "workspace-file-reference" }
>;

export type MessageDisplayState = {
	shouldHide: boolean;
	userInlineContent: UserInlineRenderBlock[];
	userFileBlocks: FileRenderBlock[];
	workspaceFileReferenceCount: number;
	hasUserMessageBody: boolean;
	hasFileBlocks: boolean;
	hasWorkspaceFileReferences: boolean;
	hasCopyableContent: boolean;
	needsAssistantBottomSpacer: boolean;
};

const isUserInlineRenderBlock = (
	block: RenderBlock,
): block is UserInlineRenderBlock =>
	block.type === "response" || block.type === "file-reference";

const isFileRenderBlock = (block: RenderBlock): block is FileRenderBlock =>
	block.type === "file";

const isWorkspaceFileReferenceBlock = (
	block: RenderBlock,
): block is WorkspaceFileReferenceBlock =>
	block.type === "workspace-file-reference";

const isProviderToolResultOnlyMessage = (
	parts: readonly TypesGen.ChatMessagePart[],
): boolean =>
	parts.length > 0 &&
	parts.every((part) => part.type === "tool-result" && part.provider_executed);

const isMetadataOnlyMessage = (
	parts: readonly TypesGen.ChatMessagePart[],
): boolean =>
	parts.length > 0 &&
	parts.every((part) => part.type === "context-file" || part.type === "skill");

const getRenderableContentState = (parsed: ParsedMessageContent) => {
	const visibleTools = parsed.tools.filter((tool) =>
		shouldRenderTool({
			name: tool.name,
			status: tool.status,
			args: tool.args,
			result: tool.result,
		}),
	);
	const visibleToolIds = new Set(visibleTools.map((tool) => tool.id));
	const visibleBlocks = parsed.blocks.filter(
		(block) =>
			block.type !== "workspace-file-reference" &&
			(block.type !== "tool" || visibleToolIds.has(block.id)),
	);
	const hasRenderableContent =
		visibleBlocks.length > 0 ||
		visibleTools.length > 0 ||
		parsed.sources.length > 0;
	const hasThinkingOnlyContent =
		visibleBlocks.length > 0 &&
		visibleBlocks.every((block) => block.type === "thinking");

	return {
		hasRenderableContent,
		hasThinkingOnlyContent,
	};
};

export const deriveMessageDisplayState = ({
	message,
	parsed,
	hideActions,
	hasActiveStream,
	isAwaitingFirstStreamChunk = false,
}: {
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	hideActions: boolean;
	hasActiveStream: boolean;
	isAwaitingFirstStreamChunk?: boolean;
}): MessageDisplayState => {
	const isUser = message.role === "user";
	const userInlineContent: UserInlineRenderBlock[] = [];
	const userFileBlocks: FileRenderBlock[] = [];
	let workspaceFileReferenceCount = 0;
	let hasFileAttachments = false;
	for (const block of parsed.blocks) {
		if (isFileRenderBlock(block)) {
			hasFileAttachments = true;
			if (isUser) {
				userFileBlocks.push(block);
			}
			continue;
		}

		if (!isUser) {
			continue;
		}
		if (isUserInlineRenderBlock(block)) {
			userInlineContent.push(block);
			continue;
		}
		if (isWorkspaceFileReferenceBlock(block)) {
			workspaceFileReferenceCount++;
		}
	}
	const hasWorkspaceFileReferences = workspaceFileReferenceCount > 0;
	const hasUserMessageBody =
		userInlineContent.length > 0 || Boolean(parsed.markdown.trim());
	const hasFileBlocks = userFileBlocks.length > 0;
	const hasCopyableContent =
		Boolean(parsed.markdown.trim()) &&
		!hasFileAttachments &&
		!hasWorkspaceFileReferences;
	const { hasRenderableContent, hasThinkingOnlyContent } =
		getRenderableContentState(parsed);
	const needsAssistantBottomSpacer =
		!hideActions &&
		!hasActiveStream &&
		!isAwaitingFirstStreamChunk &&
		!isUser &&
		!hasCopyableContent &&
		(hasThinkingOnlyContent || parsed.sources.length > 0);
	const hasToolResultsOnly =
		parsed.toolResults.length > 0 &&
		parsed.toolCalls.length === 0 &&
		parsed.markdown === "" &&
		parsed.reasoning === "";
	const parts = message.content ?? [];

	return {
		shouldHide:
			hasToolResultsOnly ||
			isProviderToolResultOnlyMessage(parts) ||
			isMetadataOnlyMessage(parts) ||
			(!isUser && !hasRenderableContent),
		userInlineContent,
		userFileBlocks,
		workspaceFileReferenceCount,
		hasUserMessageBody,
		hasFileBlocks,
		hasWorkspaceFileReferences,
		hasCopyableContent,
		needsAssistantBottomSpacer,
	};
};
