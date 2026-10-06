import type React from "react";
import { getPathBasename } from "../../../utils/path";
import { asRecord, asString, humanizeMCPToolName, parseArgs } from "./utils";

type ToolLabelProps = {
	name: string;
	args: unknown;
	result: unknown;
	isError: boolean;
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

const manageAutomationsLabels: Partial<
	Record<string, readonly [running: string, done: string, failed: string]>
> = {
	list: ["Listing automations", "Listed automations", "list automations"],
	get: ["Reading automation", "Read automation", "read automation"],
	create: ["Creating automation", "Created automation", "create automation"],
	update: ["Updating automation", "Updated automation", "update automation"],
	enable: ["Enabling automation", "Enabled automation", "enable automation"],
	disable: [
		"Disabling automation",
		"Disabled automation",
		"disable automation",
	],
	delete: ["Deleting automation", "Deleted automation", "delete automation"],
	run_now: ["Running automation", "Ran automation", "run automation"],
};

const ManageAutomationsLabel: React.FC<ToolLabelProps> = ({
	args,
	result,
	isError,
}) => {
	const parsed = parseArgs(args);
	const action = asString(parsed?.action).trim();
	// The action comes from model output, so ignore inherited keys such as
	// "constructor" instead of reading them as label tuples.
	const labels = Object.hasOwn(manageAutomationsLabels, action)
		? manageAutomationsLabels[action]
		: undefined;
	const parsedResult = asRecord(result);
	const automationName =
		asString(asRecord(parsedResult?.automation)?.name) ||
		asString(parsed?.name);
	const suffix = automationName ? ` ${automationName}` : "";
	let label = "Manage automations";
	if (labels) {
		const [running, done, failed] = labels;
		if (isError || parsedResult?.error) {
			label = `Failed to ${failed}${suffix}`;
		} else if (result === undefined || result === null) {
			label = `${running}${suffix}…`;
		} else {
			label = `${done}${suffix}`;
		}
	}
	return <span className="truncate text-[13px]">{label}</span>;
};

export const genericToolLabels: Partial<
	Record<string, React.FC<ToolLabelProps>>
> = {
	process_signal: ProcessSignalLabel,
	process_list: () => (
		<span className="truncate text-[13px]">Listing processes</span>
	),
	attach_file: AttachFileLabel,
	manage_automations: ManageAutomationsLabel,
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
