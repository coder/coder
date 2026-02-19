import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import { FileDiff, File as FileViewer } from "@pierre/diffs/react";
import { CopyButton } from "components/CopyButton/CopyButton";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	BotIcon,
	CheckIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	CircleAlertIcon,
	ExternalLinkIcon,
	FileIcon,
	FilePenIcon,
	LoaderIcon,
	PlusCircleIcon,
	TerminalIcon,
	WrenchIcon,
} from "lucide-react";
import { forwardRef, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { cn } from "utils/cn";
import { Response } from "./response";

type ToolStatus = "completed" | "error" | "running";

interface ToolProps
	extends Omit<React.HTMLAttributes<HTMLDivElement>, "children"> {
	name: string;
	status?: ToolStatus;
	args?: unknown;
	result?: unknown;
	isError?: boolean;
}

const asRecord = (value: unknown): Record<string, unknown> | null => {
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return null;
	}
	return value as Record<string, unknown>;
};

const asString = (value: unknown): string =>
	typeof value === "string" ? value : "";

const asNumber = (value: unknown): number | undefined => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}
	if (typeof value === "string") {
		const parsed = Number(value);
		if (Number.isFinite(parsed)) {
			return parsed;
		}
	}
	return undefined;
};

/**
 * Formats a duration in milliseconds into a compact label using
 * the same style as {@link shortRelativeTime} in utils/time.
 */
const shortDurationMs = (durationMs: number | undefined): string => {
	if (durationMs === undefined || durationMs < 0) {
		return "";
	}
	const seconds = Math.round(durationMs / 1000);
	if (seconds < 60) {
		return `${seconds}s`;
	}
	const minutes = Math.round(seconds / 60);
	if (minutes < 60) {
		return `${minutes}m`;
	}
	const hours = Math.round(minutes / 60);
	return `${hours}h`;
};

const normalizeStatus = (status: string): string =>
	status.trim().toLowerCase();

const isSubagentSuccessStatus = (status: string): boolean => {
	switch (normalizeStatus(status)) {
		case "completed":
		case "reported":
			return true;
		default:
			return false;
	}
};

const isSubagentRunningStatus = (status: string): boolean => {
	switch (normalizeStatus(status)) {
		case "pending":
		case "running":
		case "awaiting":
			return true;
		default:
			return false;
	}
};

const parseArgs = (args: unknown): Record<string, unknown> | null => {
	if (!args) {
		return null;
	}
	if (typeof args === "string") {
		try {
			const parsed = JSON.parse(args);
			return asRecord(parsed);
		} catch {
			return null;
		}
	}
	return asRecord(args);
};

const ToolIcon: React.FC<{ name: string; isError: boolean }> = ({
	name,
	isError,
}) => {
	const color = isError ? "text-content-destructive" : "text-content-secondary";
	const base = `h-4 w-4 shrink-0 ${color}`;
	switch (name) {
		case "execute":
			return <TerminalIcon className={base} />;
		case "read_file":
			return <FileIcon className={base} />;
		case "write_file":
		case "edit_files":
			return <FilePenIcon className={base} />;
		case "create_workspace":
			return <PlusCircleIcon className={base} />;
		default:
			return <WrenchIcon className={base} />;
	}
};

const formatResultOutput = (result: unknown): string | null => {
	if (result === undefined || result === null) {
		return null;
	}
	if (typeof result === "string") {
		const trimmed = result.trim();
		return trimmed || null;
	}
	const rec = asRecord(result);
	if (rec) {
		// For execute tool, show the output field.
		const output = asString(rec.output).trim();
		if (output) {
			return output;
		}
		// For read_file, show the content field.
		const content = asString(rec.content).trim();
		if (content) {
			return content;
		}
	}
	if (typeof result === "object") {
		try {
			return JSON.stringify(result, null, 2);
		} catch {
			return String(result);
		}
	}
	return String(result);
};

const ToolLabel: React.FC<{ name: string; args: unknown; result: unknown }> = ({
	name,
	args,
	result,
}) => {
	const parsed = parseArgs(args);
	const parsedResult = asRecord(result);

	switch (name) {
		case "execute": {
			const command = parsed ? asString(parsed.command) : "";
			if (command) {
				return (
					<code className="truncate font-mono text-xs text-content-primary">
						{command}
					</code>
				);
			}
			return (
				<span className="truncate text-sm text-content-secondary">
					Running command
				</span>
			);
		}
		case "read_file":
			return (
				<span className="truncate text-sm text-content-secondary">
					Reading file…
				</span>
			);
		case "write_file": {
			const path = parsed ? asString(parsed.path) : "";
			if (path) {
				return (
					<code className="truncate font-mono text-xs text-content-primary">
						{path}
					</code>
				);
			}
			return (
				<span className="truncate text-sm text-content-secondary">
					Writing file
				</span>
			);
		}
		case "edit_files": {
			const files = parsed?.files;
			if (Array.isArray(files) && files.length === 1) {
				const path = asString((files[0] as Record<string, unknown>)?.path);
				if (path) {
					return (
						<code className="truncate font-mono text-xs text-content-primary">
							{path}
						</code>
					);
				}
			}
			return (
				<span className="truncate text-sm text-content-secondary">
					Editing files
				</span>
			);
		}
		case "create_workspace": {
			const wsName = parsedResult ? asString(parsedResult.workspace_name) : "";
			if (wsName) {
				return (
					<span className="truncate text-sm text-content-secondary">
						Created {wsName}
					</span>
				);
			}
			return (
				<span className="truncate text-sm text-content-secondary">
					Creating workspace
				</span>
			);
		}
		case "subagent_terminate":
			return (
				<span className="truncate text-sm text-content-secondary">
					Terminated sub-agent
				</span>
			);
		case "subagent": {
			const parsed = parseArgs(args);
			const title = parsed ? asString(parsed.title) : "";
			return (
				<span className="truncate text-sm text-content-secondary">
					{title || "Delegating to sub-agent…"}
				</span>
			);
		}
		case "subagent_await":
			return (
				<span className="truncate text-sm text-content-secondary">
					Awaiting sub-agent…
				</span>
			);
		case "subagent_message":
			return (
				<span className="truncate text-sm text-content-secondary">
					Messaging sub-agent…
				</span>
			);
		case "subagent_report":
			return (
				<span className="truncate text-sm text-content-secondary">
					Sub-agent report
				</span>
			);
		default:
			return (
				<span className="truncate text-sm text-content-secondary">{name}</span>
			);
	}
};

const fileViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; }";

const diffViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; } [data-diffs-header] { border-left: 1px solid var(--border); }";

/**
 * Checks whether a tool result should be rendered as a syntax-highlighted
 * file viewer. Returns the file path, content, and whether the header
 * should be hidden.
 */
const getFileContentForViewer = (
	toolName: string,
	args: unknown,
	result: unknown,
): {
	path: string;
	content: string;
	disableHeader?: boolean;
	disableLineNumbers?: boolean;
} | null => {
	if (toolName === "execute") {
		const rec = asRecord(result);
		if (!rec) {
			return null;
		}
		const output = asString(rec.output).trim();
		if (!output) {
			return null;
		}
		return {
			path: "output.sh",
			content: output,
			disableHeader: true,
			disableLineNumbers: true,
		};
	}
	if (toolName !== "read_file") {
		return null;
	}
	const parsed = parseArgs(args);
	const path = parsed ? asString(parsed.path).trim() : "";
	if (!path) {
		return null;
	}
	const rec = asRecord(result);
	if (!rec) {
		return null;
	}
	const content = asString(rec.content).trim();
	if (!content) {
		return null;
	}
	return { path, content };
};

/**
 * Builds a FileDiffMetadata representing a new-file diff (all lines
 * are additions) from the content written by a write_file tool call.
 * Returns null when the content is empty or unparseable.
 */
const buildWriteFileDiff = (
	path: string,
	content: string,
): FileDiffMetadata | null => {
	const lines = content.split("\n");
	// Remove trailing empty line produced by a final newline.
	if (lines.length > 0 && lines[lines.length - 1] === "") {
		lines.pop();
	}
	if (lines.length === 0) {
		return null;
	}

	const patchLines = [
		`diff --git a/${path} b/${path}`,
		"new file mode 100644",
		"--- /dev/null",
		`+++ b/${path}`,
		`@@ -0,0 +1,${lines.length} @@`,
		...lines.map((l) => `+${l}`),
	];
	const patch = `${patchLines.join("\n")}\n`;

	const parsed = parsePatchFiles(patch);
	if (!parsed.length || !parsed[0].files.length) {
		return null;
	}
	return parsed[0].files[0];
};

/**
 * For write_file tool calls, extracts the path and content from args
 * and builds a FileDiffMetadata showing all lines as additions.
 */
const getWriteFileDiff = (
	toolName: string,
	args: unknown,
): FileDiffMetadata | null => {
	if (toolName !== "write_file") {
		return null;
	}
	const parsed = parseArgs(args);
	if (!parsed) {
		return null;
	}
	const path = asString(parsed.path).trim();
	const content = asString(parsed.content).trim();
	if (!path || !content) {
		return null;
	}
	return buildWriteFileDiff(path, content);
};

/** Height that fits roughly 3 lines of monospace text-xs output. */
const COLLAPSED_OUTPUT_HEIGHT = 54;

/**
 * Specialized rendering for `execute` tool calls. Shows the command
 * in a terminal-style block with a copy button. Output is shown in a
 * collapsed preview (~3 lines) with an expand chevron at the bottom.
 */
const ExecuteTool: React.FC<{
	command: string;
	output: string;
	status: ToolStatus;
	isError: boolean;
}> = ({ command, output, status, isError }) => {
	const [expanded, setExpanded] = useState(false);
	const outputRef = useRef<HTMLPreElement>(null);
	const hasOutput = output.length > 0;
	const isRunning = status === "running";

	// Check whether the output overflows the collapsed height so we
	// know if we need to show the expand toggle at all.
	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLPreElement | null) => {
		(outputRef as React.MutableRefObject<HTMLPreElement | null>).current = node;
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_OUTPUT_HEIGHT);
		}
	};

	return (
		<div className="group/exec w-full overflow-hidden rounded-md border border-solid border-border-default bg-surface-primary">
			{/* Header: $ command + copy button */}
			<div className="flex items-center gap-2 px-2.5 py-0.5">
				<span className="shrink-0 font-mono text-xs text-content-secondary">
					$
				</span>
				<code className="min-w-0 flex-1 truncate font-mono text-xs text-content-primary">
					{command}
				</code>
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
				)}
				<span className="opacity-0 transition-opacity group-hover/exec:opacity-100">
					<CopyButton text={command} label="Copy command" />
				</span>
			</div>

			{/* Output preview / expanded */}
			{hasOutput && (
				<>
					<div
						className="h-px"
						style={{ background: "hsl(var(--border-default))" }}
					/>
					<ScrollArea
						className="text-2xs"
						viewportClassName={expanded ? "max-h-96" : ""}
						scrollBarClassName="w-1.5"
					>
						<pre
							ref={measureRef}
							style={
								expanded
									? undefined
									: { maxHeight: COLLAPSED_OUTPUT_HEIGHT, overflow: "hidden" }
							}
							className={cn(
								"m-0 border-0 whitespace-pre-wrap break-all bg-transparent px-2.5 py-2 font-mono text-xs",
								isError ? "text-content-destructive" : "text-content-secondary",
							)}
						>
							{output}
						</pre>
					</ScrollArea>

					{/* Expand / collapse toggle at the bottom */}
					{overflows && (
						<div
							role="button"
							tabIndex={0}
							onClick={() => setExpanded((v) => !v)}
							onKeyDown={(e) => {
								if (e.key === "Enter" || e.key === " ") {
									setExpanded((v) => !v);
								}
							}}
							className="flex w-full cursor-pointer items-center justify-center py-0.5 text-content-secondary transition-colors hover:bg-surface-secondary hover:text-content-primary"
							aria-label={expanded ? "Collapse output" : "Expand output"}
						>
							<ChevronDownIcon
								className={cn(
									"h-3 w-3 transition-transform",
									expanded && "rotate-180",
								)}
							/>
						</div>
					)}
				</>
			)}
		</div>
	);
};

/**
 * Collapsed-by-default rendering for `read_file` tool calls. Shows
 * "Read <filename>" with a chevron; expanding reveals the file viewer.
 */
const ReadFileTool: React.FC<{
	path: string;
	content: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ path, content, status, isError, errorMessage }) => {
	const [expanded, setExpanded] = useState(false);
	const hasContent = content.length > 0;
	const isRunning = status === "running";

	return (
		<div className="w-full">
			<div
				role="button"
				tabIndex={0}
				onClick={() => hasContent && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && hasContent) {
						setExpanded((v) => !v);
					}
				}}
				className={cn(
					"flex items-center gap-2",
					hasContent && "cursor-pointer",
				)}
			>
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					Read {path.split("/").pop() || path}
				</span>
				{isError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
						</TooltipTrigger>
						<TooltipContent>
							{errorMessage || "Failed to read file"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
				)}
				{hasContent && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>

			{expanded && hasContent && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileViewer
						file={{
							name: path,
							contents: content,
						}}
						options={{
							overflow: "scroll",
							disableLineNumbers: true,
							disableFileHeader: true,
							themeType: "dark",
							theme: "github-dark-high-contrast",
							unsafeCSS: fileViewerCSS,
						}}
						style={{
							"--diffs-font-size": "11px",
							"--diffs-line-height": "1.5",
						}}
					/>
				</ScrollArea>
			)}
		</div>
	);
};

/**
 * Collapsed-by-default rendering for `write_file` tool calls. Shows
 * "Wrote <filename>" with a chevron; expanding reveals the unified diff.
 */
const WriteFileTool: React.FC<{
	path: string;
	diff: FileDiffMetadata | null;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ path, diff, status, isError, errorMessage }) => {
	const [expanded, setExpanded] = useState(false);
	const hasDiff = diff !== null;
	const isRunning = status === "running";

	const filename = path.split("/").pop() || path;
	const label = isRunning ? `Writing ${filename}…` : `Wrote ${filename}`;

	return (
		<div className="w-full">
			<div
				role="button"
				tabIndex={0}
				onClick={() => hasDiff && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && hasDiff) {
						setExpanded((v) => !v);
					}
				}}
				className={cn("flex items-center gap-2", hasDiff && "cursor-pointer")}
			>
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{label}
				</span>
				{isError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
						</TooltipTrigger>
						<TooltipContent>
							{errorMessage || "Failed to write file"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
				)}
				{hasDiff && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>

			{expanded && hasDiff && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileDiff
						fileDiff={diff}
						options={{
							diffStyle: "unified",
							diffIndicators: "bars",
							overflow: "scroll",
							themeType: "dark",
							theme: "github-dark-high-contrast",
							unsafeCSS: diffViewerCSS,
						}}
						style={{
							"--diffs-font-size": "11px",
							"--diffs-line-height": "1.5",
						}}
					/>
				</ScrollArea>
			)}
		</div>
	);
};

interface EditFilesFileEntry {
	path: string;
	edits: Array<{ search: string; replace: string }>;
}

/**
 * Parses the args of an edit_files tool call into a typed array
 * of file entries.
 */
const parseEditFilesArgs = (args: unknown): EditFilesFileEntry[] => {
	const parsed = parseArgs(args);
	if (!parsed) return [];
	const files = parsed.files;
	if (!Array.isArray(files)) return [];
	return files.filter(
		(f): f is EditFilesFileEntry =>
			f !== null &&
			typeof f === "object" &&
			typeof (f as Record<string, unknown>).path === "string" &&
			Array.isArray((f as Record<string, unknown>).edits),
	);
};

/**
 * Builds a synthetic unified diff from search/replace edit pairs
 * for a single file. Each pair becomes a separate hunk in the
 * diff. Line numbers are synthetic since we don't have the full
 * file content.
 */
const buildEditDiff = (
	path: string,
	edits: Array<{ search: string; replace: string }>,
): FileDiffMetadata | null => {
	if (!edits.length) return null;

	// Strip leading slash so the a/ and b/ prefixes don't
	// produce a double-slash that confuses the diff parser.
	const diffPath = path.startsWith("/") ? path.slice(1) : path;

	const patchLines: string[] = [
		`diff --git a/${diffPath} b/${diffPath}`,
		`--- a/${diffPath}`,
		`+++ b/${diffPath}`,
	];

	let lineOffset = 1;
	for (const edit of edits) {
		const searchLines = edit.search.split("\n");
		const replaceLines = edit.replace.split("\n");

		// Remove trailing empty line produced by a final newline.
		if (searchLines.length > 0 && searchLines[searchLines.length - 1] === "") {
			searchLines.pop();
		}
		if (
			replaceLines.length > 0 &&
			replaceLines[replaceLines.length - 1] === ""
		) {
			replaceLines.pop();
		}
		if (searchLines.length === 0 && replaceLines.length === 0) continue;

		patchLines.push(
			`@@ -${lineOffset},${searchLines.length} +${lineOffset},${replaceLines.length} @@`,
		);
		for (const l of searchLines) patchLines.push(`-${l}`);
		for (const l of replaceLines) patchLines.push(`+${l}`);

		lineOffset += Math.max(searchLines.length, replaceLines.length) + 1;
	}

	const patch = `${patchLines.join("\n")}\n`;
	const parsed = parsePatchFiles(patch);
	if (!parsed.length || !parsed[0].files.length) return null;
	return parsed[0].files[0];
};

/**
 * Collapsed-by-default rendering for `edit_files` tool calls.
 * Shows "Edited <filename>" (or "Edited N files") with a chevron;
 * expanding reveals a unified diff for each file.
 */
const EditFilesTool: React.FC<{
	files: EditFilesFileEntry[];
	diffs: (FileDiffMetadata | null)[];
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({ files, diffs, status, isError, errorMessage }) => {
	const [expanded, setExpanded] = useState(true);
	const isRunning = status === "running";
	const hasDiffs = diffs.some((d) => d !== null);

	let label: string;
	if (isRunning) {
		label =
			files.length === 1
				? `Editing ${files[0].path.split("/").pop() || files[0].path}…`
				: `Editing ${files.length} files…`;
	} else if (files.length === 1) {
		const filename = files[0].path.split("/").pop() || files[0].path;
		label = `Edited ${filename}`;
	} else {
		label = `Edited ${files.length} files`;
	}

	return (
		<div className="w-full">
			<div
				role="button"
				tabIndex={0}
				onClick={() => hasDiffs && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && hasDiffs) {
						setExpanded((v) => !v);
					}
				}}
				className={cn("flex items-center gap-2", hasDiffs && "cursor-pointer")}
			>
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{label}
				</span>
				{isError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
						</TooltipTrigger>
						<TooltipContent>
							{errorMessage || "Failed to edit files"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
				)}
				{hasDiffs && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>

			{expanded && hasDiffs && (
				<div className="mt-1.5 space-y-1.5">
					{diffs.map((diff, i) =>
						diff ? (
							<ScrollArea
								key={files[i].path}
								className="rounded-md border border-solid border-border-default text-2xs"
								viewportClassName="max-h-64"
								scrollBarClassName="w-1.5"
							>
								<FileDiff
									fileDiff={diff}
									options={{
										diffStyle: "unified",
										diffIndicators: "bars",
										overflow: "scroll",
										themeType: "dark",
										theme: "github-dark-high-contrast",
										unsafeCSS: diffViewerCSS,
									}}
									style={{
										"--diffs-font-size": "11px",
										"--diffs-line-height": "1.5",
									}}
								/>
							</ScrollArea>
						) : null,
					)}
				</div>
			)}
		</div>
	);
};

/**
 * Collapsed-by-default rendering for `create_workspace` tool calls.
 *
 * While the workspace is being built, build logs stream in as
 * `result_delta` strings. Once complete the result becomes a JSON
 * object with workspace metadata. This component handles both:
 *   - **Building**: shows "Creating workspace…" with a spinner and
 *     live build-log output.
 *   - **Completed**: shows "Created <name>" collapsed, expandable
 *     to reveal the full result JSON.
 */
const CreateWorkspaceTool: React.FC<{
	workspaceName: string;
	resultJson: string;
	buildLogs: string;
	status: ToolStatus;
	isError: boolean;
	errorMessage?: string;
}> = ({
	workspaceName,
	resultJson,
	buildLogs,
	status,
	isError,
	errorMessage,
}) => {
	const [expanded, setExpanded] = useState(false);
	const isBuilding = buildLogs.length > 0 && resultJson.length === 0;
	const isRunning = status === "running" || isBuilding;
	const hasContent = resultJson.length > 0;

	const label = isRunning
		? "Creating workspace…"
		: workspaceName
			? `Created ${workspaceName}`
			: "Created workspace";

	return (
		<div className="w-full">
			<div
				role="button"
				tabIndex={0}
				onClick={() => hasContent && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && hasContent) {
						setExpanded((v) => !v);
					}
				}}
				className={cn(
					"flex items-center gap-2",
					hasContent && "cursor-pointer",
				)}
			>
				<span
					className={cn(
						"text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{label}
				</span>
				{isError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
						</TooltipTrigger>
						<TooltipContent>
							{errorMessage || "Failed to create workspace"}
						</TooltipContent>
					</Tooltip>
				)}
				{isRunning && (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
				)}
				{hasContent && !isBuilding && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>

			{/* Live build-log output while workspace is being created. */}
			{isBuilding && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-48"
					scrollBarClassName="w-1.5"
				>
					<pre className="m-0 whitespace-pre-wrap break-all border-0 bg-transparent px-2.5 py-2 font-mono text-xs text-content-secondary">
						{buildLogs}
					</pre>
				</ScrollArea>
			)}

			{/* Expandable JSON result once workspace creation completes. */}
			{expanded && hasContent && !isBuilding && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default text-2xs"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<FileViewer
						file={{
							name: "result.json",
							contents: resultJson,
						}}
						options={{
							overflow: "scroll",
							disableLineNumbers: true,
							disableFileHeader: true,
							themeType: "dark",
							theme: "github-dark-high-contrast",
							unsafeCSS: fileViewerCSS,
						}}
						style={{
							"--diffs-font-size": "11px",
							"--diffs-line-height": "1.5",
						}}
					/>
				</ScrollArea>
			)}
		</div>
	);
};

/** Height for the collapsed report preview (~3 lines of rendered markdown). */
const COLLAPSED_REPORT_HEIGHT = 72;

/**
 * Resolves a sub-agent status string and tool-level status into a
 * display icon. The sub-agent status in the tool result is a
 * snapshot from when the tool returned and may be stale (e.g. a
 * background sub-agent records "pending" forever). The icon is
 * therefore driven primarily by the tool-call status itself.
 */
const SubagentStatusIcon: React.FC<{
	subagentStatus: string;
	toolStatus: ToolStatus;
	isError: boolean;
}> = ({ subagentStatus, toolStatus, isError }) => {
	const subagentCompleted = isSubagentSuccessStatus(subagentStatus);
	if (isError && !subagentCompleted) {
		return (
			<CircleAlertIcon className="h-4 w-4 shrink-0 text-content-destructive" />
		);
	}
	if (toolStatus === "error") {
		return (
			<CircleAlertIcon className="h-4 w-4 shrink-0 text-content-destructive" />
		);
	}
	if (toolStatus === "running") {
		return (
			<LoaderIcon className="h-4 w-4 shrink-0 animate-spin text-content-link" />
		);
	}
	return <BotIcon className="h-4 w-4 shrink-0 text-content-secondary" />;
};

/**
 * Specialized rendering for delegated sub-agent tool calls.
 * Shows a clickable header row with the sub-agent title, status
 * icon, and a chevron to expand the prompt / report below. A
 * "View Agent" link navigates to the sub-agent chat.
 */
const SubagentTool: React.FC<{
	toolName: string;
	title: string;
	chatId: string;
	subagentStatus: string;
	prompt?: string;
	durationMs?: number;
	report?: string;
	toolStatus: ToolStatus;
	isError: boolean;
}> = ({
	toolName,
	title,
	chatId,
	subagentStatus,
	prompt,
	durationMs,
	report,
	toolStatus,
	isError,
}) => {
	const [expanded, setExpanded] = useState(false);
	const hasPrompt = Boolean(prompt?.trim());
	const hasReport = Boolean(report?.trim());
	const hasExpandableContent = hasPrompt || hasReport;
	const durationLabel = shortDurationMs(durationMs);

	return (
		<div className="w-full">
			<div
				role="button"
				tabIndex={0}
				onClick={() => hasExpandableContent && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if (
						(e.key === "Enter" || e.key === " ") &&
						hasExpandableContent
					) {
						setExpanded((v) => !v);
					}
				}}
				className={cn(
					"flex items-center gap-2",
					hasExpandableContent && "cursor-pointer",
				)}
			>
				<SubagentStatusIcon
					subagentStatus={subagentStatus}
					toolStatus={toolStatus}
					isError={isError}
				/>
				<span className="min-w-0 flex-1 truncate text-sm text-content-secondary">
					{toolName === "subagent" && toolStatus === "completed"
						? "Spawned "
						: toolName === "subagent_await" && toolStatus === "completed"
							? "Waited for "
							: ""}
					<span className="text-content-secondary opacity-60">{title}</span>
					{chatId && (
						<Link
							to={`/agents/${chatId}`}
							onClick={(e) => e.stopPropagation()}
							className="ml-1 inline-flex align-middle text-content-secondary opacity-50 transition-opacity hover:opacity-100"
							aria-label="View agent"
						>
							<ExternalLinkIcon className="h-3 w-3" />
						</Link>
					)}
				</span>
				{durationLabel && (
					<span className="shrink-0 text-xs text-content-secondary">
						Worked for {durationLabel}
					</span>
				)}
				{hasExpandableContent && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>

			{expanded && hasPrompt && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{prompt ?? ""}</Response>
					</div>
				</ScrollArea>
			)}

			{expanded && hasReport && (
				<ScrollArea
					className="mt-1.5 rounded-md border border-solid border-border-default"
					viewportClassName="max-h-64"
					scrollBarClassName="w-1.5"
				>
					<div className="px-3 py-2">
						<Response>{report ?? ""}</Response>
					</div>
				</ScrollArea>
			)}
		</div>
	);
};

/**
 * Specialized rendering for `subagent_report` tool calls. Shows the
 * report as collapsible markdown with a short preview that users
 * can expand.
 */
const AgentReportTool: React.FC<{
	title: string;
	report: string;
	toolStatus: ToolStatus;
	isError: boolean;
}> = ({ title, report, toolStatus, isError }) => {
	const [expanded, setExpanded] = useState(false);
	const contentRef = useRef<HTMLDivElement>(null);
	const [overflows, setOverflows] = useState(false);
	const measureRef = (node: HTMLDivElement | null) => {
		(contentRef as React.MutableRefObject<HTMLDivElement | null>).current =
			node;
		if (node) {
			setOverflows(node.scrollHeight > COLLAPSED_REPORT_HEIGHT);
		}
	};
	const isRunning = toolStatus === "running";

	return (
		<div className="w-full overflow-hidden rounded-lg border border-solid border-border-default bg-surface-primary">
			<div
				role="button"
				tabIndex={0}
				onClick={() => overflows && setExpanded((v) => !v)}
				onKeyDown={(e) => {
					if ((e.key === "Enter" || e.key === " ") && overflows) {
						setExpanded((v) => !v);
					}
				}}
				className={cn(
					"flex items-center gap-2 px-3 py-2",
					overflows && "cursor-pointer",
				)}
			>
				{isRunning ? (
					<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-link" />
				) : isError ? (
					<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
				) : (
					<CheckIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
				)}
				<span
					className={cn(
						"min-w-0 flex-1 truncate text-sm",
						isError ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{title}
				</span>
				{overflows && (
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							expanded ? "rotate-0" : "-rotate-90",
						)}
					/>
				)}
			</div>
			{report.trim() && (
				<>
					<div
						className="h-px"
						style={{ background: "hsl(var(--border-default))" }}
					/>
					<div className="px-3 py-2">
						<div
							ref={measureRef}
							style={
								expanded
									? undefined
									: {
											maxHeight: COLLAPSED_REPORT_HEIGHT,
											overflow: "hidden",
										}
							}
						>
							<Response>{report}</Response>
						</div>
					</div>
				</>
			)}
		</div>
	);
};

export const Tool = forwardRef<HTMLDivElement, ToolProps>(
	(
		{
			className,
			name,
			status = "completed",
			args,
			result,
			isError = false,
			...props
		},
		ref,
	) => {
		const resultOutput = formatResultOutput(result);
		const fileContent = useMemo(
			() => getFileContentForViewer(name, args, result),
			[name, args, result],
		);
		const writeFileDiff = useMemo(
			() => getWriteFileDiff(name, args),
			[name, args],
		);

		// Render execute tools with the specialized terminal-style block.
		if (name === "execute") {
			const parsed = parseArgs(args);
			const command = parsed ? asString(parsed.command) : "";
			const rec = asRecord(result);
			const output = rec ? asString(rec.output).trim() : "";

			return (
				<div ref={ref} className={cn("w-full py-0.5", className)} {...props}>
					<ExecuteTool
						command={command || "Running command…"}
						output={output}
						status={status}
						isError={isError}
					/>
				</div>
			);
		}

		// Render read_file with a collapsed-by-default viewer.
		if (name === "read_file") {
			const parsed = parseArgs(args);
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
			const parsed = parseArgs(args);
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
			const files = parseEditFilesArgs(args);
			const diffs = files.map((f) => buildEditDiff(f.path, f.edits));
			const rec = asRecord(result);

			return (
				<div ref={ref} className={cn("py-0.5", className)} {...props}>
					<EditFilesTool
						files={files}
						diffs={diffs}
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
			name === "subagent_message"
		) {
			const parsed = parseArgs(args);
			const rec = asRecord(result);
			// subagent_await and subagent_message have chat_id in
			// args, so check both result and args.
			const chatId =
				(rec ? asString(rec.chat_id) : "") ||
				(parsed ? asString(parsed.chat_id) : "");
			const subagentStatus = rec ? asString(rec.status || rec.subagent_status) : "";
			const durationMs = rec ? asNumber(rec.duration_ms) : undefined;
			const report = rec ? asString(rec.report) : "";
			const prompt = parsed ? asString(parsed.prompt) : "";
			const title =
				(rec ? asString(rec.title) : "") ||
				(parsed ? asString(parsed.title) : "") ||
				"Sub-agent";
			const subagentToolStatus: ToolStatus = isSubagentSuccessStatus(
				subagentStatus,
			)
				? "completed"
				: status;
			const subagentIsError =
				(status === "error" || isError) && !isSubagentSuccessStatus(subagentStatus);

			if (chatId) {
				return (
					<div ref={ref} className={cn("py-0.5", className)} {...props}>
						<SubagentTool
							toolName={name}
							title={title}
							chatId={chatId}
							subagentStatus={subagentStatus}
							prompt={prompt || undefined}
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
						toolStatus={subagentToolStatus}
						isError={subagentIsError}
					/>
				</div>
			);
		}

		// Render subagent_report as a collapsible markdown report.
		if (name === "subagent_report") {
			const parsed = parseArgs(args);
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
						<FileDiff
							fileDiff={writeFileDiff}
							options={{
								diffStyle: "unified",
								diffIndicators: "bars",
								overflow: "scroll",
								themeType: "dark",
								theme: "github-dark-high-contrast",
								unsafeCSS: diffViewerCSS,
							}}
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
							options={{
								overflow: "scroll",
								themeType: "dark",
								theme: "github-dark-high-contrast",
								unsafeCSS: fileViewerCSS,
								disableFileHeader: fileContent.disableHeader,
								disableLineNumbers: fileContent.disableLineNumbers,
							}}
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
								options={{
									overflow: "scroll",
									themeType: "dark",
									theme: "github-dark-high-contrast",
									unsafeCSS: fileViewerCSS,
									disableFileHeader: true,
								}}
								style={{
									"--diffs-font-size": "11px",
									"--diffs-line-height": "1.5",
								}}
							/>
						</ScrollArea>
					)
				)}
			</div>
		);
	},
);

Tool.displayName = "Tool";
