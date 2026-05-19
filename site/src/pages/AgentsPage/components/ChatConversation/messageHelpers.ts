import type * as TypesGen from "#/api/typesGenerated";
import type { ParsedMessageContent, RenderBlock } from "./types";

export type UserInlineRenderBlock =
	| Extract<RenderBlock, { type: "response" }>
	| Extract<RenderBlock, { type: "file-reference" }>;

type FileRenderBlock = Extract<RenderBlock, { type: "file" }>;
type WorkspaceFileRenderBlock = Extract<
	RenderBlock,
	{ type: "workspace-file-reference" }
>;

export type MessageDisplayState = {
	shouldHide: boolean;
	userInlineContent: UserInlineRenderBlock[];
	userFileBlocks: FileRenderBlock[];
	userWorkspaceFileBlocks: WorkspaceFileRenderBlock[];
	hasUserMessageBody: boolean;
	hasFileBlocks: boolean;
	hasWorkspaceFileBlocks: boolean;
	hasCopyableContent: boolean;
	needsAssistantBottomSpacer: boolean;
};

const isUserInlineRenderBlock = (
	block: RenderBlock,
): block is UserInlineRenderBlock =>
	block.type === "response" || block.type === "file-reference";

const isFileRenderBlock = (block: RenderBlock): block is FileRenderBlock =>
	block.type === "file";

const isWorkspaceFileRenderBlock = (
	block: RenderBlock,
): block is WorkspaceFileRenderBlock =>
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
	const userInlineContent = isUser
		? parsed.blocks.filter(isUserInlineRenderBlock)
		: [];
	const userFileBlocks = isUser ? parsed.blocks.filter(isFileRenderBlock) : [];
	const userWorkspaceFileBlocks = isUser
		? parsed.blocks.filter(isWorkspaceFileRenderBlock)
		: [];
	const hasFileAttachments = parsed.blocks.some(isFileRenderBlock);
	const hasUserMessageBody =
		userInlineContent.length > 0 || Boolean(parsed.markdown.trim());
	const hasFileBlocks = userFileBlocks.length > 0;
	const hasWorkspaceFileBlocks = userWorkspaceFileBlocks.length > 0;
	const hasCopyableContent =
		Boolean(parsed.markdown.trim()) && !hasFileAttachments;
	const hasRenderableContent =
		parsed.blocks.length > 0 ||
		parsed.tools.length > 0 ||
		parsed.sources.length > 0;
	const needsAssistantBottomSpacer =
		!hideActions &&
		!hasActiveStream &&
		!isAwaitingFirstStreamChunk &&
		!isUser &&
		!hasCopyableContent &&
		(Boolean(parsed.reasoning) ||
			parsed.sources.length > 0 ||
			!hasRenderableContent);
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
			isMetadataOnlyMessage(parts),
		userInlineContent,
		userFileBlocks,
		userWorkspaceFileBlocks,
		hasUserMessageBody,
		hasFileBlocks,
		hasWorkspaceFileBlocks,
		hasCopyableContent,
		needsAssistantBottomSpacer,
	};
};
