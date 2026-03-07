import { Button } from "components/Button/Button";
import {
	AlertTriangleIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	FileTextIcon,
	FolderOpenIcon,
} from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { cn } from "utils/cn";
import type { DisplayToolCall } from "./useTemplateAgent";

interface ToolCallCardProps {
	toolCall: DisplayToolCall;
	onNavigateToFile?: (path: string) => void;
}

interface StructuredValueSectionProps {
	label: string;
	value: unknown;
	emptyState: string;
}

const MAX_VISIBLE_READ_LINES = 20;
const MAX_VISIBLE_DATA_LINES = 16;

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const createInspectableJsonReplacer = () => {
	const seenObjects = new WeakSet<object>();

	return (_key: string, value: unknown): unknown => {
		if (typeof value === "bigint") {
			return `${value}n`;
		}
		if (typeof value === "function") {
			return `[Function ${value.name || "anonymous"}]`;
		}
		if (value instanceof Error) {
			return {
				name: value.name,
				message: value.message,
				...(value.stack ? { stack: value.stack } : {}),
			};
		}
		if (typeof value === "object" && value !== null) {
			if (seenObjects.has(value)) {
				return "[Circular]";
			}
			seenObjects.add(value);
		}
		return value;
	};
};

const formatStructuredValue = (value: unknown): string => {
	if (value === undefined) {
		return "";
	}
	if (typeof value === "string") {
		return value;
	}

	try {
		const formatted = JSON.stringify(value, createInspectableJsonReplacer(), 2);
		if (typeof formatted === "string") {
			return formatted;
		}
	} catch {
		// Ignore serialization failures and fall back to String below.
	}

	return String(value);
};

const StructuredValueSection: FC<StructuredValueSectionProps> = ({
	label,
	value,
	emptyState,
}) => {
	const [expanded, setExpanded] = useState(false);

	const formattedValue = useMemo(() => formatStructuredValue(value), [value]);
	const valueLines = useMemo(
		() => formattedValue.split("\n"),
		[formattedValue],
	);
	const hasValue = formattedValue.length > 0;
	const shouldTruncate = valueLines.length > MAX_VISIBLE_DATA_LINES;
	const displayedValue =
		expanded || !shouldTruncate
			? formattedValue
			: valueLines.slice(0, MAX_VISIBLE_DATA_LINES).join("\n");

	return (
		<div className="space-y-1.5">
			<p className="m-0 text-2xs font-medium uppercase tracking-wide text-content-secondary">
				{label}
			</p>
			{hasValue ? (
				<>
					<pre className="m-0 overflow-x-auto whitespace-pre-wrap break-words rounded-md bg-surface-secondary p-2 text-[11px] leading-relaxed text-content-primary">
						{displayedValue}
					</pre>
					{shouldTruncate && (
						<Button
							variant="subtle"
							size="xs"
							onClick={() => setExpanded((prev) => !prev)}
						>
							{expanded ? "Show less" : "Show more"}
						</Button>
					)}
				</>
			) : (
				<p className="m-0 text-xs text-content-secondary">{emptyState}</p>
			)}
		</div>
	);
};

export const ToolCallCard: FC<ToolCallCardProps> = ({
	toolCall,
	onNavigateToFile,
}) => {
	const [expanded, setExpanded] = useState(false);
	const [showAllReadLines, setShowAllReadLines] = useState(false);

	const result = isRecord(toolCall.result) ? toolCall.result : null;
	const error = typeof result?.error === "string" ? result.error : null;
	const path =
		typeof toolCall.args.path === "string" ? toolCall.args.path : null;
	const isListFiles = toolCall.toolName === "listFiles";
	const isReadFile = toolCall.toolName === "readFile";
	const hasSpecialRendering = isListFiles || isReadFile;

	const files = useMemo(() => {
		if (!Array.isArray(result?.files)) {
			return [];
		}
		return result.files.filter(
			(file): file is string => typeof file === "string",
		);
	}, [result]);

	const readFileContent =
		typeof result?.content === "string" ? result.content : undefined;
	const readFileLines = useMemo(() => {
		if (typeof readFileContent !== "string") {
			return [];
		}
		return readFileContent.split("\n");
	}, [readFileContent]);
	const hasTruncatedReadFile = readFileLines.length > MAX_VISIBLE_READ_LINES;
	const displayedReadFileLines = showAllReadLines
		? readFileLines
		: readFileLines.slice(0, MAX_VISIBLE_READ_LINES);

	const Icon = isListFiles ? FolderOpenIcon : FileTextIcon;

	return (
		<div>
			<button
				type="button"
				onClick={() => setExpanded((prev) => !prev)}
				aria-expanded={expanded}
				className={cn(
					"flex w-full items-center gap-2 rounded-md px-1 py-1 text-left",
					"cursor-pointer border-none bg-transparent transition-colors",
					"hover:bg-surface-secondary",
				)}
			>
				{expanded ? (
					<ChevronDownIcon className="size-3.5 text-content-secondary" />
				) : (
					<ChevronRightIcon className="size-3.5 text-content-secondary" />
				)}
				<Icon className="size-3.5 text-content-secondary" />
				<span className="text-xs font-medium text-content-primary">
					{toolCall.toolName}
				</span>
				{toolCall.state === "pending" && (
					<span className="ml-auto text-2xs text-content-secondary">
						Running…
					</span>
				)}
			</button>

			{expanded && (
				<div className="ml-3 border-solid border-0 border-l-2 border-border pl-3 pb-1 pt-1">
					{error && (
						<div className="mb-2 flex items-start gap-2 rounded-md bg-surface-destructive/10 p-2 text-xs text-content-destructive">
							<AlertTriangleIcon className="mt-0.5 size-3.5 shrink-0" />
							<span>{error}</span>
						</div>
					)}

					{isListFiles && files.length > 0 && (
						<ul className="m-0 list-none space-y-0.5 p-0">
							{files.map((file) => (
								<li key={file}>
									<button
										type="button"
										onClick={() => onNavigateToFile?.(file)}
										className={cn(
											"border-none bg-transparent p-0 text-left text-xs",
											"text-content-link hover:underline",
											onNavigateToFile ? "cursor-pointer" : "cursor-default",
										)}
										disabled={!onNavigateToFile}
									>
										{file}
									</button>
								</li>
							))}
						</ul>
					)}

					{isReadFile && readFileContent !== undefined && (
						<div className="space-y-1.5">
							{path && (
								<button
									type="button"
									onClick={() => onNavigateToFile?.(path)}
									disabled={!onNavigateToFile}
									className={cn(
										"border-none bg-transparent p-0 text-xs",
										"text-content-link hover:underline",
										onNavigateToFile ? "cursor-pointer" : "cursor-default",
									)}
								>
									{path}
								</button>
							)}
							<pre className="m-0 overflow-x-auto rounded-md bg-surface-secondary p-2 text-[11px] leading-relaxed text-content-primary">
								{displayedReadFileLines.join("\n")}
							</pre>
							{hasTruncatedReadFile && (
								<Button
									variant="subtle"
									size="xs"
									onClick={() => setShowAllReadLines((prev) => !prev)}
								>
									{showAllReadLines ? "Show less" : "Show more"}
								</Button>
							)}
						</div>
					)}

					{!hasSpecialRendering && (
						<div className="space-y-3">
							<StructuredValueSection
								label="Arguments"
								value={toolCall.args}
								emptyState="No arguments were provided."
							/>
							{toolCall.state === "pending" ? (
								<p className="m-0 text-xs text-content-secondary">
									Waiting for tool result…
								</p>
							) : (
								<StructuredValueSection
									label="Result"
									value={toolCall.result}
									emptyState="No result was returned."
								/>
							)}
						</div>
					)}

					{hasSpecialRendering && toolCall.state === "pending" && !error && (
						<p className="m-0 text-xs text-content-secondary">
							Waiting for tool result…
						</p>
					)}
				</div>
			)}
		</div>
	);
};
