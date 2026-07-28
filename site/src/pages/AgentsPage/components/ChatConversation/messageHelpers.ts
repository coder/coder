import type * as TypesGen from "#/api/typesGenerated";
import { shouldRenderTool } from "../ChatElements/tools/toolVisibility";
import type {
	ParsedMessageContent,
	ParsedMessageEntry,
	RenderBlock,
} from "./types";

export type UserInlineRenderBlock =
	| Extract<RenderBlock, { type: "response" }>
	| Extract<RenderBlock, { type: "file-reference" }>;

type FileRenderBlock = Extract<RenderBlock, { type: "file" }>;

export type MessageDisplayState = {
	shouldHide: boolean;
	userInlineContent: UserInlineRenderBlock[];
	userFileBlocks: FileRenderBlock[];
	hasUserMessageBody: boolean;
	hasFileBlocks: boolean;
	hasCopyableContent: boolean;
	needsAssistantBottomSpacer: boolean;
};

type MessageEntryInput = {
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
};

type HiddenTimelineEntryReason =
	| "tool-result"
	| "metadata-only"
	| "empty-non-user";

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
	const endsWithResponseBlock =
		visibleBlocks.length > 0 &&
		visibleBlocks[visibleBlocks.length - 1].type === "response";

	return {
		hasRenderableContent,
		hasThinkingOnlyContent,
		endsWithResponseBlock,
	};
};

const isToolResultOnlyEntry = ({
	message,
	parsed,
}: MessageEntryInput): boolean =>
	message.role === "tool" &&
	parsed.toolResults.length > 0 &&
	parsed.toolCalls.length === 0 &&
	parsed.markdown === "" &&
	parsed.reasoning === "";

const getHiddenTimelineEntryReason = ({
	message,
	parsed,
}: MessageEntryInput): HiddenTimelineEntryReason | undefined => {
	const parts = message.content ?? [];
	const { hasRenderableContent } = getRenderableContentState(parsed);

	if (
		isToolResultOnlyEntry({ message, parsed }) ||
		isProviderToolResultOnlyMessage(parts)
	) {
		return "tool-result";
	}

	if (isMetadataOnlyMessage(parts)) {
		return "metadata-only";
	}

	if (message.role !== "user" && !hasRenderableContent) {
		return "empty-non-user";
	}

	return undefined;
};

const shouldHideTimelineEntry = (entry: MessageEntryInput): boolean =>
	getHiddenTimelineEntryReason(entry) !== undefined;

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
	const { hasThinkingOnlyContent, endsWithResponseBlock } =
		getRenderableContentState(parsed);
	// The copy action row renders below the whole message, so assistant
	// messages only get one when the last visible block is text.
	// Otherwise the button would sit under a tool call with nothing
	// copyable directly above it.
	const hasCopyableContent =
		Boolean(parsed.markdown.trim()) &&
		!hasFileAttachments &&
		(isUser || endsWithResponseBlock);
	const needsAssistantBottomSpacer =
		!hideActions &&
		!hasActiveStream &&
		!isAwaitingFirstStreamChunk &&
		!isUser &&
		!hasCopyableContent &&
		(hasThinkingOnlyContent || parsed.sources.length > 0);
	return {
		shouldHide: shouldHideTimelineEntry({ message, parsed }),
		userInlineContent,
		userFileBlocks,
		hasUserMessageBody,
		hasFileBlocks,
		hasCopyableContent,
		needsAssistantBottomSpacer,
	};
};

const isReadFileOnlyMessage = (entry: ParsedMessageEntry): boolean => {
	if (entry.message.role !== "assistant") {
		return false;
	}
	if (
		entry.parsed.blocks.length === 0 ||
		entry.parsed.markdown.trim() ||
		entry.parsed.reasoning.trim() ||
		entry.parsed.sources.length > 0
	) {
		return false;
	}

	const toolByID = new Map(entry.parsed.tools.map((tool) => [tool.id, tool]));
	return entry.parsed.blocks.every(
		(block) =>
			block.type === "tool" && toolByID.get(block.id)?.name === "read_file",
	);
};

const mergeReadFileMessageGroup = (
	group: readonly ParsedMessageEntry[],
): ParsedMessageEntry => {
	if (group.length === 1) {
		return group[0];
	}

	const [first] = group;
	return {
		message: first.message,
		parsed: {
			markdown: "",
			reasoning: "",
			toolCalls: group.flatMap((entry) => entry.parsed.toolCalls),
			toolResults: group.flatMap((entry) => entry.parsed.toolResults),
			tools: group.flatMap((entry) => entry.parsed.tools),
			blocks: group.flatMap((entry) => entry.parsed.blocks),
			sources: [],
		},
	};
};

// Real transcripts place hidden tool-result-only messages between
// sequential read_file assistant messages. Those hidden entries stay
// transparent so the visible timeline reflects one file-reading run instead
// of one row per persisted message. Synthetic grouped entries deliberately
// render from merged parsed fields because their raw message payload still
// belongs to the first persisted message.
export const buildDisplayMessages = (
	entries: readonly ParsedMessageEntry[],
): ParsedMessageEntry[] => {
	const grouped: ParsedMessageEntry[] = [];
	let currentReadFileEntries: ParsedMessageEntry[] = [];

	const flushReadFileEntries = () => {
		if (currentReadFileEntries.length === 0) {
			return;
		}
		grouped.push(mergeReadFileMessageGroup(currentReadFileEntries));
		currentReadFileEntries = [];
	};

	for (const entry of entries) {
		if (shouldHideTimelineEntry(entry)) {
			continue;
		}
		if (isReadFileOnlyMessage(entry)) {
			currentReadFileEntries.push(entry);
			continue;
		}

		flushReadFileEntries();
		grouped.push(entry);
	}

	flushReadFileEntries();
	return grouped;
};
