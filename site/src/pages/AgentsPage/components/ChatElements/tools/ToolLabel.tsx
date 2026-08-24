import type React from "react";
import { getPathBasename } from "../../../utils/path";
import { asRecord, asString, humanizeMCPToolName, parseArgs } from "./utils";

type ToolLabelProps = {
	name: string;
	args: unknown;
	result: unknown;
	mcpSlug?: string;
};

const ProcessSignalLabel: React.FC<ToolLabelProps> = ({ args, result }) => {
	const parsed = parseArgs(args);
	const parsedResult = asRecord(result);
	const signal = parsed ? asString(parsed.signal) : "";
	const processId = parsed ? asString(parsed.process_id) : "";
	const shortId = processId ? processId.slice(0, 8) : "";
	const suffix = shortId ? ` ${shortId}` : "";
	const isKill = signal === "kill";
	const isTerminate = signal === "terminate";

	const hasResult = result !== undefined && result !== null;
	if (!hasResult) {
		const inFlightVerb = isKill
			? "Killing process…"
			: isTerminate
				? "Terminating process…"
				: "Sending signal…";
		return <span className="truncate text-[13px]">{inFlightVerb}</span>;
	}

	const success = parsedResult ? Boolean(parsedResult.success) : false;
	if (success) {
		const verb = isKill ? "Killed" : "Terminated";
		return (
			<span className="truncate text-[13px]">
				{verb} process{suffix}
			</span>
		);
	}

	const failedVerb = isKill ? "kill" : isTerminate ? "terminate" : "signal";
	return (
		<span className="truncate text-[13px]">
			Failed to {failedVerb} process{suffix}
		</span>
	);
};

const AttachFileLabel: React.FC<ToolLabelProps> = ({ args, result }) => {
	const parsed = parseArgs(args);
	const parsedResult = asRecord(result);
	const resultName = parsedResult ? asString(parsedResult.name) : "";
	const argName = parsed ? asString(parsed.name) : "";
	const argPath = parsed ? asString(parsed.path) : "";
	const attachedName =
		resultName || argName || getPathBasename(argPath) || "file";
	return (
		<span className="truncate text-[13px]">{`Attached ${attachedName}`}</span>
	);
};

export const genericToolLabels: Partial<
	Record<string, React.FC<ToolLabelProps>>
> = {
	process_signal: ProcessSignalLabel,
	process_list: () => (
		<span className="truncate text-[13px]">Listing processes</span>
	),
	attach_file: AttachFileLabel,
	advisor: () => (
		<span className="truncate text-[13px] leading-4 text-content-secondary">
			Advisor
		</span>
	),
};

export const ToolLabel: React.FC<ToolLabelProps> = (props) => {
	const Label = genericToolLabels[props.name];
	if (Label) {
		return <Label {...props} />;
	}
	const displayName = props.mcpSlug
		? humanizeMCPToolName(props.mcpSlug, props.name)
		: props.name;
	return <span className="truncate text-[13px]">{displayName}</span>;
};
