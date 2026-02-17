import { forwardRef } from "react";
import { cn } from "utils/cn";
import {
	CheckCircle2Icon,
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

const StatusIcon: React.FC<{ status: ToolStatus; isError: boolean }> = ({
	status,
	isError,
}) => {
	if (status === "running") {
		return (
			<LoaderIcon className="h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary" />
		);
	}
	if (status === "error" || isError) {
		return (
			<CircleAlertIcon className="h-3.5 w-3.5 shrink-0 text-content-destructive" />
		);
	}
	return (
		<CheckCircle2Icon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
	);
};

const ToolIcon: React.FC<{ name: string }> = ({ name }) => {
	switch (name) {
		case "execute":
			return (
				<TerminalIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
			);
		case "read_file":
			return (
				<FileIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
			);
		case "write_file":
			return (
				<FilePenIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
			);
		case "create_workspace":
			return (
				<PlusCircleIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
			);
		default:
			return (
				<WrenchIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
			);
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
					<code className="truncate font-mono text-2xs text-content-primary">
						{command}
					</code>
				);
			}
			return (
				<span className="truncate text-xs text-content-secondary">
					Running command
				</span>
			);
		}
		case "read_file": {
			const path = parsed ? asString(parsed.path) : "";
			if (path) {
				return (
					<code className="truncate font-mono text-2xs text-content-primary">
						{path}
					</code>
				);
			}
			return (
				<span className="truncate text-xs text-content-secondary">
					Reading file
				</span>
			);
		}
		case "write_file": {
			const path = parsed ? asString(parsed.path) : "";
			if (path) {
				return (
					<code className="truncate font-mono text-2xs text-content-primary">
						{path}
					</code>
				);
			}
			return (
				<span className="truncate text-xs text-content-secondary">
					Writing file
				</span>
			);
		}
		case "create_workspace": {
			const wsName = parsedResult ? asString(parsedResult.workspace_name) : "";
			if (wsName) {
				return (
					<span className="truncate text-xs text-content-secondary">
						Created {wsName}
					</span>
				);
			}
			return (
				<span className="truncate text-xs text-content-secondary">
					Creating workspace
				</span>
			);
		}
		default:
			return (
				<span className="truncate text-xs text-content-secondary">{name}</span>
			);
	}
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

		return (
			<div ref={ref} className={cn("py-0.5", className)} {...props}>
				<div className="flex items-center gap-2">
					<StatusIcon status={status} isError={isError} />
					<ToolIcon name={name} />
					<ToolLabel name={name} args={args} result={result} />
				</div>
				{resultOutput && (
					<pre className="mt-1.5 ml-8 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-surface-tertiary px-3 py-2 text-2xs leading-relaxed text-content-secondary">
						{resultOutput}
					</pre>
				)}
			</div>
		);
	},
);

Tool.displayName = "Tool";
