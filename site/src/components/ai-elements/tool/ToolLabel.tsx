import type React from "react";
import { asRecord, asString, parseArgs } from "./utils";

export const ToolLabel: React.FC<{
	name: string;
	args: unknown;
	result: unknown;
}> = ({ name, args, result }) => {
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
		case "list_templates": {
			const count = parsedResult
				? ((parsedResult.count as number | undefined) ?? 0)
				: 0;
			return (
				<span className="truncate text-sm text-content-secondary">
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
				<span className="truncate text-sm text-content-secondary">
					{tmplName ? `Read template ${tmplName}` : "Reading template…"}
				</span>
			);
		}
		case "spawn_agent": {
			const spawnTitle =
				(parsedResult ? asString(parsedResult.title) : "") ||
				(parsed ? asString(parsed.title) : "");
			return (
				<span className="truncate text-sm text-content-secondary">
					{spawnTitle ? `Spawning ${spawnTitle}` : "Spawning sub-agent…"}
				</span>
			);
		}
		case "wait_agent": {
			const awaitTitle =
				(parsedResult ? asString(parsedResult.title) : "") ||
				(parsed ? asString(parsed.title) : "");
			return (
				<span className="truncate text-sm text-content-secondary">
					{awaitTitle ? `Waiting for ${awaitTitle}` : "Waiting for sub-agent…"}
				</span>
			);
		}
		case "message_agent": {
			const msgTitle =
				(parsedResult ? asString(parsedResult.title) : "") ||
				(parsed ? asString(parsed.title) : "");
			return (
				<span className="truncate text-sm text-content-secondary">
					{msgTitle ? `Messaging ${msgTitle}` : "Messaging sub-agent…"}
				</span>
			);
		}
		case "close_agent":
			return (
				<span className="truncate text-sm text-content-secondary">
					Terminating sub-agent
				</span>
			);
		case "chat_summarized":
			return (
				<span className="truncate text-sm text-content-secondary">
					Summarized
				</span>
			);
		default:
			return (
				<span className="truncate text-sm text-content-secondary">{name}</span>
			);
	}
};
