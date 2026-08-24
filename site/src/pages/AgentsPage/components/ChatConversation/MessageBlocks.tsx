import { type FC, memo, useLayoutEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import type { UrlTransform } from "streamdown";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { Response, Tool } from "../ChatElements";
import { WebSearchSources } from "../ChatElements/tools";
import { ReadFilesTool } from "../ChatElements/tools/ReadFilesTool";
import {
	getReadFileToolData,
	ReadFileTool,
} from "../ChatElements/tools/ReadFileTool";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import { groupSequentialReadFileBlocks } from "./blockUtils";
import { useSmoothStreamingText } from "./SmoothText";
import { getThinkingDisclosureDisplay } from "./thinkingTitle";
import type { MergedTool, RenderBlock } from "./types";

const ReasoningDisclosure = memo<{
	id: string;
	text: string;
	isStreaming?: boolean;
	urlTransform?: UrlTransform;
	thinkingDisplayMode?: ThinkingDisplayMode;
}>(
	({
		id,
		text,
		isStreaming = false,
		urlTransform,
		thinkingDisplayMode: mode = "auto",
	}) => {
		const [manualToggle, setManualToggle] = useState<boolean | null>(null);

		// Reset manual override on streaming transitions so
		// auto/preview modes collapse when streaming stops.
		const [prevStreaming, setPrevStreaming] = useState(isStreaming);
		if (prevStreaming !== isStreaming) {
			setPrevStreaming(isStreaming);
			if (mode === "auto" || mode === "preview") {
				setManualToggle(null);
			}
		}

		const autoExpanded = (() => {
			switch (mode) {
				case "always_expanded":
					return true;
				case "always_collapsed":
					return false;
				case "auto":
				case "preview":
					return isStreaming;
				default: {
					const _exhaustive: never = mode;
					return _exhaustive;
				}
			}
		})();

		const expanded = manualToggle ?? autoExpanded;

		const isPreviewConstrained =
			mode === "preview" && isStreaming && manualToggle === null;

		const previewScrollRef = useRef<HTMLDivElement>(null);

		const { visibleText } = useSmoothStreamingText({
			fullText: text,
			isStreaming,
			bypassSmoothing: !isStreaming,
			streamKey: id,
		});
		const displayText = isStreaming ? visibleText : text;
		const { title, body } = getThinkingDisclosureDisplay(displayText);
		const hasText = body.trim().length > 0;

		// Auto-scroll the preview container to the bottom as new
		// thinking content streams in. useLayoutEffect avoids a
		// visible frame where content has grown but not scrolled.
		const displayTextLength = body.length;
		useLayoutEffect(() => {
			if (
				displayTextLength &&
				isPreviewConstrained &&
				previewScrollRef.current
			) {
				previewScrollRef.current.scrollTop =
					previewScrollRef.current.scrollHeight;
			}
		}, [displayTextLength, isPreviewConstrained]);

		return (
			<div data-transcript-row="">
				<ToolCall.Root
					className="w-full"
					status={isStreaming ? "running" : "completed"}
					hasContent={hasText}
					expanded={expanded}
					onExpandedChange={(open) => setManualToggle(open)}
				>
					<ToolCall.Header
						iconName="thinking"
						label={title}
						showStatus={false}
					/>
					<ToolCall.Content>
						<div
							ref={previewScrollRef}
							className={cn(
								"mt-1.5",
								isPreviewConstrained && "max-h-24 overflow-y-auto",
							)}
						>
							<Response
								className="text-[11px] text-content-secondary"
								urlTransform={urlTransform}
								streaming={isStreaming}
							>
								{body}
							</Response>
						</div>
					</ToolCall.Content>
				</ToolCall.Root>
			</div>
		);
	},
);

// Runs the smooth-streaming jitter buffer while the turn is live and renders
// the raw text once it is durable, so both shapes render through the same
// code path.
const ResponseBlock = memo<{
	text: string;
	isStreaming: boolean;
	streamKey: string;
	urlTransform?: UrlTransform;
}>(({ text, isStreaming, streamKey, urlTransform }) => {
	const { visibleText } = useSmoothStreamingText({
		fullText: text,
		isStreaming,
		bypassSmoothing: !isStreaming,
		streamKey,
	});
	return (
		<Response streaming={isStreaming} urlTransform={urlTransform}>
			{isStreaming ? visibleText : text}
		</Response>
	);
});

const ReadFileTimelineBlock = memo<{
	tools: readonly [MergedTool, ...MergedTool[]];
}>(({ tools }) => {
	const [expanded, setExpanded] = useState(false);
	const [firstTool] = tools;
	if (tools.length === 1) {
		const readFile = getReadFileToolData(firstTool);
		return (
			<ToolCall.PolicyProvider hookRewritten={firstTool.hookRewritten ?? false}>
				<div data-tool-call="">
					<ReadFileTool
						{...readFile}
						status={firstTool.status}
						expanded={expanded}
						onExpandedChange={setExpanded}
					/>
				</div>
			</ToolCall.PolicyProvider>
		);
	}

	return (
		<ReadFilesTool
			tools={tools}
			expanded={expanded}
			onExpandedChange={setExpanded}
		/>
	);
});

export type BlockListProps = {
	organizationId?: string;
	blocks: readonly RenderBlock[];
	tools: readonly MergedTool[];
	keyPrefix: string;
	isStreaming?: boolean;
	subagentTitles?: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	showDesktopPreviews?: boolean;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	latestAskUserQuestionToolId?: string;
	askUserQuestionResponseTextByToolId?: ReadonlyMap<string, string>;
	hasUserResponseAfterAskQuestion?: boolean;
	urlTransform?: UrlTransform;
};

// Shared block renderer for durable messages and the live assistant turn.
// Encapsulates the response / thinking / tool / file / sources switch so both
// consumers stay in sync. PascalCase so the React Compiler auto-memoizes every
// element inside.
export const BlockList: FC<BlockListProps> = ({
	organizationId,
	blocks,
	tools,
	keyPrefix,
	isStreaming = false,
	subagentTitles,
	subagentVariants,
	showDesktopPreviews,
	subagentStatusOverrides,
	mcpServers,
	onImageClick,
	onTextFileClick,
	onImplementPlan,
	onSendAskUserQuestionResponse,
	isChatCompleted,
	latestAskUserQuestionToolId,
	askUserQuestionResponseTextByToolId,
	hasUserResponseAfterAskQuestion = false,
	urlTransform,
}) => {
	const prefQuery = useQuery(preferenceSettings());
	const thinkingDisplayMode: ThinkingDisplayMode =
		prefQuery.data?.thinking_display_mode || "auto";
	const shellToolDisplayMode: TypesGen.AgentDisplayMode =
		prefQuery.data?.shell_tool_display_mode || "always_collapsed";
	const codeDiffDisplayMode: TypesGen.AgentDisplayMode =
		prefQuery.data?.code_diff_display_mode || "auto";

	const toolByID = new Map(tools.map((tool) => [tool.id, tool]));
	const displayBlocks = groupSequentialReadFileBlocks(blocks, tools);

	// Pre-compute which tool IDs have a corresponding block so
	// we can render "remaining" (block-less) tools afterwards.
	const blockToolIDs = new Set(
		displayBlocks.flatMap((block) => {
			if (block.type === "tool") {
				return toolByID.has(block.id) || isStreaming ? [block.id] : [];
			}
			if (block.type === "tool-group") {
				return block.ids;
			}
			return [];
		}),
	);

	const remainingTools = tools.filter((tool) => !blockToolIDs.has(tool.id));

	// A thinking block is actively streaming only when it is the
	// very last block in the list. Once newer content arrives
	// (response, tool call, etc.) the thinking phase is over.
	const lastDisplayBlockIsThinking =
		displayBlocks.length > 0 &&
		displayBlocks[displayBlocks.length - 1].type === "thinking";

	return (
		<>
			{displayBlocks.map((block, index) => {
				switch (block.type) {
					case "response":
						return (
							<ResponseBlock
								key={`${keyPrefix}-response-${index}`}
								text={block.text}
								isStreaming={isStreaming}
								streamKey={keyPrefix}
								urlTransform={urlTransform}
							/>
						);
					case "thinking":
						return (
							<ReasoningDisclosure
								key={`${keyPrefix}-thinking-${index}`}
								id={`${keyPrefix}-thinking-${index}`}
								text={block.text}
								isStreaming={
									isStreaming &&
									lastDisplayBlockIsThinking &&
									index === displayBlocks.length - 1
								}
								urlTransform={urlTransform}
								thinkingDisplayMode={thinkingDisplayMode}
							/>
						);
					case "file-reference":
						return (
							<div
								key={`${keyPrefix}-file-reference-${index}`}
								className="my-1 flex items-start gap-2 rounded-md border border-content-link/20 bg-content-link/5 px-2.5 py-1.5"
							>
								<span className="shrink-0 text-xs font-medium text-content-link">
									{block.file_name}:
									{block.start_line === block.end_line
										? block.start_line
										: `${block.start_line}\u2013${block.end_line}`}
								</span>
							</div>
						);
					case "tool-group": {
						const [firstGroupTool, ...restGroupTools] = block.ids
							.map((id) => toolByID.get(id))
							.filter((tool) => tool !== undefined);
						if (!firstGroupTool) {
							return null;
						}
						return (
							<ReadFileTimelineBlock
								key={firstGroupTool.id}
								tools={[firstGroupTool, ...restGroupTools]}
							/>
						);
					}
					case "tool": {
						const tool = toolByID.get(block.id);
						if (!tool) {
							if (!isStreaming) {
								return null;
							}
							// Streaming placeholder for not-yet-resolved tool.
							return (
								<Tool
									organizationId={organizationId}
									key={block.id}
									name="Tool"
									status="running"
									isError={false}
									shellToolDisplayMode={shellToolDisplayMode}
									codeDiffDisplayMode={codeDiffDisplayMode}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									subagentStatusOverrides={subagentStatusOverrides}
									mcpServers={mcpServers}
								/>
							);
						}
						if (tool.name === "read_file") {
							return <ReadFileTimelineBlock key={tool.id} tools={[tool]} />;
						}
						return (
							<Tool
								organizationId={organizationId}
								key={tool.id}
								name={tool.name}
								args={tool.args}
								result={tool.result}
								status={tool.status}
								isError={tool.isError}
								killedBySignal={tool.killedBySignal}
								shellToolDisplayMode={shellToolDisplayMode}
								codeDiffDisplayMode={codeDiffDisplayMode}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								showDesktopPreviews={showDesktopPreviews}
								subagentStatusOverrides={
									isStreaming ? subagentStatusOverrides : undefined
								}
								mcpServerConfigId={tool.mcpServerConfigId}
								mcpServers={mcpServers}
								onImplementPlan={onImplementPlan}
								onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
								isChatCompleted={isChatCompleted}
								isLatestAskUserQuestion={
									tool.id === latestAskUserQuestionToolId &&
									!hasUserResponseAfterAskQuestion
								}
								previousResponseText={
									tool.name === "ask_user_question"
										? askUserQuestionResponseTextByToolId?.get(tool.id)
										: undefined
								}
								modelIntent={tool.modelIntent}
								parsedCommands={tool.parsedCommands}
								hookRewritten={tool.hookRewritten}
							/>
						);
					}
					case "file":
						return (
							<AttachmentBlock
								key={`${keyPrefix}-file-${block.file_id ?? index}`}
								block={block}
								onImageClick={onImageClick}
								onTextFileClick={onTextFileClick}
								framePreview
								showTextStatus
							/>
						);
					case "sources":
						return (
							<WebSearchSources
								key={`${keyPrefix}-sources-${index}`}
								sources={block.sources}
							/>
						);
					default: {
						const _exhaustive: never = block;
						return _exhaustive;
					}
				}
			})}
			{remainingTools.map((tool) => (
				<Tool
					organizationId={organizationId}
					key={tool.id}
					name={tool.name}
					args={tool.args}
					result={tool.result}
					status={tool.status}
					isError={tool.isError}
					killedBySignal={tool.killedBySignal}
					shellToolDisplayMode={shellToolDisplayMode}
					codeDiffDisplayMode={codeDiffDisplayMode}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					showDesktopPreviews={showDesktopPreviews}
					subagentStatusOverrides={
						isStreaming ? subagentStatusOverrides : undefined
					}
					mcpServerConfigId={tool.mcpServerConfigId}
					mcpServers={mcpServers}
					onImplementPlan={onImplementPlan}
					onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
					isChatCompleted={isChatCompleted}
					isLatestAskUserQuestion={
						tool.id === latestAskUserQuestionToolId &&
						!hasUserResponseAfterAskQuestion
					}
					previousResponseText={
						tool.name === "ask_user_question"
							? askUserQuestionResponseTextByToolId?.get(tool.id)
							: undefined
					}
					modelIntent={tool.modelIntent}
					parsedCommands={tool.parsedCommands}
					hookRewritten={tool.hookRewritten}
				/>
			))}
		</>
	);
};
