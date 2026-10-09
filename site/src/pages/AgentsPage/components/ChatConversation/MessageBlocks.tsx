import { memo, useState } from "react";
import { useQuery } from "react-query";
import type { UrlTransform } from "streamdown";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";
import { ReadFilesTool } from "../ChatElements/tools/ReadFilesTool";
import {
	getReadFileToolData,
	ReadFileTool,
} from "../ChatElements/tools/ReadFileTool";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { Tool } from "../ChatElements/tools/Tool";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import WebSearchSources from "../ChatElements/tools/WebSearchSources";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import { groupSequentialReadFileBlocks } from "./blockUtils";
import { ReasoningDisclosure, ResponseBlock } from "./MessageText";
import type { MergedTool, RenderBlock } from "./types";

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
	organizationId: string;
	blocks: readonly RenderBlock[];
	tools: readonly MergedTool[];
	keyPrefix: string;
	isStreaming: boolean;
	subagentTitles: Map<string, string>;
	subagentVariants: Map<string, SubagentVariant>;
	showDesktopPreviews?: boolean;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	mcpServers: readonly TypesGen.MCPServerConfig[];
	onImageClick: (src: string) => void;
	onTextFileClick: (attachment: PreviewTextAttachment) => void;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	latestAskUserQuestionToolId?: string;
	askUserQuestionResponseTextByToolId?: ReadonlyMap<string, string>;
	hasUserResponseAfterAskQuestion: boolean;
	urlTransform: UrlTransform;
};

// Shared block renderer for durable messages and the live assistant turn.
// Encapsulates the response / thinking / tool / file / sources switch so both
// consumers stay in sync. PascalCase so the React Compiler auto-memoizes every
// element inside.
export const BlockList: React.FC<BlockListProps> = ({
	organizationId,
	blocks,
	tools,
	keyPrefix,
	isStreaming,
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
	hasUserResponseAfterAskQuestion,
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
								reasoning={tool.reasoning}
								status={tool.status}
								isError={tool.isError}
								isMedia={tool.isMedia}
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
								startedAt={tool.startedAt}
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
							/>
						);
					case "sources":
						return (
							<WebSearchSources
								key={`${keyPrefix}-sources-${index}`}
								sources={block.sources}
							/>
						);
					// Workspace file references render through the user
					// message display state, not as timeline blocks.
					case "workspace-file-reference":
						return null;
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
					reasoning={tool.reasoning}
					status={tool.status}
					isError={tool.isError}
					isMedia={tool.isMedia}
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
					startedAt={tool.startedAt}
					hookRewritten={tool.hookRewritten}
				/>
			))}
		</>
	);
};
