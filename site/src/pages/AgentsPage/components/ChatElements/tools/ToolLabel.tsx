import type React from "react";
import {
	getProvidedSubagentTitle,
	getSubagentDescriptor,
} from "./subagentDescriptor";
import { asRecord, asString, humanizeMCPToolName, parseArgs } from "./utils";

const renderSubagentLabel = (
	name: string,
	args: unknown,
	result: unknown,
): React.ReactNode | null => {
	const descriptor = getSubagentDescriptor({ name, args, result });
	if (!descriptor) {
		return null;
	}

	const providedTitle = getProvidedSubagentTitle({ args, result });
	const fallbackTitle = descriptor.fallbackTitle;
	const text = (() => {
		switch (descriptor.action) {
			case "spawn":
				if (providedTitle) {
					return `Spawning ${providedTitle}`;
				}
				if (descriptor.variant === "explore") {
					return "Spawning Explore agent…";
				}
				if (descriptor.variant === "computer_use") {
					return "Spawning computer use sub-agent…";
				}
				return `Spawning ${fallbackTitle}…`;
			case "wait":
				return providedTitle
					? `Waiting for ${providedTitle}`
					: `Waiting for ${fallbackTitle}…`;
			case "message":
				return providedTitle
					? `Messaging ${providedTitle}`
					: `Messaging ${fallbackTitle}…`;
			case "close":
				return providedTitle
					? `Terminating ${providedTitle}`
					: `Terminating ${fallbackTitle}`;
		}
	})();

	return <span className="truncate text-[13px]">{text}</span>;
};

export const ToolLabel: React.FC<{
	name: string;
	args: unknown;
	result: unknown;
	mcpSlug?: string;
}> = ({ name, args, result, mcpSlug }) => {
	const parsed = parseArgs(args);
	const parsedResult = asRecord(result);
	const subagentLabel = renderSubagentLabel(
		name,
		parsed ?? args,
		parsedResult ?? result,
	);
	if (subagentLabel) {
		return subagentLabel;
	}

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
			return <span className="truncate text-[13px]">Running command</span>;
		}
		case "process_output":
			return (
				<span className="truncate text-[13px]">Reading process output</span>
			);
		case "process_signal": {
			const signal = parsed ? asString(parsed.signal) : "";
			const processId = parsed ? asString(parsed.process_id) : "";
			const shortId = processId ? processId.slice(0, 8) : "";
			const hasResult = result !== undefined && result !== null;
			const success = parsedResult ? Boolean(parsedResult.success) : false;
			if (hasResult && success) {
				const verb = signal === "kill" ? "Killed" : "Terminated";
				return (
					<span className="truncate text-[13px]">
						{verb} process{shortId ? ` ${shortId}` : ""}
					</span>
				);
			}
			if (hasResult && !success) {
				const verb =
					signal === "kill"
						? "kill"
						: signal === "terminate"
							? "terminate"
							: "signal";
				return (
					<span className="truncate text-[13px]">
						Failed to {verb} process{shortId ? ` ${shortId}` : ""}
					</span>
				);
			}
			return (
				<span className="truncate text-[13px]">
					{signal === "kill"
						? "Killing process…"
						: signal === "terminate"
							? "Terminating process…"
							: "Sending signal…"}
				</span>
			);
		}
		case "process_list":
			return <span className="truncate text-[13px]">Listing processes</span>;
		case "read_file":
			return <span className="truncate text-[13px]">Reading file…</span>;
		case "write_file": {
			const path = parsed ? asString(parsed.path) : "";
			if (path) {
				return (
					<code className="truncate font-mono text-xs text-content-primary">
						{path}
					</code>
				);
			}
			return <span className="truncate text-[13px]">Writing file</span>;
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
			return <span className="truncate text-[13px]">Editing files</span>;
		}
		case "create_workspace": {
			const wsName = parsedResult ? asString(parsedResult.workspace_name) : "";
			if (wsName) {
				return <span className="truncate text-[13px]">Created {wsName}</span>;
			}
			return <span className="truncate text-[13px]">Creating workspace</span>;
		}
		case "list_templates": {
			const count = parsedResult
				? ((parsedResult.count as number | undefined) ?? 0)
				: 0;
			return (
				<span className="truncate text-[13px]">
					{count === 0
						? "Listing templates…"
						: count === 1
							? "Listed 1 template"
							: `Listed ${count} templates`}
				</span>
			);
		}
		case "read_template": {
			const templateRec = parsedResult
				? asRecord(parsedResult.template)
				: undefined;
			const tmplName = templateRec
				? asString(templateRec.display_name) || asString(templateRec.name)
				: "";
			return (
				<span className="truncate text-[13px]">
					{tmplName ? `Read template ${tmplName}` : "Reading template…"}
				</span>
			);
		}
		case "chat_summarized":
			return <span className="truncate text-[13px]">Summarized</span>;
		case "attach_file": {
			const attachedName =
				(parsedResult ? asString(parsedResult.name) : "") ||
				(parsed ? asString(parsed.name) : "") ||
				(parsed ? asString(parsed.path).split("/").pop() : "") ||
				"file";
			return (
				<span className="truncate text-[13px]">{`Attached ${attachedName}`}</span>
			);
		}
		case "computer":
			return <span className="truncate text-[13px]">Screenshot</span>;
		case "propose_plan": {
			const path = parsed ? asString(parsed.path) || "PLAN.md" : "PLAN.md";
			const filename = path.split("/").pop() || "PLAN.md";
			return <span className="truncate text-[13px]">{filename}</span>;
		}
		case "read_skill": {
			const skillName = parsed ? asString(parsed.name) : "";
			return (
				<span className="truncate text-[13px]">
					{skillName
						? parsedResult
							? `Read skill ${skillName}`
							: `Reading skill ${skillName}…`
						: "Reading skill…"}
				</span>
			);
		}
		case "read_skill_file": {
			const skillName = parsed ? asString(parsed.name) : "";
			const filePath = parsed ? asString(parsed.path) : "";
			const label =
				skillName && filePath
					? `${skillName}/${filePath}`
					: skillName || filePath || "skill file";
			return (
				<span className="truncate text-[13px]">
					{parsedResult ? `Read ${label}` : `Reading ${label}…`}
				</span>
			);
		}
		case "start_workspace": {
			const wsName = parsedResult ? asString(parsedResult.workspace_name) : "";
			return (
				<span className="truncate text-[13px]">
					{wsName ? `Started ${wsName}` : "Starting workspace…"}
				</span>
			);
		}

		default: {
			const displayName = mcpSlug ? humanizeMCPToolName(mcpSlug, name) : name;
			return <span className="truncate text-[13px]">{displayName}</span>;
		}
	}
};
