import { FileDiff, File as FileViewer } from "@pierre/diffs/react";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import type { ComponentPropsWithRef } from "react";
import { memo } from "react";
import { cn } from "utils/cn";
import { AgentReportTool } from "./AgentReportTool";
import { ChatSummarizedTool } from "./ChatSummarizedTool";
import { CreateWorkspaceTool } from "./CreateWorkspaceTool";
import { EditFilesTool } from "./EditFilesTool";
import {
	ExecuteAuthRequiredTool,
	ExecuteTool as ExecuteToolComponent,
	WaitForExternalAuthTool,
} from "./ExecuteTool";
import { ReadFileTool } from "./ReadFileTool";
import { SubagentTool } from "./SubagentTool";
import { ToolIcon } from "./ToolIcon";
import { ToolLabel } from "./ToolLabel";
import {
	asNumber,
	asRecord,
	asString,
	buildEditDiff,
	DIFF_VIEWER_OPTIONS,
	DIFFS_FONT_STYLE,
	FILE_VIEWER_OPTIONS,
	FILE_VIEWER_OPTIONS_NO_HEADER,
	formatResultOutput,
	getFileContentForViewer,
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
		const parsedArgs = parseArgs(args);
		const resultOutput = formatResultOutput(result);
		const fileContent = getFileContentForViewer(name, args, result);
		const writeFileDiff = getWriteFileDiff(name, args);
		const editFiles = name === "edit_files" ? parseEditFilesArgs(args) : [];
		const editDiffs =
			name === "edit_files"
				? editFiles.map((file) => buildEditDiff(file.path, file.edits))
				: [];
		const fileContentOptions = fileContent
			? {
					...FILE_VIEWER_OPTIONS,
					disableFileHeader: fileContent.disableHeader,
					disableLineNumbers: fileContent.disableLineNumbers,
				}
			: FILE_VIEWER_OPTIONS;

		// Render execute tools with the specialized terminal-style block.
		if (name === "execute") {
			const parsed = parsedArgs;
			const command = parsed ? asString(parsed.command) : "";
			const rec = asRecord(result);
			const output = rec ? asString(rec.output).trim() : "";
			const authRequired = rec ? Boolean(rec.auth_required) : false;
			const authenticateURL = rec ? asString(rec.authenticate_url).trim() : "";
			const providerLabel = toProviderLabel(
				rec ? asString(rec.provider_display_name).trim() : "",
				rec ? asString(rec.provider_id).trim() : "",
				rec ? asString(rec.provider_type).trim() : "",
			);

			return (
				<div ref={ref} className={cn("w-full py-0.5", className)} {...props}>
					{authRequired && authenticateURL ? (
						<ExecuteAuthRequiredTool
							command={command}
							output={output}
							authenticateURL={authenticateURL}
							providerLabel={providerLabel}
						/>
					) : (
						<ExecuteToolComponent
							command={command}
							output={output}
							status={status}
							isError={isError}
						/>
					)}
				</div>
			);
		}

		if (name === "wait_for_external_auth") {
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
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<WaitForExternalAuthTool
						providerLabel={providerLabel}
						status={status}
						authenticated={authenticated}
						timedOut={timedOut}
						isError={isError}
						errorMessage={errorMessage || undefined}
					/>
				</div>
			);
		}

		// Render read_file with a collapsed-by-default viewer.
		if (name === "read_file") {
			const parsed = parsedArgs;
			const path = parsed ? asString(parsed.path).trim() : "";
			const rec = asRecord(result);
			const content = rec ? asString(rec.content).trim() : "";

			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<ReadFileTool
						path={path || "file"}
						content={content}
						status={status}
						isError={isError}
						errorMessage={rec ? asString(rec.error || rec.message) : undefined}
					/>
				</div>
			);
		}

		// Render write_file with a collapsed-by-default diff viewer.
		if (name === "write_file") {
			const parsed = parsedArgs;
			const path = parsed ? asString(parsed.path).trim() : "";
			const rec = asRecord(result);

			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<WriteFileTool
						path={path || "file"}
						diff={writeFileDiff}
						status={status}
						isError={isError}
						errorMessage={rec ? asString(rec.error || rec.message) : undefined}
					/>
				</div>
			);
		}

		// Render edit_files with a collapsed-by-default diff viewer.
		if (name === "edit_files") {
			const rec = asRecord(result);

			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<EditFilesTool
						files={editFiles}
						diffs={editDiffs}
						status={status}
						isError={isError}
						errorMessage={rec ? asString(rec.error || rec.message) : undefined}
					/>
				</div>
			);
		}

		// Render create_workspace with a collapsed-by-default viewer.
		// During workspace creation, build logs stream as result_delta
		// strings. Once the tool finishes, the result becomes a JSON
		// object with workspace metadata.
		if (name === "create_workspace") {
			const rec = asRecord(result);
			const buildLogs = typeof result === "string" ? result.trim() : "";
			const wsName = rec ? asString(rec.workspace_name) : "";
			const resultJson = rec ? JSON.stringify(rec, null, 2) : "";

			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<CreateWorkspaceTool
						workspaceName={wsName}
						resultJson={resultJson}
						buildLogs={buildLogs}
						status={status}
						isError={isError}
						errorMessage={rec ? asString(rec.error || rec.reason) : undefined}
					/>
				</div>
			);
		}

		// Render delegated sub-agent tools as a sub-agent link card.
		if (
			name === "subagent" ||
			name === "subagent_await" ||
			name === "subagent_message" ||
			name === "subagent_terminate"
		) {
			const parsed = parsedArgs;
			const rec = asRecord(result);
			// subagent_await and subagent_message have chat_id in
			// args, so check both result and args.
			const chatId =
				(rec ? asString(rec.chat_id) : "") ||
				(parsed ? asString(parsed.chat_id) : "");
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
			const prompt = parsed ? asString(parsed.prompt) : "";
			const subagentMessage = parsed ? asString(parsed.message) : "";
			const title =
				(rec ? asString(rec.title) : "") ||
				(parsed ? asString(parsed.title) : "") ||
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

			if (chatId) {
				return (
					<div ref={ref} className={cn("py-0.5", className)} {...props}>
						<SubagentTool
							toolName={name}
							title={title}
							chatId={chatId}
							subagentStatus={subagentStatus}
							prompt={prompt || undefined}
							message={subagentMessage || undefined}
							durationMs={durationMs}
							report={report || undefined}
							toolStatus={subagentToolStatus}
							isError={subagentIsError}
						/>
					</div>
				);
			}

			// No chat_id yet — the sub-agent is still being spawned.
			// Show a pending row with the title (and expandable
			// prompt) instead of falling through to the generic
			// "wrench" rendering.
			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<SubagentTool
						toolName={name}
						title={title}
						chatId=""
						subagentStatus={subagentStatus}
						prompt={prompt || undefined}
						message={subagentMessage || undefined}
						toolStatus={subagentToolStatus}
						isError={subagentIsError}
					/>
				</div>
			);
		}

		// Render subagent_report as a collapsible markdown report.
		if (name === "subagent_report") {
			const parsed = parsedArgs;
			const rec = asRecord(result);
			const report =
				(parsed ? asString(parsed.report) : "") ||
				(rec ? asString(rec.report) : "");
			const title = (rec ? asString(rec.title) : "") || "Sub-agent report";

			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<AgentReportTool
						title={title}
						report={report}
						toolStatus={status}
						isError={isError}
					/>
				</div>
			);
		}

		if (name === "chat_summarized") {
			const rec = asRecord(result);
			const summary =
				(rec ? asString(rec.summary) : "") ||
				(typeof result === "string" ? result : "");
			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<ChatSummarizedTool
						summary={summary}
						status={status}
						isError={isError}
						errorMessage={rec ? asString(rec.error || rec.message) : undefined}
					/>
				</div>
			);
		}

		return (
			<div ref={ref} className={cn("py-0.5", className)} {...props}>
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
						<FileDiff fileDiff={writeFileDiff} options={DIFF_VIEWER_OPTIONS} />
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
								options={FILE_VIEWER_OPTIONS_NO_HEADER}
								style={DIFFS_FONT_STYLE}
							/>
						</ScrollArea>
					)
				)}
			</div>
		);
	},
);

Tool.displayName = "Tool";
