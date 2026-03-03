import { useTheme } from "@emotion/react";
import { FileDiff, File as FileViewer } from "@pierre/diffs/react";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import type { ComponentPropsWithRef, FC } from "react";
import { memo } from "react";
import { cn } from "utils/cn";
import { ChatSummarizedTool } from "./ChatSummarizedTool";
import { CreateWorkspaceTool } from "./CreateWorkspaceTool";
import { EditFilesTool } from "./EditFilesTool";
import {
	ExecuteAuthRequiredTool,
	ExecuteTool as ExecuteToolComponent,
	WaitForExternalAuthTool,
} from "./ExecuteTool";
import { ListTemplatesTool } from "./ListTemplatesTool";
import { ReadFileTool } from "./ReadFileTool";
import { ReadTemplateTool } from "./ReadTemplateTool";
import { SubagentTool } from "./SubagentTool";
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
	/** Maps sub-agent chat IDs to their titles, built from spawn tool results. */
	subagentTitles?: Map<string, string>;
	/** Maps sub-agent chat IDs to real-time status updates from stream events. */
	subagentStatusOverrides?: Map<string, string>;
}

// Props passed to each tool-specific renderer function. Each renderer
// only computes the expensive values it needs from the raw args/result.
type ToolRendererProps = {
	name: string;
	status: ToolStatus;
	args: unknown;
	result: unknown;
	isError: boolean;
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, string>;
};

// ---------------------------------------------------------------------------
// Tool-specific renderer functions
// ---------------------------------------------------------------------------

const ExecuteRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
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
	const prompt = parsedArgs ? asString(parsedArgs.prompt) : "";
	const subagentMessage = parsedArgs ? asString(parsedArgs.message) : "";
	const title =
		(rec ? asString(rec.title) : "") ||
		(parsedArgs ? asString(parsedArgs.title) : "") ||
		(chatId && subagentTitles?.get(chatId)) ||
		"Sub-agent";
	const subagentCompleted = isSubagentSuccessStatus(subagentStatus);
	const subagentToolStatus = mapSubagentStatusToToolStatus(
		subagentStatus,
		status,
	);
	const subagentIsError =
		subagentToolStatus === "error" ||
		((status === "error" || isError) && !subagentCompleted);

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

// Generic fallback renderer — only path that needs theme, diff
// viewers, and file content helpers.
const GenericToolRenderer: FC<ToolRendererProps> = ({
	name,
	status,
	args,
	result,
	isError,
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

	return (
		<>
			<div className="flex items-center gap-2">
				<ToolIcon name={name} isError={status === "error" || isError} />
				<ToolLabel name={name} args={args} result={result} />
			</div>
			{writeFileDiff ? (
				<ScrollArea
					className="mt-1.5 ml-6 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileDiff
						fileDiff={writeFileDiff}
						options={getDiffViewerOptions(isDark)}
					/>
				</ScrollArea>
			) : fileContent ? (
				<ScrollArea
					className="mt-1.5 ml-6 rounded-md border border-solid border-border-default text-2xs"
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
						className="mt-1.5 ml-6 rounded-md border border-solid border-border-default text-2xs"
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
		</>
	);
};

// ---------------------------------------------------------------------------
// Renderer lookup map — maps tool names to their specialized renderers.
// ---------------------------------------------------------------------------

const toolRenderers: Record<string, FC<ToolRendererProps>> = {
	execute: ExecuteRenderer,
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
	chat_summarized: ChatSummarizedRenderer,
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
		subagentTitles,
		subagentStatusOverrides,
		ref,
		...props
	}: ToolProps) => {
		const Renderer = toolRenderers[name] ?? GenericToolRenderer;

		return (
			<div
				ref={ref}
				className={cn(
					name === "execute" ? "w-full py-0.5" : "py-0.5",
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
					subagentTitles={subagentTitles}
					subagentStatusOverrides={subagentStatusOverrides}
				/>
			</div>
		);
	},
);

Tool.displayName = "Tool";
