import type * as TypesGen from "#/api/typesGenerated";
import { shouldRenderTool } from "../ChatElements/tools/toolVisibility";
import type { ParsedMessageContent, RenderBlock } from "./types";

export type UserInlineRenderBlock =
	| Extract<RenderBlock, { type: "response" }>
	| Extract<RenderBlock, { type: "file-reference" }>;

type FileRenderBlock = Extract<RenderBlock, { type: "file" }>;

export type MessageDisplayState = {
	shouldHide: boolean;
	hasRenderableContent: boolean;
	userInlineContent: UserInlineRenderBlock[];
	userFileBlocks: FileRenderBlock[];
	hasUserMessageBody: boolean;
	hasFileBlocks: boolean;
	hasCopyableContent: boolean;
	needsAssistantBottomSpacer: boolean;
};

const isUserInlineRenderBlock = (
	block: RenderBlock,
): block is UserInlineRenderBlock =>
	block.type === "response" || block.type === "file-reference";

const isFileRenderBlock = (block: RenderBlock): block is FileRenderBlock =>
	block.type === "file";

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
		(block) => block.type !== "tool" || visibleToolIds.has(block.id),
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
	const userInlineContent = isUser
		? parsed.blocks.filter(isUserInlineRenderBlock)
		: [];
	const userFileBlocks = isUser ? parsed.blocks.filter(isFileRenderBlock) : [];
	const hasFileAttachments = parsed.blocks.some(isFileRenderBlock);
	const hasUserMessageBody =
		userInlineContent.length > 0 || Boolean(parsed.markdown.trim());
	const hasFileBlocks = userFileBlocks.length > 0;
	const hasCopyableContent =
		Boolean(parsed.markdown.trim()) && !hasFileAttachments;
	const { hasRenderableContent, hasThinkingOnlyContent } =
		getRenderableContentState(parsed);
	const hasVisibleToolPlaceholders =
		parsed.blocks.some((block) => block.type === "tool") ||
		parsed.tools.length > 0;
	const needsAssistantBottomSpacer =
		!hideActions &&
		!hasActiveStream &&
		!isAwaitingFirstStreamChunk &&
		!isUser &&
		!hasCopyableContent &&
		(hasThinkingOnlyContent ||
			parsed.sources.length > 0 ||
			(!hasRenderableContent && !hasVisibleToolPlaceholders));
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
			(!isUser && !hasRenderableContent && hasVisibleToolPlaceholders),
		hasRenderableContent,
		userInlineContent,
		userFileBlocks,
		hasUserMessageBody,
		hasFileBlocks,
		hasCopyableContent,
		needsAssistantBottomSpacer,
	};
};
