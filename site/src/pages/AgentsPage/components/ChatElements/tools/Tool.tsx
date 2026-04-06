import { useTheme } from "@emotion/react";
import { FileDiff, File as FileViewer } from "@pierre/diffs/react";
import { LoaderIcon, TriangleAlertIcon } from "lucide-react";
import { type ComponentPropsWithRef, type FC, memo } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { ChatSummarizedTool } from "./ChatSummarizedTool";
import { ComputerTool } from "./ComputerTool";
import { CreateWorkspaceTool } from "./CreateWorkspaceTool";
import { EditFilesTool } from "./EditFilesTool";
import {
	ExecuteAuthRequiredTool,
	ExecuteTool as ExecuteToolComponent,
	WaitForExternalAuthTool,
} from "./ExecuteTool";
import { ListTemplatesTool } from "./ListTemplatesTool";
import { ProcessOutputTool } from "./ProcessOutputTool";
import { ProposePlanTool } from "./ProposePlanTool";
import { ReadFileTool } from "./ReadFileTool";
import { ReadTemplateTool } from "./ReadTemplateTool";
import { SubagentTool } from "./SubagentTool";
import { ToolCollapsible } from "./ToolCollapsible";
import { ToolIcon } from "./ToolIcon";
import { ToolLabel } from "./ToolLabel";
import {
	asNumber,
	asRecord,
	asString,
	buildEditDiff,
	DIFFS_FONT_STYLE,
	formatResultOutput,
	getDiffViewerOptions,
	getFileContentForViewer,
	getFileViewerOptions,
	getFileViewerOptionsNoHeader,
	getWriteFileDiff,
	isSubagentSuccessStatus,
	mapSubagentStatusToToolStatus,
	parseArgs,
	parseEditFilesArgs,
	stripNoNewline,
	type ToolStatus,
	toProviderLabel,
} from "./utils";
import { WriteFileTool } from "./WriteFileTool";

interface ToolProps extends Omit<ComponentPropsWithRef<"div">, "children"> {
	name: string;
	status?: ToolStatus;
	args?: unknown;
	result?: unknown;
	isError?: boolean;
	killedBySignal?: "kill" | "terminate";
	/** Maps sub-agent chat IDs to their titles, built from spawn tool results. */
	subagentTitles?: Map<string, string>;
	/** Set of chat IDs spawned by `spawn_computer_use_agent`. */
	computerUseSubagentIds?: Set<string>;
	/** When false, suppresses inline VNC previews while still
	 * allowing the MonitorIcon variant to render. */
	showDesktopPreviews?: boolean;
	/** Maps sub-agent chat IDs to real-time status updates from stream events. */
	subagentStatusOverrides?: Map<string, string>;
	/** MCP server config ID associated with this tool call. */
	mcpServerConfigId?: string;
	/** Available MCP server configs for icon/name lookup. */
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	/** Human-readable intent extracted from the model's tool-call args. */
	modelIntent?: string;
}

// Props passed to each tool-specific renderer function. Each renderer
// only computes the expensive values it needs from the raw args/result.
type ToolRendererProps = {
	name: string;
	status: ToolStatus;
	args: unknown;
	result: unknown;
	isError: boolean;
	killedBySignal?: "kill" | "terminate";
	subagentTitles?: Map<string, string>;
	computerUseSubagentIds?: Set<string>;
	showDesktopPreviews?: boolean;
	subagentStatusOverrides?: Map<string, string>;
	mcpServerConfigId?: string;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	modelIntent?: string;
};

// ---------------------------------------------------------------------------
// Tool-specific renderer functions
// ---------------------------------------------------------------------------

const ExecuteRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
	killedBySignal,
}) => {
	const parsedArgs = parseArgs(args);
	const command = parsedArgs ? asString(parsedArgs.command) : "";
	const rec = asRecord(result);
	const output = rec ? asString(rec.output).trim() : "";
	const authRequired = rec ? Boolean(rec.auth_required) : false;
	const authenticateURL = rec ? asString(rec.authenticate_url).trim() : "";
	const providerLabel = toProviderLabel(
		rec ? asString(rec.provider_display_name).trim() : "",
		rec ? asString(rec.provider_id).trim() : "",
		rec ? asString(rec.provider_type).trim() : "",
	);

	if (authRequired && authenticateURL) {
		return (
			<ExecuteAuthRequiredTool
				command={command}
				output={output}
				authenticateURL={authenticateURL}
				providerLabel={providerLabel}
			/>
		);
	}
	return (
		<ExecuteToolComponent
			command={command}
			output={output}
			status={status}
			isError={isError}
			killedBySignal={killedBySignal}
		/>
	);
};

const ProcessOutputRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
	killedBySignal,
}) => {
	const rec = asRecord(result);
	const output = rec ? asString(rec.output).trim() : "";
	const exitCode = rec
		? rec.exit_code !== undefined && rec.exit_code !== null
			? Number(rec.exit_code)
			: null
		: null;

	return (
		<ProcessOutputTool
			output={output}
			isRunning={status === "running"}
			exitCode={exitCode}
			isError={isError}
			killedBySignal={killedBySignal}
		/>
	);
};

const WaitForExternalAuthRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const providerLabel = toProviderLabel(
		rec ? asString(rec.provider_display_name).trim() : "",
		rec ? asString(rec.provider_id).trim() : "",
		rec ? asString(rec.provider_type).trim() : "",
	);
	const authenticated = rec ? Boolean(rec.authenticated) : false;
	const timedOut = rec ? Boolean(rec.timed_out) : false;
	const errorMessage = rec ? asString(rec.error || rec.message) : "";

	return (
		<WaitForExternalAuthTool
			providerLabel={providerLabel}
			status={status}
			authenticated={authenticated}
			timedOut={timedOut}
			isError={isError}
			errorMessage={errorMessage || undefined}
		/>
	);
};

const ReadFileRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
}) => {
	const parsedArgs = parseArgs(args);
	const path = parsedArgs ? asString(parsedArgs.path).trim() : "";
	const rec = asRecord(result);
	const content = rec ? asString(rec.content).trim() : "";

	return (
		<ReadFileTool
			path={path || "file"}
			content={content}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

const WriteFileRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
}) => {
	const parsedArgs = parseArgs(args);
	const path = parsedArgs ? asString(parsedArgs.path).trim() : "";
	const rec = asRecord(result);
	const writeFileDiff = getWriteFileDiff("write_file", args);

	return (
		<WriteFileTool
			path={path || "file"}
			diff={writeFileDiff}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

const EditFilesRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const editFiles = parseEditFilesArgs(args);
	const editDiffs = editFiles.map((file) =>
		buildEditDiff(file.path, file.edits),
	);

	return (
		<EditFilesTool
			files={editFiles}
			diffs={editDiffs}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

// Once the tool finishes, the result becomes a JSON object
// with workspace metadata.
const CreateWorkspaceRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const wsName = rec ? asString(rec.workspace_name) : "";
	const resultJson = rec ? JSON.stringify(rec, null, 2) : "";

	return (
		<CreateWorkspaceTool
			workspaceName={wsName}
			resultJson={resultJson}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.reason) : undefined}
		/>
	);
};

const SubagentRenderer: FC<ToolRendererProps> = ({
	name,
	status,
	args,
	result,
	isError,
	subagentTitles,
	computerUseSubagentIds,
	showDesktopPreviews = true,
	subagentStatusOverrides,
}) => {
	const parsedArgs = parseArgs(args);
	const rec = asRecord(result);
	// wait_agent and message_agent have chat_id in args, so
	// check both result and args.
	const chatId =
		(rec ? asString(rec.chat_id) : "") ||
		(parsedArgs ? asString(parsedArgs.chat_id) : "");
	const resultSubagentStatus = rec
		? asString(rec.status || rec.subagent_status)
		: "";
	const streamSubagentStatus =
		(chatId && subagentStatusOverrides?.get(chatId)) || "";
	const subagentStatus = streamSubagentStatus || resultSubagentStatus;
	const durationMs = rec
		? asNumber(rec.duration_ms, { parseString: true })
		: undefined;
	const report = rec ? asString(rec.report) : "";
	const recordingFileId = rec ? asString(rec.recording_file_id) : "";
	const prompt = parsedArgs ? asString(parsedArgs.prompt) : "";
	const subagentMessage = parsedArgs ? asString(parsedArgs.message) : "";
	const title =
		(rec ? asString(rec.title) : "") ||
		(parsedArgs ? asString(parsedArgs.title) : "") ||
		(chatId && subagentTitles?.get(chatId)) ||
		(name === "spawn_computer_use_agent"
			? "Computer use sub-agent"
			: "Sub-agent");
	const subagentCompleted = isSubagentSuccessStatus(subagentStatus);
	const subagentToolStatus = mapSubagentStatusToToolStatus(
		subagentStatus,
		status,
	);
	const subagentIsError =
		subagentToolStatus === "error" ||
		((status === "error" || isError) && !subagentCompleted);

	// Detect timeout from the result. A timed-out wait_agent
	// typically returns an error string or an object with an
	// error field containing "timed out".
	const resultStr = typeof result === "string" ? result : "";
	const errorStr = rec ? asString(rec.error) : "";
	const isTimeout =
		subagentIsError &&
		(resultStr.toLowerCase().includes("timed out") ||
			errorStr.toLowerCase().includes("timed out"));

	// Postpone rendering wait_agent / message_agent until the
	// chat_id has been parsed from the streaming args. Without it
	// we can't determine variant or title, which causes a brief
	// flash of the generic "Waiting for Sub-agent" text.
	if (
		!chatId &&
		status === "running" &&
		(name === "wait_agent" || name === "message_agent")
	) {
		return null;
	}

	const variant =
		name === "spawn_computer_use_agent" || computerUseSubagentIds?.has(chatId)
			? "computer-use"
			: "default";
	return (
		<SubagentTool
			toolName={name}
			title={title}
			chatId={chatId}
			subagentStatus={subagentStatus}
			prompt={prompt || undefined}
			message={subagentMessage || undefined}
			durationMs={chatId ? durationMs : undefined}
			report={chatId ? report || undefined : undefined}
			toolStatus={subagentToolStatus}
			isError={subagentIsError}
			isTimeout={isTimeout}
			showDesktopPreview={
				showDesktopPreviews && computerUseSubagentIds?.has(chatId)
			}
			variant={variant}
			recordingFileId={recordingFileId || undefined}
		/>
	);
};

const ListTemplatesRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const templates = rec && Array.isArray(rec.templates) ? rec.templates : [];
	const count = rec
		? (asNumber(rec.count, { parseString: true }) ?? templates.length)
		: 0;

	return (
		<ListTemplatesTool
			templates={templates}
			count={count}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

const ReadTemplateRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const templateRec = rec ? asRecord(rec.template) : undefined;
	const name = templateRec
		? asString(templateRec.display_name) || asString(templateRec.name)
		: "";

	return (
		<ReadTemplateTool
			templateName={name}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

const ChatSummarizedRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const summary =
		(rec ? asString(rec.summary) : "") ||
		(typeof result === "string" ? result : "");

	return (
		<ChatSummarizedTool
			summary={summary}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

const ProposePlanRenderer: FC<ToolRendererProps> = ({
	args,
	status,
	result,
	isError,
}) => {
	const parsedArgs = parseArgs(args);
	const path = parsedArgs ? asString(parsedArgs.path) || "PLAN.md" : "PLAN.md";
	const rec = asRecord(result);
	const content = rec && "content" in rec ? asString(rec.content) : undefined;
	const fileID = rec && "file_id" in rec ? asString(rec.file_id) : undefined;
	const errorMessage = isError
		? (rec ? asString(rec.error || rec.message) : undefined) ||
			(typeof result === "string" ? result : undefined)
		: undefined;

	return (
		<ProposePlanTool
			content={content}
			fileID={fileID}
			path={path}
			status={status}
			isError={isError}
			errorMessage={errorMessage}
		/>
	);
};

const ComputerRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	// The result can be a single object with {data, text, mime_type}
	// or an array of content blocks.
	let imageData = "";
	let mimeType = "image/png";
	let text = "";

	if (Array.isArray(result)) {
		for (const block of result) {
			const blockRec = asRecord(block);
			if (blockRec) {
				if (blockRec.type === "image" || asString(blockRec.data)) {
					imageData = asString(blockRec.data);
					mimeType = asString(blockRec.mime_type) || "image/png";
				}
				if (
					blockRec.type === "text" ||
					(!imageData && asString(blockRec.text))
				) {
					text = asString(blockRec.text);
				}
			}
		}
	} else {
		const rec = asRecord(result);
		if (rec) {
			imageData = asString(rec.data);
			mimeType = asString(rec.mime_type) || "image/png";
			text = asString(rec.text);
		}
	}

	return (
		<ComputerTool
			imageData={imageData}
			mimeType={mimeType}
			text={text}
			status={status}
			isError={isError}
		/>
	);
};

// Generic fallback renderer — only path that needs theme, diff
// viewers, and file content helpers.
const GenericToolRenderer: FC<ToolRendererProps> = ({
	name,
	status,
	args,
	result,
	isError,
	mcpServerConfigId,
	mcpServers,
	modelIntent,
}) => {
	const theme = useTheme();
	const isDark = theme.palette.mode === "dark";
	const resultOutput = formatResultOutput(result);
	const fileContent = getFileContentForViewer(name, args, result);
	const writeFileDiff = getWriteFileDiff(name, args);
	const fileViewerOpts = getFileViewerOptions(isDark);
	const fileContentOptions = fileContent
		? {
				...fileViewerOpts,
				disableFileHeader: fileContent.disableHeader,
				disableLineNumbers: fileContent.disableLineNumbers,
			}
		: fileViewerOpts;

	// Look up MCP server config for icon and slug.
	const mcpServer = mcpServerConfigId
		? mcpServers?.find((s) => s.id === mcpServerConfigId)
		: undefined;

	const hasContent = Boolean(writeFileDiff || fileContent || resultOutput);
	const isRunning = status === "running";
	const rec = asRecord(result);
	const errorMessage = rec ? asString(rec.error || rec.message) : "";

	return (
		<ToolCollapsible
			hasContent={hasContent}
			header={
				<>
					<ToolIcon
						name={name}
						isError={status === "error" || isError}
						iconUrl={mcpServer?.icon_url}
						isRunning={isRunning}
						serverName={mcpServer?.display_name}
					/>
					{modelIntent ? (
						<span className="truncate text-sm text-content-secondary">
							{modelIntent.charAt(0).toUpperCase() + modelIntent.slice(1)}
						</span>
					) : (
						<ToolLabel
							name={name}
							args={args}
							result={result}
							mcpSlug={mcpServer?.slug}
						/>
					)}
					{isError && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
							</TooltipTrigger>
							<TooltipContent>
								{errorMessage || "Tool call failed"}
							</TooltipContent>
						</Tooltip>
					)}
					{isRunning && (
						<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin motion-reduce:animate-none text-content-secondary" />
					)}
				</>
			}
		>
			{writeFileDiff ? (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileDiff
						fileDiff={stripNoNewline(writeFileDiff)}
						options={getDiffViewerOptions(isDark)}
						style={DIFFS_FONT_STYLE}
					/>
				</ScrollArea>
			) : fileContent ? (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileViewer
						file={{
							name: fileContent.path,
							contents: fileContent.content,
						}}
						options={fileContentOptions}
					/>
				</ScrollArea>
			) : (
				resultOutput && (
					<ScrollArea
						className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
						viewportClassName="max-h-64"
						scrollBarClassName="w-1.5"
					>
						<FileViewer
							file={{
								name: "output.json",
								contents: resultOutput,
							}}
							options={getFileViewerOptionsNoHeader(isDark)}
							style={DIFFS_FONT_STYLE}
						/>
					</ScrollArea>
				)
			)}
		</ToolCollapsible>
	);
};

// ---------------------------------------------------------------------------
// process_signal — thin wrapper that promotes soft failures (success=false
// in the result body, isError=false at protocol level) so the generic
// renderer shows the error indicator and tooltip.
// ---------------------------------------------------------------------------

const ProcessSignalRenderer: FC<ToolRendererProps> = (props) => {
	const rec = asRecord(props.result);
	const isSoftFailure =
		!props.isError &&
		props.status !== "running" &&
		rec !== null &&
		!rec.success;
	return (
		<GenericToolRenderer {...props} isError={props.isError || isSoftFailure} />
	);
};

// ---------------------------------------------------------------------------
// Renderer lookup map — maps tool names to their specialized renderers.
// ---------------------------------------------------------------------------

const toolRenderers: Record<string, FC<ToolRendererProps>> = {
	execute: ExecuteRenderer,
	process_output: ProcessOutputRenderer,
	process_signal: ProcessSignalRenderer,
	wait_for_external_auth: WaitForExternalAuthRenderer,
	read_file: ReadFileRenderer,
	write_file: WriteFileRenderer,
	edit_files: EditFilesRenderer,
	create_workspace: CreateWorkspaceRenderer,
	list_templates: ListTemplatesRenderer,
	read_template: ReadTemplateRenderer,
	spawn_agent: SubagentRenderer,
	wait_agent: SubagentRenderer,
	message_agent: SubagentRenderer,
	close_agent: SubagentRenderer,
	spawn_computer_use_agent: SubagentRenderer,
	chat_summarized: ChatSummarizedRenderer,
	propose_plan: ProposePlanRenderer,
	computer: ComputerRenderer,
};

// ---------------------------------------------------------------------------
// Public Tool component — single wrapper div + map dispatch.
// ---------------------------------------------------------------------------

export const Tool = memo(
	({
		className,
		name,
		status = "completed",
		args,
		result,
		isError = false,
		killedBySignal,
		subagentTitles,
		computerUseSubagentIds,
		showDesktopPreviews,
		subagentStatusOverrides,
		mcpServerConfigId,
		mcpServers,
		modelIntent,
		ref,
		...props
	}: ToolProps) => {
		const Renderer = toolRenderers[name] ?? GenericToolRenderer;

		return (
			<div
				ref={ref}
				className={cn(
					name === "execute" ||
						name === "process_output" ||
						name === "propose_plan"
						? "w-full py-0.5"
						: "py-0.5",
					className,
				)}
				{...props}
			>
				<Renderer
					name={name}
					status={status}
					args={args}
					result={result}
					isError={isError}
					killedBySignal={killedBySignal}
					subagentTitles={subagentTitles}
					computerUseSubagentIds={computerUseSubagentIds}
					showDesktopPreviews={showDesktopPreviews}
					subagentStatusOverrides={subagentStatusOverrides}
					mcpServerConfigId={mcpServerConfigId}
					mcpServers={mcpServers}
					modelIntent={modelIntent}
				/>
			</div>
		);
	},
);
