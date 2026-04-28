import type * as TypesGen from "#/api/typesGenerated";
import type { ParsedMessageContent, RenderBlock } from "./types";

export type UserInlineRenderBlock =
	| Extract<RenderBlock, { type: "response" }>
	| Extract<RenderBlock, { type: "file-reference" }>;

type UserFileRenderBlock = Extract<RenderBlock, { type: "file" }>;

export type MessageDisplayState = {
	shouldHide: boolean;
	userInlineContent: UserInlineRenderBlock[];
	userFileBlocks: UserFileRenderBlock[];
	hasUserMessageBody: boolean;
	hasFileBlocks: boolean;
	hasCopyableContent: boolean;
	needsAssistantBottomSpacer: boolean;
};

const isUserInlineRenderBlock = (
	block: RenderBlock,
): block is UserInlineRenderBlock =>
	block.type === "response" || block.type === "file-reference";

const isUserFileRenderBlock = (
	block: RenderBlock,
): block is UserFileRenderBlock => block.type === "file";

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
}: {
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	hideActions: boolean;
}): MessageDisplayState => {
	const isUser = message.role === "user";
	const userInlineContent = isUser
		? parsed.blocks.filter(isUserInlineRenderBlock)
		: [];
	const userFileBlocks = isUser
		? parsed.blocks.filter(isUserFileRenderBlock)
		: [];
	const hasUserMessageBody =
		userInlineContent.length > 0 || Boolean(parsed.markdown.trim());
	const hasFileBlocks = userFileBlocks.length > 0;
	// On assistant messages, suppress the copy affordance when tool
	// calls or tool results are present. The actions toolbar reserves
	// vertical space (~24px), and toggling that reservation as the
	// message moves out of "last in chain" causes a visible layout
	// shift below tool-heavy responses. Pure-text assistant turns
	// keep the copy button.
	const hasCopyableContent =
		Boolean(parsed.markdown.trim()) &&
		(message.role === "user" ||
			(parsed.toolCalls.length === 0 && parsed.toolResults.length === 0));
	const hasRenderableContent =
		parsed.blocks.length > 0 ||
		parsed.tools.length > 0 ||
		parsed.sources.length > 0;
	const needsAssistantBottomSpacer =
		!hideActions &&
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
		hasUserMessageBody,
		hasFileBlocks,
		hasCopyableContent,
		needsAssistantBottomSpacer,
	};
};
