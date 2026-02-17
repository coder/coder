import { parsePatchFiles } from "@pierre/diffs";
import { File as FileViewer, FileDiff } from "@pierre/diffs/react";
import type { FileDiffMetadata } from "@pierre/diffs";
import { forwardRef, useMemo, useRef, useState } from "react";
import { CopyButton } from "components/CopyButton/CopyButton";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { cn } from "utils/cn";
import {
	ChevronDownIcon,
	CircleAlertIcon,
	FileIcon,
	FilePenIcon,
	LoaderIcon,
	PlusCircleIcon,
	TerminalIcon,
	WrenchIcon,
} from "lucide-react";

type ToolStatus = "completed" | "error" | "running";

interface ToolProps extends Omit<React.HTMLAttributes<HTMLDivElement>, "children"> {
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
	const color = isError
		? "text-content-destructive"
		: "text-content-secondary";
	const base = `h-4 w-4 shrink-0 ${color}`;
	switch (name) {
		case "execute":
			return <TerminalIcon className={base} />;
		case "read_file":
			return <FileIcon className={base} />;
		case "write_file":
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
): { path: string; content: string; disableHeader?: boolean; disableLineNumbers?: boolean } | null => {
	if (toolName === "execute") {
		const rec = asRecord(result);
		if (!rec) {
			return null;
		}
		const output = asString(rec.output).trim();
		if (!output) {
			return null;
		}
		return { path: "output.sh", content: output, disableHeader: true, disableLineNumbers: true };
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
			<div className="h-px" style={{ background: "hsl(var(--border-default))" }} />

			{/* Output preview / expanded */}
			{hasOutput && (
				<>
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
								isError
									? "text-content-destructive"
									: "text-content-secondary",
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
}> = ({ path, content, status, isError }) => {
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
				<span className={cn(
					"text-sm",
					isError ? "text-content-destructive" : "text-content-secondary",
				)}>
					Read{" "}
					{path.split("/").pop() || path}
				</span>
				{isError && (
					<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
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
							'--diffs-font-size': '11px',
							'--diffs-line-height': '1.5',
						}}
					/>
				</ScrollArea>
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
				<ScrollArea className="mt-1.5 ml-6 rounded-md border border-solid border-border-default text-2xs" viewportClassName="max-h-64" scrollBarClassName="w-1.5">
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
				<ScrollArea className="mt-1.5 ml-6 rounded-md border border-solid border-border-default text-2xs" viewportClassName="max-h-64" scrollBarClassName="w-1.5">
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
					<ScrollArea className="mt-1.5 ml-6 rounded-md border border-solid border-border-default text-2xs" viewportClassName="max-h-64" scrollBarClassName="w-1.5">
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
						/>
					</ScrollArea>
				)
			)}
			</div>
		);
	},
);

Tool.displayName = "Tool";
