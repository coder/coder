import { useTheme } from "@emotion/react";
import { File as FileViewer } from "@pierre/diffs/react";
import { type ComponentPropsWithRef, type FC, memo } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { cn } from "#/utils/cn";
import { AdvisorTool, type AdvisorToolResultType } from "./AdvisorTool";
import {
	type AskUserQuestion,
	AskUserQuestionTool,
} from "./AskUserQuestionTool";
import { ChatSummarizedTool } from "./ChatSummarizedTool";
import { ComputerTool } from "./ComputerTool";
import { CreateWorkspaceTool } from "./CreateWorkspaceTool";
import { DiffFileHeader } from "./DiffFileHeader";
import { EditFilesTool } from "./EditFilesTool";
import {
	ExecuteAuthRequiredTool,
	ExecuteTool as ExecuteToolComponent,
	WaitForExternalAuthTool,
} from "./ExecuteTool";
import { ListAgentsTool } from "./ListAgentsTool";
import { ListTemplatesTool } from "./ListTemplatesTool";
import { ProcessOutputTool } from "./ProcessOutputTool";
import { ProposePlanTool } from "./ProposePlanTool";
import { getReadFileToolData, ReadFileTool } from "./ReadFileTool";
import { ReadSkillTool } from "./ReadSkillTool";
import { ReadTemplateTool } from "./ReadTemplateTool";
import { StartWorkspaceTool } from "./StartWorkspaceTool";
import { SubagentTool } from "./SubagentTool";
import {
	getProvidedSubagentTitle,
	getSubagentChatId,
	getSubagentDescriptor,
	isSubagentToolName,
	type SubagentVariant,
} from "./subagentDescriptor";
import { ToolCall } from "./ToolCall";
import { ToolLabel } from "./ToolLabel";
import { getExecuteRenderData, shouldRenderTool } from "./toolVisibility";
import {
	asNumber,
	asRecord,
	asString,
	buildEditDiff,
	DIFFS_FONT_STYLE,
	formatModelIntentLabel,
	formatResultOutput,
	formatToolInput,
	getFileContentForViewer,
	getFileViewerOptions,
	getFileViewerOptionsNoHeader,
	getWriteFileDiff,
	humanizeMCPToolName,
	isSubagentSuccessStatus,
	mapSubagentStatusToToolStatus,
	parseArgs,
	parseEditFilesArgs,
	parseServerEditDiffText,
	parseServerEditResults,
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
	/** Maps sub-agent chat IDs to their titles, built from transcript metadata. */
	subagentTitles?: Map<string, string>;
	/** Maps sub-agent chat IDs to their normalized variants. */
	subagentVariants?: Map<string, SubagentVariant>;
	/** When false, suppresses inline VNC previews while still
	 * allowing the MonitorIcon variant to render. */
	showDesktopPreviews?: boolean;
	/** Maps sub-agent chat IDs to real-time status updates from stream events. */
	subagentStatusOverrides?: Map<string, string>;
	/** MCP server config ID associated with this tool call. */
	mcpServerConfigId?: string;
	/** Available MCP server configs for icon/name lookup. */
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	isLatestAskUserQuestion?: boolean;
	previousResponseText?: string;
	/** Human-readable intent extracted from the model's tool-call args. */
	modelIntent?: string;
	/** Parsed command tuples ([program] or [program, arg]) for execute tool calls. */
	parsedCommands?: readonly string[][];
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
	codeDiffDisplayMode?: TypesGen.AgentDisplayMode;
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
	subagentVariants?: Map<string, SubagentVariant>;
	showDesktopPreviews?: boolean;
	subagentStatusOverrides?: Map<string, string>;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	isLatestAskUserQuestion?: boolean;
	previousResponseText?: string;
	mcpServerConfigId?: string;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	shellToolDisplayMode?: TypesGen.AgentDisplayMode;
	codeDiffDisplayMode?: TypesGen.AgentDisplayMode;
};

// ---------------------------------------------------------------------------
// Tool-specific renderer functions
// ---------------------------------------------------------------------------

const parseAskUserQuestionOptions = (
	value: unknown,
): AskUserQuestion["options"] | null => {
	if (!Array.isArray(value)) {
		return null;
	}

	const options: AskUserQuestion["options"] = [];
	for (const option of value) {
		const optionRecord = asRecord(option);
		if (!optionRecord) {
			continue;
		}

		options.push({
			label: asString(optionRecord.label).trim(),
			description: asString(optionRecord.description).trim(),
		});
	}

	return options;
};

const parseAskUserQuestions = (value: unknown): AskUserQuestion[] | null => {
	if (!Array.isArray(value)) {
		return null;
	}

	const questions: AskUserQuestion[] = [];
	for (const question of value) {
		const questionRecord = asRecord(question);
		if (!questionRecord) {
			continue;
		}

		questions.push({
			header: asString(questionRecord.header).trim(),
			question: asString(questionRecord.question).trim(),
			options: parseAskUserQuestionOptions(questionRecord.options) ?? [],
		});
	}

	return questions;
};

const insufficientQuotaErrorCode = "INSUFFICIENT_QUOTA";

const getWorkspaceQuotaTitle = (
	rec: Record<string, unknown> | null,
): string | undefined => {
	if (!rec || asString(rec.error_code) !== insufficientQuotaErrorCode) {
		return undefined;
	}
	return asString(rec.title).trim() || "Workspace quota reached";
};

const parseAskUserQuestionResult = (
	result: unknown,
): AskUserQuestion[] | null => {
	const parsedResult = parseArgs(result);
	const directQuestions = parsedResult
		? parseAskUserQuestions(parsedResult.questions)
		: null;
	if (directQuestions) {
		return directQuestions;
	}

	const resultRecord = asRecord(result);
	if (!resultRecord) {
		return null;
	}

	for (const value of [
		resultRecord.output,
		resultRecord.content,
		resultRecord.text,
	]) {
		const parsedValue = parseArgs(value);
		const questions = parsedValue
			? parseAskUserQuestions(parsedValue.questions)
			: null;
		if (questions) {
			return questions;
		}
	}

	return null;
};

const ExecuteRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
	killedBySignal,
	modelIntent,
	parsedCommands,
	shellToolDisplayMode,
}) => {
	const data = getExecuteRenderData(args, result);
	const outputBlock = data.transcriptBlocks.find(
		(block) => block.kind === "output",
	);

	if (data.authenticateURL) {
		return (
			<ExecuteAuthRequiredTool
				command={data.command}
				output={outputBlock?.text ?? ""}
				authenticateURL={data.authenticateURL}
				providerLabel={data.providerLabel}
			/>
		);
	}
	return (
		<ExecuteToolComponent
			command={data.command}
			transcriptBlocks={data.transcriptBlocks}
			status={status}
			isError={isError}
			durationMs={data.durationMs}
			isBackgrounded={data.isBackgrounded}
			killedBySignal={killedBySignal}
			modelIntent={modelIntent}
			parsedCommands={parsedCommands}
			shellToolDisplayMode={shellToolDisplayMode}
		/>
	);
};

const ProcessOutputRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
	killedBySignal,
	shellToolDisplayMode,
}) => {
	const rec = asRecord(result);
	const output = rec ? asString(rec.output).trim() : "";
	const exitCode = rec
		? (asNumber(rec.exit_code, { parseString: true }) ?? null)
		: null;
	const errorMessage = rec ? asString(rec.error || rec.message) : "";

	return (
		<ProcessOutputTool
			output={output}
			isRunning={status === "running"}
			exitCode={exitCode}
			isError={isError}
			errorMessage={errorMessage || undefined}
			killedBySignal={killedBySignal}
			shellToolDisplayMode={shellToolDisplayMode}
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
}) => (
	<ReadFileTool
		{...getReadFileToolData({ args, result, isError })}
		status={status}
	/>
);

const ReadSkillRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
}) => {
	const parsedArgs = parseArgs(args);
	const skillName = parsedArgs ? asString(parsedArgs.name) : "";
	const rec = asRecord(result);
	const body = rec ? asString(rec.body) : "";

	return (
		<ReadSkillTool
			label={skillName ? `skill ${skillName}` : "skill"}
			body={body}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
		/>
	);
};

const ReadSkillFileRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
}) => {
	const parsedArgs = parseArgs(args);
	const skillName = parsedArgs ? asString(parsedArgs.name) : "";
	const filePath = parsedArgs ? asString(parsedArgs.path) : "";
	const label =
		skillName && filePath
			? `${skillName}/${filePath}`
			: skillName || filePath || "skill file";
	const rec = asRecord(result);
	const content = rec ? asString(rec.content) : "";

	return (
		<ReadSkillTool
			label={label}
			body={content}
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
	codeDiffDisplayMode,
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
			codeDiffDisplayMode={codeDiffDisplayMode}
		/>
	);
};

const EditFilesRenderer: FC<ToolRendererProps> = ({
	status,
	args,
	result,
	isError,
	codeDiffDisplayMode,
}) => {
	const rec = asRecord(result);
	const editFiles = parseEditFilesArgs(args);
	// On error, render no diff: the agent rejected the edit, so a
	// synthetic args-derived diff would misrepresent it as applied.
	const serverResults = parseServerEditResults(result);
	const editDiffs = isError
		? editFiles.map(() => null)
		: editFiles.map((file) => {
				const entry = serverResults?.find((d) => d.path === file.path);
				return entry
					? parseServerEditDiffText(entry.diff)
					: buildEditDiff(file.path, file.edits);
			});

	return (
		<EditFilesTool
			files={editFiles}
			diffs={editDiffs}
			status={status}
			isError={isError}
			errorMessage={rec ? asString(rec.error || rec.message) : undefined}
			codeDiffDisplayMode={codeDiffDisplayMode}
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
	const buildId = rec ? asString(rec.build_id) : undefined;
	const resultJson = rec ? JSON.stringify(rec, null, 2) : "";
	const hasErrorInResult = Boolean(rec?.error);
	const created = rec?.created !== false;
	const quotaTitle = getWorkspaceQuotaTitle(rec);

	return (
		<CreateWorkspaceTool
			workspaceName={wsName}
			resultJson={resultJson}
			status={status}
			isError={isError || hasErrorInResult}
			errorMessage={rec ? asString(rec.error || rec.reason) : undefined}
			buildId={buildId}
			created={created}
			labelOverride={quotaTitle}
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
	subagentVariants,
	showDesktopPreviews = true,
	subagentStatusOverrides,
}) => {
	const parsedArgs = parseArgs(args);
	const rec = asRecord(result);
	const chatId = getSubagentChatId({
		args: parsedArgs ?? args,
		result: rec ?? result,
	});
	const inferredVariant = chatId ? subagentVariants?.get(chatId) : undefined;
	const descriptor = getSubagentDescriptor({
		name,
		args: parsedArgs ?? args,
		result: rec ?? result,
		inferredVariant,
	});
	if (!descriptor) {
		return null;
	}

	const resultSubagentStatus = rec
		? asString(rec.status || rec.subagent_status)
		: "";
	let streamSubagentStatus = "";
	if (chatId) {
		streamSubagentStatus = subagentStatusOverrides?.get(chatId) || "";
	}
	const subagentStatus = streamSubagentStatus || resultSubagentStatus;
	const report = rec ? asString(rec.report) : "";
	const recordingFileId = rec ? asString(rec.recording_file_id) : "";
	const thumbnailFileId = rec ? asString(rec.thumbnail_file_id) : "";
	const prompt = parsedArgs ? asString(parsedArgs.prompt) : "";
	const subagentMessage = parsedArgs ? asString(parsedArgs.message) : "";
	const rawTitle = getProvidedSubagentTitle({
		args: parsedArgs ?? args,
		result: rec ?? result,
	});
	let title =
		descriptor.fallbackTitle.charAt(0).toUpperCase() +
		descriptor.fallbackTitle.slice(1);
	if (chatId) {
		const mappedTitle = subagentTitles?.get(chatId);
		if (mappedTitle) {
			title = mappedTitle;
		}
	}
	if (rawTitle) {
		title = rawTitle;
	}
	const subagentCompleted = isSubagentSuccessStatus(subagentStatus);
	const subagentToolStatus = mapSubagentStatusToToolStatus(
		subagentStatus,
		status,
	);
	let subagentIsError = subagentToolStatus === "error";
	if (!subagentIsError) {
		const toolFailed = status === "error" || isError;
		if (toolFailed && !subagentCompleted) {
			subagentIsError = true;
		}
	}

	// Detect timeout from the result. A timed-out wait_agent
	// returns a structured payload with timed_out: true
	// (IsError=false), or an error string containing "timed out".
	const resultStr = typeof result === "string" ? result : "";
	const errorStr = rec ? asString(rec.error) : "";
	let isTimeout = false;
	if (rec && rec.timed_out === true) {
		isTimeout = true;
	} else if (subagentIsError) {
		const timedOutInResult = resultStr.toLowerCase().includes("timed out");
		const timedOutInError = errorStr.toLowerCase().includes("timed out");
		if (timedOutInResult || timedOutInError) {
			isTimeout = true;
		}
	}

	return (
		<SubagentTool
			descriptor={descriptor}
			title={title}
			chatId={chatId}
			subagentStatus={subagentStatus}
			prompt={prompt || undefined}
			message={subagentMessage || undefined}
			report={chatId ? report || undefined : undefined}
			toolStatus={subagentToolStatus}
			isError={subagentIsError}
			isTimeout={isTimeout}
			showDesktopPreview={
				Boolean(chatId) &&
				showDesktopPreviews &&
				descriptor.supportsDesktopAffordance
			}
			recordingFileId={recordingFileId || undefined}
			thumbnailFileId={thumbnailFileId || undefined}
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

const ListAgentsRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const agents = rec && Array.isArray(rec.agents) ? rec.agents : [];
	const total = rec
		? (asNumber(rec.total, { parseString: true }) ?? agents.length)
		: 0;

	return (
		<ListAgentsTool
			agents={agents}
			total={total}
			status={status}
			isError={isError}
			errorMessage={
				rec
					? asString(rec.error || rec.message)
					: typeof result === "string" && isError
						? result
						: undefined
			}
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

const AskUserQuestionRenderer: FC<ToolRendererProps> = ({
	args,
	status,
	result,
	isError,
	onSendAskUserQuestionResponse,
	isChatCompleted,
	isLatestAskUserQuestion,
	previousResponseText,
}) => {
	const parsedArgs = parseArgs(args);
	const questionsFromArgs = parsedArgs
		? parseAskUserQuestions(parsedArgs.questions)
		: null;
	const questionsFromResult = parseAskUserQuestionResult(result);
	const questions =
		questionsFromArgs && questionsFromArgs.length > 0
			? questionsFromArgs
			: questionsFromResult && questionsFromResult.length > 0
				? questionsFromResult
				: (questionsFromArgs ?? questionsFromResult ?? []);
	const resultRecord = asRecord(result);
	const errorMessage =
		(resultRecord
			? asString(resultRecord.error || resultRecord.message)
			: "") || (typeof result === "string" && isError ? result : "");

	return (
		<AskUserQuestionTool
			questions={questions}
			status={status}
			isError={isError}
			errorMessage={errorMessage || undefined}
			onSubmitAnswer={onSendAskUserQuestionResponse}
			isChatCompleted={isChatCompleted}
			isLatestAskUserQuestion={isLatestAskUserQuestion}
			previousResponseText={previousResponseText}
		/>
	);
};

const ProposePlanRenderer: FC<ToolRendererProps> = ({
	args,
	status,
	result,
	isError,
	onImplementPlan,
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
			onImplementPlan={onImplementPlan}
		/>
	);
};

const AdvisorRenderer: FC<ToolRendererProps> = ({
	args,
	status,
	result,
	isError,
}) => {
	const parsedArgs = parseArgs(args);
	const question = parsedArgs ? asString(parsedArgs.question) : "";
	const rec = asRecord(result);
	const rawResultType = rec ? asString(rec.type) : "";
	const hasError = status === "error" || isError;
	const advice = rec
		? asString(rec.advice)
		: typeof result === "string" && !hasError
			? result
			: undefined;
	const adviceText = (advice ?? "").trim();
	const resolvedResultType: AdvisorToolResultType | undefined =
		rawResultType === "advice" ||
		rawResultType === "limit_reached" ||
		rawResultType === "error"
			? rawResultType
			: adviceText
				? "advice"
				: undefined;
	const errorMessage =
		(rec ? asString(rec.error || rec.message) : "") ||
		(typeof result === "string" && (hasError || resolvedResultType === "error")
			? result
			: "");
	const advisorModel = rec ? asString(rec.advisor_model) : "";
	const remainingUses = rec
		? asNumber(rec.remaining_uses, { parseString: true })
		: undefined;

	return (
		<AdvisorTool
			question={question}
			status={status}
			isError={hasError}
			resultType={resolvedResultType}
			advice={advice}
			errorMessage={errorMessage || undefined}
			advisorModel={advisorModel || undefined}
			remainingUses={remainingUses}
		/>
	);
};

const ComputerRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	let imageData = "";
	let mimeType = "image/png";
	let text = "";
	let attachmentFileId = "";
	let attachmentName = "";

	if (Array.isArray(result)) {
		for (const block of result) {
			const blockRec = asRecord(block);
			if (!blockRec) {
				continue;
			}
			if (blockRec.type === "image" || asString(blockRec.data)) {
				imageData = asString(blockRec.data);
				mimeType = asString(blockRec.mime_type) || "image/png";
			}
			if (blockRec.type === "text" || (!imageData && asString(blockRec.text))) {
				text = asString(blockRec.text);
			}
			if (!attachmentFileId) {
				attachmentFileId = asString(blockRec.attachment_file_id);
			}
			if (!attachmentName) {
				attachmentName = asString(blockRec.attachment_name);
			}
		}
	} else {
		const rec = asRecord(result);
		if (rec) {
			imageData = asString(rec.data);
			mimeType = asString(rec.mime_type) || "image/png";
			text = asString(rec.text);
			attachmentFileId = asString(rec.attachment_file_id);
			attachmentName = asString(rec.attachment_name);
		}
	}

	if (attachmentFileId) {
		imageData = "";
		if (!text) {
			text = attachmentName
				? `Attached ${attachmentName}`
				: "Attached screenshot";
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

type ToolFileViewerProps = {
	label?: string;
	file: ComponentPropsWithRef<typeof FileViewer>["file"];
	options: ComponentPropsWithRef<typeof FileViewer>["options"];
};

const ToolFileViewer: FC<ToolFileViewerProps> = ({ label, file, options }) => (
	<>
		{label && (
			<div className="mt-2 text-2xs font-medium text-content-secondary">
				{label}
			</div>
		)}
		<ScrollArea
			className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
			viewportClassName="max-h-64"
			orientation="both"
			scrollBarClassName="w-1.5"
			horizontalScrollBarClassName="h-1.5"
		>
			<FileViewer
				file={file}
				options={options}
				style={DIFFS_FONT_STYLE}
				renderCustomHeader={
					options?.disableFileHeader
						? undefined
						: (file) => <DiffFileHeader file={file} />
				}
			/>
		</ScrollArea>
	</>
);

type GenericToolContentProps = {
	toolInput: string | null;
	fileContent: ReturnType<typeof getFileContentForViewer>;
	fileContentOptions: ComponentPropsWithRef<typeof FileViewer>["options"];
	isDark: boolean;
	resultOutput: string | null;
};

const GenericToolContent: FC<GenericToolContentProps> = ({
	toolInput,
	fileContent,
	fileContentOptions,
	isDark,
	resultOutput,
}) => {
	const output = fileContent
		? {
				file: { name: fileContent.path, contents: fileContent.content },
				options: fileContentOptions,
			}
		: resultOutput
			? {
					file: { name: "output.json", contents: resultOutput },
					options: getFileViewerOptionsNoHeader(isDark),
				}
			: undefined;

	return (
		<>
			{toolInput && (
				<ToolFileViewer
					label="Input"
					file={{ name: "input.json", contents: toolInput }}
					options={getFileViewerOptionsNoHeader(isDark)}
				/>
			)}
			{output && (
				<ToolFileViewer
					label={toolInput ? "Output" : undefined}
					file={output.file}
					options={output.options}
				/>
			)}
		</>
	);
};

const getGenericToolErrorMessage = ({
	name,
	mcpSlug,
}: {
	name: string;
	mcpSlug?: string;
}): string => {
	const displayName = humanizeMCPToolName(mcpSlug ?? "", name);
	return `${displayName} failed`;
};

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
	const toolInput = formatToolInput(args);
	const resultOutput = formatResultOutput(result);
	const fileContent = getFileContentForViewer(name, args, result);
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

	const hasContent = Boolean(toolInput || fileContent || resultOutput);
	const rec = asRecord(result);
	const errorMessage = rec ? asString(rec.error || rec.message) : "";
	const fallbackErrorMessage = getGenericToolErrorMessage({
		name,
		mcpSlug: mcpServer?.slug,
	});

	return (
		<ToolCall.Root
			status={status}
			isError={isError}
			errorMessage={errorMessage || fallbackErrorMessage}
			hasContent={hasContent}
		>
			<ToolCall.Header
				iconName={name}
				iconUrl={mcpServer?.icon_url}
				serverName={mcpServer?.display_name}
				label={
					modelIntent ? (
						formatModelIntentLabel(modelIntent)
					) : (
						<ToolLabel
							name={name}
							args={args}
							result={result}
							mcpSlug={mcpServer?.slug}
						/>
					)
				}
			/>
			<ToolCall.Content>
				<GenericToolContent
					toolInput={toolInput}
					fileContent={fileContent}
					fileContentOptions={fileContentOptions}
					isDark={isDark}
					resultOutput={resultOutput}
				/>
			</ToolCall.Content>
		</ToolCall.Root>
	);
};

// ---------------------------------------------------------------------------
// process_signal promotes soft failures (success=false
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

const StartWorkspaceRenderer: FC<ToolRendererProps> = ({
	status,
	result,
	isError,
}) => {
	const rec = asRecord(result);
	const wsName = rec ? asString(rec.workspace_name) : "";
	const buildId = rec ? asString(rec.build_id) : undefined;
	const hasErrorInResult = Boolean(rec?.error);
	const noBuild = Boolean(rec?.no_build);
	const quotaTitle = getWorkspaceQuotaTitle(rec);

	return (
		<StartWorkspaceTool
			status={status}
			buildId={buildId}
			workspaceName={wsName}
			isError={isError || hasErrorInResult}
			errorMessage={rec ? asString(rec.error || rec.reason) : undefined}
			noBuild={noBuild}
			labelOverride={quotaTitle}
		/>
	);
};

// ---------------------------------------------------------------------------
// Renderer lookup map for tool names and specialized renderers.
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
	start_workspace: StartWorkspaceRenderer,
	list_templates: ListTemplatesRenderer,
	list_agents: ListAgentsRenderer,
	read_template: ReadTemplateRenderer,
	read_skill: ReadSkillRenderer,
	read_skill_file: ReadSkillFileRenderer,
	chat_summarized: ChatSummarizedRenderer,
	ask_user_question: AskUserQuestionRenderer,
	propose_plan: ProposePlanRenderer,
	advisor: AdvisorRenderer,
	computer: ComputerRenderer,
};

// ---------------------------------------------------------------------------
// Public Tool component with a single wrapper div and map dispatch.
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
		subagentVariants,
		showDesktopPreviews,
		subagentStatusOverrides,
		mcpServerConfigId,
		mcpServers,
		onImplementPlan,
		onSendAskUserQuestionResponse,
		isChatCompleted,
		isLatestAskUserQuestion,
		previousResponseText,
		modelIntent,
		parsedCommands,
		shellToolDisplayMode,
		codeDiffDisplayMode,
		ref,
		...props
	}: ToolProps) => {
		const Renderer =
			isSubagentToolName(name) && name !== "list_agents"
				? SubagentRenderer
				: (toolRenderers[name] ?? GenericToolRenderer);
		const isShellTool = name === "execute" || name === "process_output";
		if (!shouldRenderTool({ name, status, args, result })) {
			return null;
		}

		return (
			<div
				ref={ref}
				data-transcript-row=""
				className={cn(
					isShellTool || name === "propose_plan" || name === "advisor"
						? "w-full"
						: undefined,
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
					subagentVariants={subagentVariants}
					showDesktopPreviews={showDesktopPreviews}
					subagentStatusOverrides={subagentStatusOverrides}
					mcpServerConfigId={mcpServerConfigId}
					mcpServers={mcpServers}
					onImplementPlan={onImplementPlan}
					onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
					isChatCompleted={isChatCompleted}
					isLatestAskUserQuestion={isLatestAskUserQuestion}
					previousResponseText={previousResponseText}
					modelIntent={modelIntent}
					parsedCommands={parsedCommands}
					shellToolDisplayMode={shellToolDisplayMode}
					codeDiffDisplayMode={codeDiffDisplayMode}
				/>
			</div>
		);
	},
);
