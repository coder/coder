import type React from "react";
import { getPathBasename } from "../../../utils/path";
import { asRecord, asString, humanizeMCPToolName, parseArgs } from "./utils";

/**
 * Label for tools rendered by `GenericToolRenderer`, which is every tool
 * without an entry in `toolRenderers`, plus `process_signal`, which delegates
 * to it, and `advisor`, which renders this directly.
 */
export const ToolLabel: React.FC<{
	name: string;
	args: unknown;
	result: unknown;
	mcpSlug?: string;
}> = ({ name, args, result, mcpSlug }) => {
	const parsed = parseArgs(args);
	const parsedResult = asRecord(result);

	switch (name) {
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
		case "attach_file": {
			const attachedName =
				(parsedResult ? asString(parsedResult.name) : "") ||
				(parsed ? asString(parsed.name) : "") ||
				(parsed ? getPathBasename(asString(parsed.path)) : "") ||
				"file";
			return (
				<span className="truncate text-[13px]">{`Attached ${attachedName}`}</span>
			);
		}
		case "advisor":
			return (
				<span className="truncate text-[13px] leading-4 text-content-secondary">
					Advisor
				</span>
			);

		default: {
			const displayName = mcpSlug ? humanizeMCPToolName(mcpSlug, name) : name;
			return <span className="truncate text-[13px]">{displayName}</span>;
		}
	}
};
