import type * as TypesGen from "api/typesGenerated";
import {
	ConversationItem,
	Message,
	MessageContent,
	Response,
	Shimmer,
	Tool,
} from "components/ai-elements";
import { ChevronDownIcon } from "lucide-react";
import { type FC, memo, type ReactNode, type RefObject, useState } from "react";
import { cn } from "utils/cn";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageSection,
	RenderBlock,
	StreamState,
} from "./types";

const ReasoningDisclosure: FC<{
	id: string;
	title?: string;
	text: string;
	isStreaming?: boolean;
}> = ({ id, title, text, isStreaming = false }) => {
	const [isOpen, setIsOpen] = useState(false);
	const hasText = text.trim().length > 0;
	const label = title ?? "Thinking";
	const showStreamingPlaceholder = isStreaming && !hasText;

	if (!title && hasText) {
		return (
			<div className="w-full">
				<Response className="text-[11px] text-content-secondary">
					{text}
				</Response>
			</div>
		);
	}

	const labelContent = (
		<span className="text-sm">
			{showStreamingPlaceholder ? (
				<Shimmer as="span">Thinking...</Shimmer>
			) : (
				label
			)}
		</span>
	);

	return (
		<div className="w-full">
			{hasText ? (
				<div
					role="button"
					tabIndex={0}
					aria-expanded={isOpen}
					aria-controls={id}
					className="flex items-center gap-2 text-content-secondary transition-colors hover:text-content-primary cursor-pointer"
					onClick={() => setIsOpen((prev) => !prev)}
					onKeyDown={(event) => {
						if (event.key === "Enter" || event.key === " ") {
							setIsOpen((prev) => !prev);
						}
					}}
				>
					{labelContent}
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							isOpen ? "rotate-0" : "-rotate-90",
						)}
					/>
				</div>
			) : (
				<div className="flex items-center gap-2 text-content-secondary transition-colors hover:text-content-primary">
					{labelContent}
				</div>
			)}
			{isOpen && hasText ? (
				<div id={id} className="mt-1.5">
					<Response className="text-[11px] text-content-secondary">
						{text}
					</Response>
				</div>
			) : null}
		</div>
	);
};

// Shared block renderer used by both ChatMessageItem (historical
// messages) and StreamingOutput (live stream). Encapsulates the
// response / thinking / tool switch so the two consumers stay in sync.
type RenderBlockListParams = {
	blocks: readonly RenderBlock[];
	toolByID: ReadonlyMap<string, MergedTool>;
	keyPrefix: string;
	isStreaming?: boolean;
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
};

type RenderBlockListResult = {
	elements: ReactNode[];
	renderedToolIDs: ReadonlySet<string>;
};

function renderBlockList({
	blocks,
	toolByID,
	keyPrefix,
	isStreaming = false,
	subagentTitles,
	subagentStatusOverrides,
}: RenderBlockListParams): RenderBlockListResult {
	const renderedToolIDs = new Set<string>();
	const elements = blocks
		.map((block, index) => {
			switch (block.type) {
				case "response":
					return (
						<Response key={`${keyPrefix}-response-${index}`}>
							{block.text}
						</Response>
					);
				case "thinking":
					return (
						<ReasoningDisclosure
							key={`${keyPrefix}-thinking-${index}`}
							id={`${keyPrefix}-thinking-${index}`}
							title={block.title}
							text={block.text}
							isStreaming={isStreaming}
						/>
					);
				case "tool": {
					const tool = toolByID.get(block.id);
					if (!tool) {
						if (!isStreaming) {
							return null;
						}
						// Streaming placeholder for not-yet-resolved tool.
						renderedToolIDs.add(block.id);
						return (
							<Tool
								key={block.id}
								name="Tool"
								status="running"
								isError={false}
								subagentTitles={subagentTitles}
								subagentStatusOverrides={subagentStatusOverrides}
							/>
						);
					}
					renderedToolIDs.add(tool.id);
					return (
						<Tool
							key={tool.id}
							name={tool.name}
							args={tool.args}
							result={tool.result}
							status={tool.status}
							isError={tool.isError}
							subagentTitles={isStreaming ? subagentTitles : undefined}
							subagentStatusOverrides={
								isStreaming ? subagentStatusOverrides : undefined
							}
						/>
					);
				}
				default:
					return null;
			}
		})
		.filter((el): el is NonNullable<typeof el> => el != null);
	return { elements, renderedToolIDs };
}

const ChatMessageItem = memo<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
}>(({ message, parsed }) => {
	const isUser = message.role === "user";
	const toolByID = new Map(parsed.tools.map((tool) => [tool.id, tool]));

	if (
		parsed.toolResults.length > 0 &&
		parsed.toolCalls.length === 0 &&
		parsed.markdown === "" &&
		parsed.reasoning === ""
	) {
		return null;
	}

	const hasRenderableContent =
		parsed.blocks.length > 0 || parsed.tools.length > 0;
	const conversationItemProps: { role: "user" | "assistant" } = {
		role: isUser ? "user" : "assistant",
	};
	const { elements: orderedBlocks, renderedToolIDs } = renderBlockList({
		blocks: parsed.blocks,
		toolByID,
		keyPrefix: String(message.id),
	});
	const remainingTools = parsed.tools.filter(
		(tool) => !renderedToolIDs.has(tool.id),
	);

	return (
		<ConversationItem {...conversationItemProps}>
			{isUser ? (
				<Message className="my-2 w-full max-w-none">
					<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
						{parsed.markdown || ""}
					</MessageContent>
				</Message>
			) : (
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<div className="space-y-3">
							{orderedBlocks}
							{remainingTools.map((tool) => (
								<Tool
									key={tool.id}
									name={tool.name}
									args={tool.args}
									result={tool.result}
									status={tool.status}
									isError={tool.isError}
								/>
							))}
							{!hasRenderableContent && (
								<div className="text-xs text-content-secondary">
									Message has no renderable content.
								</div>
							)}
						</div>
					</MessageContent>
				</Message>
			)}
		</ConversationItem>
	);
});
ChatMessageItem.displayName = "ChatMessageItem";

const StreamingOutput = memo<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	showInitialPlaceholder?: boolean;
}>(
	({
		streamState,
		streamTools,
		subagentTitles,
		subagentStatusOverrides,
		showInitialPlaceholder = false,
	}) => {
		const conversationItemProps = { role: "assistant" as const };
		const toolByID = new Map(streamTools.map((tool) => [tool.id, tool]));
		const blocks = streamState?.blocks ?? [];
		const { elements: orderedBlocks, renderedToolIDs } = renderBlockList({
			blocks,
			toolByID,
			keyPrefix: "stream",
			isStreaming: true,
			subagentTitles,
			subagentStatusOverrides,
		});
		const remainingTools = streamTools.filter(
			(tool) => !renderedToolIDs.has(tool.id),
		);

		return (
			<ConversationItem {...conversationItemProps}>
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<div className="space-y-3">
							{orderedBlocks}
							{showInitialPlaceholder ||
							(streamState &&
								orderedBlocks.length === 0 &&
								streamTools.length === 0) ? (
								<div className="relative">
									<Response aria-hidden className="invisible">
										Thinking...
									</Response>
									<div className="pointer-events-none absolute inset-0">
										<Shimmer as="div" className="text-[13px] leading-relaxed">
											Thinking...
										</Shimmer>
									</div>
								</div>
							) : null}
							{remainingTools.map((tool) => (
								<Tool
									key={tool.id}
									name={tool.name}
									args={tool.args}
									result={tool.result}
									status={tool.status}
									isError={tool.isError}
									subagentTitles={subagentTitles}
									subagentStatusOverrides={subagentStatusOverrides}
								/>
							))}
						</div>
					</MessageContent>
				</Message>
			</ConversationItem>
		);
	},
);
StreamingOutput.displayName = "StreamingOutput";

const StickyUserMessage: FC<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
}> = ({ message, parsed }) => {
	return (
		<div className="sticky -top-2 z-10 pt-2">
			<ChatMessageItem message={message} parsed={parsed} />
		</div>
	);
};

type ConversationTimelineProps = {
	isEmpty: boolean;
	hasMoreMessages: boolean;
	loadMoreSentinelRef: RefObject<HTMLDivElement | null>;
	parsedSections: readonly ParsedMessageSection[];
	hasStreamOutput: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles: Map<string, string>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	isAwaitingFirstStreamChunk: boolean;
	detailErrorMessage?: string | null;
};

export const ConversationTimeline: FC<ConversationTimelineProps> = ({
	isEmpty,
	hasMoreMessages,
	loadMoreSentinelRef,
	parsedSections,
	hasStreamOutput,
	streamState,
	streamTools,
	subagentTitles,
	subagentStatusOverrides,
	isAwaitingFirstStreamChunk,
	detailErrorMessage,
}) => {
	const shouldRenderStreamInLastSection =
		hasStreamOutput && parsedSections.length > 0;

	return (
		<div className="mx-auto w-full max-w-3xl py-6">
			{isEmpty && !hasStreamOutput ? (
				<div className="py-12 text-center text-content-secondary">
					<p className="text-sm">Start a conversation with your agent.</p>
				</div>
			) : (
				<div className="flex flex-col">
					{hasMoreMessages && (
						<div
							ref={loadMoreSentinelRef}
							className="flex items-center justify-center py-4 text-xs text-content-secondary"
						>
							Loading earlier messages…
						</div>
					)}
					{parsedSections.map((section, sectionIdx) => (
						<div
							key={section.userEntry?.message.id ?? `section-${sectionIdx}`}
							style={{
								contentVisibility: "auto",
								containIntrinsicSize: "1px 600px",
							}}
						>
							<div className="flex flex-col gap-3">
								{section.entries.map(({ message, parsed }) =>
									message.role === "user" ? (
										<StickyUserMessage
											key={message.id}
											message={message}
											parsed={parsed}
										/>
									) : (
										<ChatMessageItem
											key={message.id}
											message={message}
											parsed={parsed}
										/>
									),
								)}
								{shouldRenderStreamInLastSection &&
									sectionIdx === parsedSections.length - 1 && (
										<StreamingOutput
											streamState={streamState}
											streamTools={streamTools}
											subagentTitles={subagentTitles}
											subagentStatusOverrides={subagentStatusOverrides}
											showInitialPlaceholder={isAwaitingFirstStreamChunk}
										/>
									)}
							</div>
						</div>
					))}
					{hasStreamOutput && parsedSections.length === 0 && (
						<StreamingOutput
							streamState={streamState}
							streamTools={streamTools}
							subagentTitles={subagentTitles}
							subagentStatusOverrides={subagentStatusOverrides}
							showInitialPlaceholder={isAwaitingFirstStreamChunk}
						/>
					)}
				</div>
			)}
			{detailErrorMessage && (
				<div className="mt-4 rounded-md border border-border-destructive bg-surface-red px-3 py-2 text-xs text-content-destructive">
					{detailErrorMessage}
				</div>
			)}
		</div>
	);
};
